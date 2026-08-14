package samsung

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const cleanupTimeout = 2 * time.Second

type MessageController interface {
	domain.ConversationController
	OpenConversationWithBody(context.Context, domain.VirtualDisplay, string, string) error
	MainDisplayOff(context.Context) (bool, error)
	WakeVirtualDisplay(context.Context, domain.VirtualDisplay) error
	ShowHome(context.Context, domain.VirtualDisplay) error
	SleepMainDisplay(context.Context) error
	FocusComposer(context.Context, domain.VirtualDisplay) error
	ClearComposer(context.Context, domain.VirtualDisplay) error
	Paste(context.Context, domain.VirtualDisplay) error
	TapSend(context.Context, domain.VirtualDisplay) error
}

type SenderConfig struct {
	ConversationReadyDelay time.Duration
	ClipboardSyncDelay     time.Duration
	SendSettleDelay        time.Duration
	VerificationTimeout    time.Duration
	VerificationInterval   time.Duration
	InputMode              domain.TextInputMode
}

func (c SenderConfig) validate() error {
	if c.ConversationReadyDelay <= 0 || c.ClipboardSyncDelay <= 0 || c.SendSettleDelay <= 0 || c.VerificationTimeout <= 0 || c.VerificationInterval <= 0 {
		return fmt.Errorf("invalid Samsung sender timing configuration")
	}
	if !c.InputMode.Valid() {
		return fmt.Errorf("invalid Samsung text input mode")
	}
	return nil
}

type Sender struct {
	display    domain.VirtualDisplayManager
	controller MessageController
	clipboard  domain.Clipboard
	store      domain.MessageStore
	config     SenderConfig
	wait       func(context.Context, time.Duration) error

	mu sync.Mutex
}

func NewSender(
	display domain.VirtualDisplayManager,
	controller MessageController,
	clipboard domain.Clipboard,
	store domain.MessageStore,
	config SenderConfig,
) (*Sender, error) {
	if display == nil || controller == nil || clipboard == nil || store == nil {
		return nil, fmt.Errorf("Samsung sender dependencies are required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Sender{
		display: display, controller: controller, clipboard: clipboard, store: store,
		config: config, wait: waitContext,
	}, nil
}

func (s *Sender) Send(ctx context.Context, phone, text string) (result domain.SendResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedPhone := domain.NormalizePhone(phone)
	if normalizedPhone == "" {
		return domain.SendResult{}, fmt.Errorf("recipient is required")
	}
	if strings.TrimSpace(text) == "" {
		return domain.SendResult{}, fmt.Errorf("message text is required")
	}

	display, err := s.display.Start(ctx)
	if err != nil {
		return domain.SendResult{}, fmt.Errorf("start virtual display: %w", err)
	}
	mainWasOff, err := s.controller.MainDisplayOff(ctx)
	if err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if mainWasOff {
		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			if restoreErr := s.controller.SleepMainDisplay(restoreCtx); restoreErr != nil {
				restoreErr = fmt.Errorf("restore main display power: %w", restoreErr)
				if err == nil {
					err = restoreErr
				} else {
					err = errors.Join(err, restoreErr)
				}
			}
		}()
		if err := s.controller.WakeVirtualDisplay(ctx, display); err != nil {
			return domain.SendResult{}, s.controllerError(err)
		}
	}
	if s.config.InputMode == domain.TextInputIntentBody {
		if err := s.controller.OpenConversationWithBody(ctx, display, normalizedPhone, text); err != nil {
			return domain.SendResult{}, s.controllerError(err)
		}
		if err := s.wait(ctx, s.config.ConversationReadyDelay); err != nil {
			return domain.SendResult{}, fmt.Errorf("wait for Samsung Messages conversation: %w", err)
		}
		if err := s.controller.FocusComposer(ctx, display); err != nil {
			return domain.SendResult{}, s.controllerError(err)
		}
		if err := s.wait(ctx, s.config.SendSettleDelay); err != nil {
			return domain.SendResult{}, fmt.Errorf("wait for Samsung Messages composer: %w", err)
		}
		return s.tapAndVerify(ctx, display, normalizedPhone, text)
	}
	if err := s.controller.ShowHome(ctx, display); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}

	oldClipboard, err := s.clipboard.Read(ctx)
	if err != nil {
		return domain.SendResult{}, err
	}
	if err := s.clipboard.Set(ctx, text); err != nil {
		return domain.SendResult{}, err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if restoreErr := s.clipboard.Set(restoreCtx, oldClipboard); restoreErr != nil {
			restoreErr = fmt.Errorf("restore host clipboard: %w", restoreErr)
			if err == nil {
				err = restoreErr
			} else {
				err = errors.Join(err, restoreErr)
			}
		}
	}()

	if err := s.display.SyncClipboard(ctx); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if err := s.wait(ctx, s.config.ClipboardSyncDelay); err != nil {
		return domain.SendResult{}, fmt.Errorf("wait for clipboard synchronization: %w", err)
	}
	if err := s.controller.OpenConversation(ctx, display, phone); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if err := s.wait(ctx, s.config.ConversationReadyDelay); err != nil {
		return domain.SendResult{}, fmt.Errorf("wait for Samsung Messages conversation: %w", err)
	}
	if err := s.controller.FocusComposer(ctx, display); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if err := s.controller.ClearComposer(ctx, display); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if err := s.controller.Paste(ctx, display); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	if err := s.wait(ctx, s.config.SendSettleDelay); err != nil {
		return domain.SendResult{}, fmt.Errorf("wait for Samsung Messages composer: %w", err)
	}

	return s.tapAndVerify(ctx, display, normalizedPhone, text)
}

func (s *Sender) tapAndVerify(ctx context.Context, display domain.VirtualDisplay, phone, text string) (domain.SendResult, error) {
	baseline, err := s.store.LatestMessageID(ctx)
	if err != nil {
		return domain.SendResult{}, s.maybeDisconnectError(err)
	}
	if err := s.controller.TapSend(ctx, display); err != nil {
		return domain.SendResult{}, s.controllerError(err)
	}
	result, err := verifySent(ctx, s.store, baseline, phone, text, s.config.VerificationTimeout, s.config.VerificationInterval)
	if err != nil {
		return domain.SendResult{}, s.maybeDisconnectError(err)
	}
	return result, nil
}

func (s *Sender) controllerError(primary error) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if stopErr := s.display.Stop(cleanupCtx); stopErr != nil {
		return errors.Join(primary, fmt.Errorf("stop unhealthy virtual display: %w", stopErr))
	}
	return primary
}

func (s *Sender) maybeDisconnectError(primary error) error {
	if !errors.Is(primary, domain.ErrOffline) && !errors.Is(primary, domain.ErrNoDevices) && !errors.Is(primary, domain.ErrUnauthorized) {
		return primary
	}
	return s.controllerError(primary)
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var _ domain.MessageSender = (*Sender)(nil)
