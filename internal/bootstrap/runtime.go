package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/app"
	hostclipboard "github.com/galaxytty/galaxytty/internal/clipboard"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/readonly"
	"github.com/galaxytty/galaxytty/internal/remote"
	"github.com/galaxytty/galaxytty/internal/samsung"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

const adbCommandTimeout = 5 * time.Second
const helperConnectTimeout = 8 * time.Second

const (
	scrcpyStartupTimeout   = 10 * time.Second
	conversationReadyDelay = 500 * time.Millisecond
	verificationInterval   = 300 * time.Millisecond
	samsungMessagesPackage = "com.samsung.android.messaging"
)

type Runtime struct {
	Service app.API
}

func Mock(cfg config.Config) (*Runtime, error) {
	backend := mock.New()
	notifier := &mock.Notifier{}
	lifecycle := app.NewLifecycle(&mock.Display{}, notifier)
	if err := lifecycle.Transition(app.Connecting); err != nil {
		return nil, err
	}
	if err := lifecycle.Transition(app.Ready); err != nil {
		return nil, err
	}
	service := app.NewService(
		backend,
		backend,
		notifier,
		lifecycle,
		app.NotificationPolicy{Enabled: cfg.Notifications.Enabled, ShowWhenFocused: cfg.Notifications.ShowWhenFocused},
		domain.ApplicationStatus{Label: "Mock Connected"},
	)
	if err := service.InitializePolling(context.Background()); err != nil {
		return nil, err
	}
	return &Runtime{Service: service}, nil
}

func Real(ctx context.Context, cfg config.Config, selector string) (*Runtime, error) {
	client, err := adb.NewClient("", adbCommandTimeout)
	if err != nil {
		return nil, err
	}
	return realWithBackend(ctx, cfg, selector, client)
}

