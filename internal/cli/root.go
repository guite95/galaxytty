package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/tui"
	"io"
	"strconv"
	"strings"
)

func Execute(ctx context.Context, in io.Reader, out io.Writer, args []string) (err error) {
	mockMode, jsonOut := false, false
	clean := []string{}
	for _, a := range args {
		if a == "--mock" {
			mockMode = true
		} else if a == "--json" {
			jsonOut = true
		} else {
			clean = append(clean, a)
		}
	}
	help := func() {
		fmt.Fprintln(out, "GalaxyTTY - Samsung Messages in your terminal.\n\nUsage: msg [--mock] [--json] [conversations|unread|messages|send|doctor]\nWithout a command, starts the TUI.")
	}
	if len(clean) > 0 && (clean[0] == "--help" || clean[0] == "-h") {
		help()
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
	if !mockMode {
		return fmt.Errorf("real device adapters are not implemented; run with --mock")
	}
	b := mock.New()
	n := &mock.Notifier{}
	d := &mock.Display{}
	lifecycle := app.NewLifecycle(d, n)
	_ = lifecycle.Transition(app.Connecting)
	_ = lifecycle.Transition(app.Ready)
	service := app.NewService(b, b, n, lifecycle, app.NotificationPolicy{Enabled: cfg.Notifications.Enabled, ShowWhenFocused: cfg.Notifications.ShowWhenFocused}, domain.ApplicationStatus{Label: "Mock Connected"})
	if err = service.InitializePolling(ctx); err != nil {
		return err
	}
	defer func() {
		shutdownErr := service.Shutdown(context.Background())
		if err == nil {
			err = shutdownErr
		}
	}()
	if len(clean) == 0 {
		return tui.Run(ctx, in, out, service, cfg.Polling.Interval.Duration)
	}
	cmd := clean[0]
	print := func(v any) error {
		if jsonOut {
			return json.NewEncoder(out).Encode(v)
		}
		switch x := v.(type) {
		case []domain.Conversation:
			for _, c := range x {
				fmt.Fprintf(out, "%d\t%s\t%d\t%s\n", c.ThreadID, c.Title, c.UnreadCount, c.Snippet)
			}
		case []domain.Message:
			for _, m := range x {
				fmt.Fprintf(out, "%d\t%d\t%s\n", m.ID, m.Direction, m.Body)
			}
		}
		return nil
	}
	switch cmd {
	case "conversations", "unread":
		var v []domain.Conversation
		var e error
		if cmd == "unread" {
			v, e = service.Unread(ctx)
		} else {
			v, e = service.Conversations(ctx)
		}
		if e != nil {
			return e
		}
		return print(v)
	case "messages":
		if len(clean) != 2 {
			return fmt.Errorf("usage: msg messages <thread-id>")
		}
		id, e := strconv.ParseInt(clean[1], 10, 64)
		if e != nil {
			return e
		}
		v, e := service.Messages(ctx, id, domain.MessageQuery{})
		if e != nil {
			return e
		}
		return print(v)
	case "send":
		fs := flag.NewFlagSet("send", flag.ContinueOnError)
		fs.SetOutput(out)
		to := fs.String("to", "", "recipient")
		text := fs.String("text", "", "message")
		if e := fs.Parse(clean[1:]); e != nil {
			return e
		}
		return service.SendToAddress(ctx, *to, *text)
	case "doctor":
		fmt.Fprintln(out, "GalaxyTTY Doctor\n\n✓ mock adapters          ready\n✓ SMS Provider fixture   accessible\n✓ Virtual Display fake   supported\n\nConnection:\n  Mock\n\nReady.")
		return nil
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, strings.Join(clean, " "))
	}
}
