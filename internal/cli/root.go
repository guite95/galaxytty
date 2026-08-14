package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/doctor"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/tui"
)

type options struct {
	mock    bool
	json    bool
	help    bool
	device  string
	command []string
}

type dependencies struct {
	mock   func(config.Config) (*bootstrap.Runtime, error)
	real   func(context.Context, config.Config, string) (*bootstrap.Runtime, error)
	doctor func(context.Context, config.Config, string) doctor.Report
}

func defaultDependencies() dependencies {
	return dependencies{mock: bootstrap.Mock, real: bootstrap.Real, doctor: doctor.Run}
}

func (d dependencies) withDefaults() dependencies {
	defaults := defaultDependencies()
	if d.mock == nil {
		d.mock = defaults.mock
	}
	if d.real == nil {
		d.real = defaults.real
	}
	if d.doctor == nil {
		d.doctor = defaults.doctor
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
	path, err := config.Path()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	deps = deps.withDefaults()

	command := ""
	if len(opts.command) > 0 {
		command = opts.command[0]
	}
	if !opts.mock && command == "send" {
		return domain.ErrSendingNotImplemented
	}
	if command == "doctor" {
		if opts.mock {
			fmt.Fprintln(out, "GalaxyTTY Doctor\n\n✓ mock adapters          ready\n✓ SMS Provider fixture   accessible\n✓ Virtual Display fake   supported\n\nConnection:\n  Mock\n\nReady.")
			return nil
		}
		report := deps.doctor(ctx, cfg, opts.device)
		printDoctor(out, report)
		if !report.Ready {
			return fmt.Errorf("%s", report.Summary)
		}
		return nil
	}

	var runtime *bootstrap.Runtime
	if opts.mock {
		runtime, err = deps.mock(cfg)
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
		_, err := service.SendToAddress(ctx, *to, *text)
		return err
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
	default:
		return err
	}
}

func parseOptions(args []string) (options, error) {
	var opts options
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--mock":
			opts.mock = true
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
		default:
			opts.command = append(opts.command, args[index])
		}
	}
	return opts, nil
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "GalaxyTTY - Samsung Messages in your terminal.\n\nUsage: msg [--mock] [--json] [--device <adb-target>] [conversations|unread|messages|send|doctor]\nWithout a command, starts the TUI.")
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