// Helper assembles the v2 adapters below the existing application service.
// An empty address uses DNS-SD; a host:port value is a diagnostic fallback.
func Helper(ctx context.Context, cfg config.Config, address string) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	credentialStore, err := pairing.DefaultStore()
	if err != nil {
		return nil, fmt.Errorf("configure Galaxy Helper credential store: %w", err)
	}
	clientConfig := remote.Config{Address: address, Credentials: credentialStore}
	var client *remote.Client
	if address == "" {
		client, err = remote.NewDiscoveredClient(clientConfig, remote.NewDiscovery(4*time.Second))
	} else {
		client, err = remote.NewClient(clientConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("configure Galaxy Helper client: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	go func() { _ = client.Run(runContext) }()
	waitContext, waitCancel := context.WithTimeout(ctx, helperConnectTimeout)
	err = client.WaitConnected(waitContext)
	waitCancel()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect to Galaxy Helper: %w", err)
	}
	pingContext, pingCancel := context.WithTimeout(ctx, helperConnectTimeout)
	pong, err := client.Request(pingContext, protocol.TypePing, map[string]any{"probe": true})
	pingCancel()
	if err != nil || pong.Type != protocol.TypePong {
		cancel()
		if err == nil {
			err = fmt.Errorf("unexpected response %s", pong.Type)
		}
		return nil, fmt.Errorf("verify Galaxy Helper secure heartbeat: %w", err)
	}

	lifecycle := app.NewLifecycle(nil, readonly.Notifier{})
	lifecycle.BindCancel(cancel)
	if err := lifecycle.Transition(app.Connecting); err != nil {
		cancel()
		return nil, err
	}
	if err := lifecycle.Transition(app.Ready); err != nil {
		cancel()
		return nil, err
	}
	service := app.NewService(
		remote.NewStore(client),
		remote.NewSender(client),
		readonly.Notifier{},
		lifecycle,
		app.NotificationPolicy{Enabled: cfg.Notifications.Enabled, ShowWhenFocused: cfg.Notifications.ShowWhenFocused},
		client.Status(ctx),
	).WithStatusProvider(client).WithMessageEvents(remote.NewEventSource(client))
	return &Runtime{Service: service}, nil
}

func realWithBackend(ctx context.Context, cfg config.Config, selector string, backend adb.Backend) (*Runtime, error) {
	return realWithDependencies(ctx, cfg, selector, backend, realDependencies{})
}

type realDependencies struct {
	newDisplay    func(scrcpy.Config) (domain.VirtualDisplayManager, error)
	newClipboard  func() domain.Clipboard
	senderTimings func(config.Config) samsung.SenderConfig
}

func (d realDependencies) withDefaults() realDependencies {
	if d.newDisplay == nil {
		d.newDisplay = func(cfg scrcpy.Config) (domain.VirtualDisplayManager, error) {
			return scrcpy.NewManager(cfg)
		}
	}
	if d.newClipboard == nil {
		d.newClipboard = func() domain.Clipboard { return hostclipboard.New("", "") }
	}
	if d.senderTimings == nil {
		d.senderTimings = func(cfg config.Config) samsung.SenderConfig {
			return samsung.SenderConfig{
				ConversationReadyDelay: conversationReadyDelay,
				ClipboardSyncDelay:     cfg.Samsung.ClipboardSyncDelay.Duration,
				SendSettleDelay:        cfg.Samsung.SendSettleDelay.Duration,
				VerificationTimeout:    cfg.Samsung.VerificationTimeout.Duration,
				VerificationInterval:   verificationInterval,
				UseIntentBody:          true,
			}
		}
	}
	return d
}

func realWithDependencies(ctx context.Context, cfg config.Config, selector string, backend adb.Backend, deps realDependencies) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	deps = deps.withDefaults()
	if selector == "" {
		selector = cfg.Connection.Device
	}
	target, err := adb.Discover(ctx, backend, adb.SelectionOptions{
		PreferUSB: cfg.Connection.PreferUSB,
		Target:    selector,
	})
	if err != nil {
		return nil, err
	}
	store := provider.NewStore(target)
	layout := samsung.Layout{
		Width: cfg.Samsung.DisplayWidth, Height: cfg.Samsung.DisplayHeight,
		Composer: samsung.Point{X: cfg.Samsung.Layout.ComposerX, Y: cfg.Samsung.Layout.ComposerY},
		Send:     samsung.Point{X: cfg.Samsung.Layout.SendX, Y: cfg.Samsung.Layout.SendY},
	}
	controller, err := samsung.NewController(target, layout)
	if err != nil {
		return nil, fmt.Errorf("configure Samsung Messages controller: %w", err)
	}
	display, err := deps.newDisplay(scrcpy.Config{
		Target: target.Info().Serial, Package: samsungMessagesPackage,
		Width: cfg.Samsung.DisplayWidth, Height: cfg.Samsung.DisplayHeight,
		StartupTimeout: scrcpyStartupTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure virtual display: %w", err)
	}
	sender, err := samsung.NewSender(display, controller, deps.newClipboard(), store, deps.senderTimings(cfg))
	if err != nil {
		return nil, fmt.Errorf("configure Samsung Messages sender: %w", err)
	}
	notifier := readonly.Notifier{}
	lifecycle := app.NewLifecycle(display, notifier)
	if err := lifecycle.Transition(app.Connecting); err != nil {
		return nil, err
	}
	if err := lifecycle.Transition(app.Ready); err != nil {
		return nil, err
	}
	status := target.Status(ctx)
	service := app.NewService(
		store,
		sender,
		notifier,
		lifecycle,
		app.NotificationPolicy{Enabled: cfg.Notifications.Enabled, ShowWhenFocused: cfg.Notifications.ShowWhenFocused},
		status,
	).WithStatusProvider(target)
	if err := service.InitializePolling(ctx); err != nil {
		return nil, fmt.Errorf("initialize SMS polling: %w", err)
	}
	return &Runtime{Service: service}, nil
}
