package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/doctor"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/remote"
	"github.com/galaxytty/galaxytty/internal/tui"
)

type options struct {
	mock    bool
	helper  bool
	json    bool
	help    bool
	device  string
	address string
	command []string
}

type dependencies struct {
	mock    func(config.Config) (*bootstrap.Runtime, error)
	real    func(context.Context, config.Config, string) (*bootstrap.Runtime, error)
	helper  func(context.Context, config.Config, string) (*bootstrap.Runtime, error)
	doctor  func(context.Context, config.Config, string) doctor.Report
	pair    func(context.Context, string, string) (remote.PairResult, error)
	latency func(context.Context, string) (remote.LatencyResult, error)
}

func defaultDependencies() dependencies {
	return dependencies{
		mock: bootstrap.Mock, real: bootstrap.Real, helper: bootstrap.Helper, doctor: doctor.Run,
		pair: func(ctx context.Context, address, code string) (remote.PairResult, error) {
			store, err := pairing.DefaultStore()
			if err != nil {
				return remote.PairResult{}, err
			}
			return remote.Pair(ctx, remote.Config{Address: address}, remote.NewDiscovery(4*time.Second), code, store)
		},
		latency: measureHelperLatency,
	}
}

func (d dependencies) withDefaults() dependencies {
	defaults := defaultDependencies()
	if d.mock == nil {
		d.mock = defaults.mock
	}
	if d.real == nil {
		d.real = defaults.real
	}
	if d.helper == nil {
		d.helper = defaults.helper
	}
	if d.doctor == nil {
		d.doctor = defaults.doctor
	}
	if d.pair == nil {
		d.pair = defaults.pair
	}
	if d.latency == nil {
		d.latency = defaults.latency
	}
	return d
}

func Execute(ctx context.Context, in io.Reader, out io.Writer, args []string) error {
	return execute(ctx, in, out, args, defaultDependencies())
}

