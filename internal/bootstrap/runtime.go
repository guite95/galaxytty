package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/readonly"
)

const adbCommandTimeout = 5 * time.Second

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

func realWithBackend(ctx context.Context, cfg config.Config, selector string, backend adb.Backend) (*Runtime, error) {
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
	notifier := readonly.Notifier{}
	lifecycle := app.NewLifecycle(nil, notifier)
	if err := lifecycle.Transition(app.Connecting); err != nil {
		return nil, err
	}
	if err := lifecycle.Transition(app.Ready); err != nil {
		return nil, err
	}
	status := target.Status(ctx)
	service := app.NewService(
		provider.NewStore(target),
		readonly.Sender{},
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