func execute(ctx context.Context, in io.Reader, out io.Writer, args []string, deps dependencies) (err error) {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if !opts.mock {
		defer func() { err = actionableRealError(err) }()
	}
	if opts.help {
		printHelp(out)
		return nil
	}
	command := ""
	if len(opts.command) > 0 {
		command = opts.command[0]
	}
	if opts.mock && opts.helper {
		return fmt.Errorf("--mock and --helper cannot be used together")
	}
	if opts.address != "" && !opts.helper && command != "pair" {
		return fmt.Errorf("--helper-address requires --helper")
	}
	path, err := config.Path()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	deps = deps.withDefaults()

	if command == "pair" {
		if opts.mock {
			return fmt.Errorf("pairing is only available for Galaxy Helper")
		}
		promptOut := out
		if opts.json {
			promptOut = io.Discard
		}
		code, readErr := readPairingCode(in, promptOut)
		if readErr != nil {
			return readErr
		}
		result, pairErr := deps.pair(ctx, opts.address, code)
		if pairErr != nil {
			return pairErr
		}
		if opts.json {
			return json.NewEncoder(out).Encode(pairResponse{Paired: true, Device: result.DeviceName})
		}
		fmt.Fprintf(out, "Paired with %s.\n", result.DeviceName)
		return nil
	}
	if command == "doctor" {
		if opts.mock {
			fmt.Fprintln(out, "GalaxyTTY Doctor\n\n✓ mock adapters          ready\n✓ SMS Provider fixture   accessible\n✓ Virtual Display fake   supported\n\nConnection:\n  Mock\n\nRead: ready\nSend: ready")
			return nil
		}
		if opts.helper {
			runtime, helperErr := deps.helper(ctx, cfg, opts.address)
			if helperErr != nil {
				return helperErr
			}
			defer runtime.Service.Shutdown(context.Background())
			status := runtime.Service.Status(ctx)
			fmt.Fprintln(out, "GalaxyTTY Helper Doctor")
			fmt.Fprintln(out)
			fmt.Fprintf(out, "✓ discovery / local TCP  %s\n", status.Label)
			fmt.Fprintf(out, "✓ device                 %s\n", status.Device)
			fmt.Fprintln(out, "○ reply execution        requires local Galaxy approval")
			return nil
		}
		report := deps.doctor(ctx, cfg, opts.device)
		printDoctor(out, report)
		if !report.Ready {
			return fmt.Errorf("%s", report.Summary)
		}
		return nil
	}
	if command == "latency" {
		if !opts.helper {
			return fmt.Errorf("latency measurement is only available with --helper")
		}
		fmt.Fprintln(out, "Latency probe armed; waiting for the next Samsung Messages notification...")
		result, latencyErr := deps.latency(ctx, opts.address)
		if latencyErr != nil {
			return latencyErr
		}
		fmt.Fprintf(out, "notification_to_mac_ms=%d\n", result.NotificationToMac.Milliseconds())
		fmt.Fprintf(out, "calibration_rtt_ms=%d\n", result.CalibrationRTT.Milliseconds())
		fmt.Fprintf(out, "clock_offset_ms=%d\n", result.ClockOffset.Milliseconds())
		return nil
	}

	var runtime *bootstrap.Runtime
	if opts.mock {
		runtime, err = deps.mock(cfg)
	} else if opts.helper {
		runtime, err = deps.helper(ctx, cfg, opts.address)
	} else {
		runtime, err = deps.real(ctx, cfg, opts.device)
	}
	if err != nil {
		return err
	}
	service := runtime.Service
	defer func() {
		shutdownErr := service.Shutdown(context.Background())
		if err == nil {
			err = shutdownErr
		}
	}()

	if command == "" {
		return tui.Run(ctx, in, out, service, cfg.Polling.Interval.Duration)
	}
	printValue := func(value any) error {
		if opts.json {
			return json.NewEncoder(out).Encode(value)
		}
		switch typed := value.(type) {
		case []domain.Conversation:
			for _, conversation := range typed {
				fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", conversation.ThreadID, conversation.Title, conversation.UnreadCount, conversation.Snippet)
			}
		case []domain.Message:
			for _, message := range typed {
				fmt.Fprintf(out, "%d\t%d\t%s\n", message.ID, message.Direction, message.Body)
			}
		}
		return nil
	}

	switch command {
	case "conversations", "unread":
		var conversations []domain.Conversation
		if command == "unread" {
			conversations, err = service.Unread(ctx)
		} else {
			conversations, err = service.Conversations(ctx)
		}
		if err != nil {
			return err
		}
		return printValue(conversations)
	case "messages":
		if len(opts.command) != 2 {
			return fmt.Errorf("usage: msg messages <thread-id>")
		}
		threadID, parseErr := strconv.ParseInt(opts.command[1], 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid thread ID: %w", parseErr)
		}
		messages, messageErr := service.Messages(ctx, threadID, domain.MessageQuery{})
		if messageErr != nil {
			return messageErr
		}
		return printValue(messages)
	case "send":
		flags := flag.NewFlagSet("send", flag.ContinueOnError)
		flags.SetOutput(out)
		to := flags.String("to", "", "recipient")
		text := flags.String("text", "", "message")
		if err := flags.Parse(opts.command[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*to) == "" {
			return fmt.Errorf("recipient is required")
		}
		result, err := service.SendToAddress(ctx, *to, *text)
		if err != nil {
			return err
		}
		response := sendResponse{
			Success:   result.Outcome != domain.SendOutcomeAcceptedUnverified,
			MessageID: result.MessageID,
			ThreadID:  result.ThreadID,
			Outcome:   result.Outcome,
			Evidence:  result.Evidence,
		}
		if opts.json {
			return json.NewEncoder(out).Encode(response)
		}
		if result.Outcome == domain.SendOutcomeAcceptedUnverified {
			fmt.Fprintln(out, "Samsung Messages accepted the send action; delivery is unverified.")
			return nil
		}
		fmt.Fprintln(out, "Message sent.")
		return nil
	default:
		return fmt.Errorf("unknown command %q", strings.Join(opts.command, " "))
	}
}

func actionableRealError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrADBNotFound):
		return fmt.Errorf("%w. Install Android platform-tools and ensure adb is on PATH", err)
	case errors.Is(err, domain.ErrNoDevices):
		return fmt.Errorf("%w. Enable USB debugging or connect an already-paired Wireless Debugging device", err)
	case errors.Is(err, domain.ErrUnauthorized):
		return fmt.Errorf("%w. Authorize this Mac on the Galaxy, then retry", err)
	case errors.Is(err, domain.ErrOffline):
		return fmt.Errorf("%w. Reconnect USB or an already-paired Wireless Debugging target", err)
	case errors.Is(err, domain.ErrMultipleDevices):
		return fmt.Errorf("%w. Select one with --device <adb-target>", err)
	case errors.Is(err, domain.ErrSamsungMessagesNotInstalled):
		return fmt.Errorf("%w. Install or enable com.samsung.android.messaging", err)
	case errors.Is(err, domain.ErrProviderPermissionDenied):
		return fmt.Errorf("%w. This Galaxy does not allow ADB shell read access to the required provider", err)
	case errors.Is(err, domain.ErrProviderOutput):
		return fmt.Errorf("%w. Run msg doctor and verify the device provider shape", err)
	case errors.Is(err, domain.ErrWirelessDiscoveryUnavailable):
		return fmt.Errorf("%w. Connect an already-paired target so it appears in adb devices -l", err)
	case errors.Is(err, domain.ErrScrcpyNotFound):
		return fmt.Errorf("%w. Install scrcpy 4.1 or newer and ensure it is on PATH", err)
	case errors.Is(err, domain.ErrScrcpyStartup), errors.Is(err, domain.ErrVirtualDisplayIDNotFound), errors.Is(err, domain.ErrScrcpyExited):
		return fmt.Errorf("%w. Run msg doctor and verify the selected Galaxy remains connected", err)
	case errors.Is(err, domain.ErrClipboardRead), errors.Is(err, domain.ErrClipboardSet):
		return fmt.Errorf("%w. Ensure macOS pbcopy and pbpaste are available", err)
	case errors.Is(err, domain.ErrConversationOpen), errors.Is(err, domain.ErrComposerTap), errors.Is(err, domain.ErrClipboardPaste), errors.Is(err, domain.ErrSendTap):
		return fmt.Errorf("%w. Verify Samsung Messages is enabled and retry", err)
	case errors.Is(err, domain.ErrSendVerificationTimeout):
		return fmt.Errorf("%w. The outgoing provider row was not verified; check Samsung Messages before retrying", err)
	case errors.Is(err, app.ErrGroupSendUnsupported):
		return fmt.Errorf("Group conversation sending is not supported yet: %w", err)
	case errors.Is(err, remote.ErrDiscoveryTimeout):
		return fmt.Errorf("%w. Open GalaxyTTY Helper on a Galaxy connected to the same local network", err)
	case errors.Is(err, remote.ErrNotConnected), errors.Is(err, remote.ErrDisconnected):
		return fmt.Errorf("%w. Keep GalaxyTTY Helper running and verify local Wi-Fi connectivity", err)
	case errors.Is(err, remote.ErrPairingRequired):
		return fmt.Errorf("%w. Open Helper, reveal the pairing code, then run msg pair", err)
	case errors.Is(err, remote.ErrAuthentication):
		return fmt.Errorf("%w. Run msg pair again with the current Helper code", err)
	default:
		return err
	}
}

func measureHelperLatency(ctx context.Context, address string) (remote.LatencyResult, error) {
	credentialStore, err := pairing.DefaultStore()
	if err != nil {
		return remote.LatencyResult{}, fmt.Errorf("configure Galaxy Helper credential store: %w", err)
	}
	clientConfig := remote.Config{Address: address, Credentials: credentialStore}
	var client *remote.Client
	if address == "" {
		client, err = remote.NewDiscoveredClient(clientConfig, remote.NewDiscovery(4*time.Second))
	} else {
		client, err = remote.NewClient(clientConfig)
	}
	if err != nil {
		return remote.LatencyResult{}, fmt.Errorf("configure Galaxy Helper latency client: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = client.Run(runContext) }()
	waitContext, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	err = client.WaitConnected(waitContext)
	waitCancel()
	if err != nil {
		return remote.LatencyResult{}, fmt.Errorf("connect to Galaxy Helper: %w", err)
	}
	return remote.MeasureNextMessage(ctx, client, 7)
}

type sendResponse struct {
	Success   bool               `json:"success"`
	MessageID int64              `json:"message_id"`
	ThreadID  int64              `json:"thread_id"`
	Outcome   domain.SendOutcome `json:"outcome,omitempty"`
	Evidence  string             `json:"evidence,omitempty"`
}

type pairResponse struct {
	Paired bool   `json:"paired"`
	Device string `json:"device"`
}

func parseOptions(args []string) (options, error) {
	var opts options
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--mock":
			opts.mock = true
		case "--helper":
			opts.helper = true
		case "--json":
			opts.json = true
		case "--help", "-h":
			opts.help = true
		case "--device":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return options{}, fmt.Errorf("--device requires an adb target")
			}
			index++
			opts.device = args[index]
		case "--helper-address":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return options{}, fmt.Errorf("--helper-address requires host:port")
			}
			index++
			opts.address = args[index]
		default:
			opts.command = append(opts.command, args[index])
		}
	}
	return opts, nil
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "GalaxyTTY - Samsung Messages in your terminal.\n\nUsage: msg [--mock|--helper] [--json] [--device <adb-target>] [--helper-address <host:port>] [pair|conversations|unread|messages|send|doctor|latency]\nWithout a command, starts the TUI. Run 'msg pair' once before using Helper mode. --helper uses automatic local discovery; --helper-address is a debug fallback.")
}

func readPairingCode(in io.Reader, out io.Writer) (string, error) {
	fmt.Fprint(out, "Pairing code: ")
	if input, ok := in.(*os.File); ok && term.IsTerminal(input.Fd()) {
		value, err := term.ReadPassword(input.Fd())
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read pairing code: %w", err)
		}
		return strings.TrimSpace(string(value)), nil
	}
	value, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read pairing code: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func printDoctor(out io.Writer, report doctor.Report) {
	fmt.Fprintln(out, "GalaxyTTY Doctor")
	fmt.Fprintln(out)
	for _, check := range report.Checks {
		symbol := "○"
		switch check.State {
		case doctor.Pass:
			symbol = "✓"
		case doctor.Fail:
			symbol = "✗"
		}
		fmt.Fprintf(out, "%s %-22s %s\n", symbol, check.Name, check.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, report.Summary)
}
