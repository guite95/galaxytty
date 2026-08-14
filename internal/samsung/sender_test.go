package samsung

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type senderDisplay struct {
	calls *[]string
	err   error
	stops int
}

func (d *senderDisplay) Start(context.Context) (domain.VirtualDisplay, error) {
	*d.calls = append(*d.calls, "display.start")
	return domain.VirtualDisplay{AndroidDisplayID: 18, Width: 1080, Height: 1920}, d.err
}
func (d *senderDisplay) Stop(context.Context) error {
	*d.calls = append(*d.calls, "display.stop")
	d.stops++
	return nil
}
func (d *senderDisplay) Healthy(context.Context) bool { return d.err == nil }

type senderController struct {
	calls  *[]string
	failAt string
	err    error
}

func (c *senderController) call(name string) error {
	*c.calls = append(*c.calls, name)
	if c.failAt == name {
		return c.err
	}
	return nil
}
func (c *senderController) OpenConversation(context.Context, domain.VirtualDisplay, string) error {
	return c.call("conversation.open")
}
func (c *senderController) FocusComposer(context.Context, domain.VirtualDisplay) error {
	return c.call("composer.tap")
}
func (c *senderController) Paste(context.Context, domain.VirtualDisplay) error {
	return c.call("paste")
}
func (c *senderController) TapSend(context.Context, domain.VirtualDisplay) error {
	return c.call("send.tap")
}

type senderClipboard struct {
	calls    *[]string
	old      string
	readErr  error
	setError map[int]error
	sets     int
}

func (c *senderClipboard) Read(context.Context) (string, error) {
	*c.calls = append(*c.calls, "clipboard.read")
	return c.old, c.readErr
}
func (c *senderClipboard) Set(_ context.Context, value string) error {
	c.sets++
	if c.sets == 1 {
		*c.calls = append(*c.calls, "clipboard.set")
	} else {
		*c.calls = append(*c.calls, "clipboard.restore")
	}
	return c.setError[c.sets]
}

type senderStore struct {
	calls     *[]string
	latest    int64
	latestErr error
	after     []domain.Message
	afterErr  error
}

func (s *senderStore) Conversations(context.Context) ([]domain.Conversation, error) { return nil, nil }
func (s *senderStore) Messages(context.Context, int64, domain.MessageQuery) ([]domain.Message, error) {
	return nil, nil
}
func (s *senderStore) LatestMessageID(context.Context) (int64, error) {
	*s.calls = append(*s.calls, "store.latest")
	return s.latest, s.latestErr
}
func (s *senderStore) MessagesAfter(context.Context, int64) ([]domain.Message, error) {
	*s.calls = append(*s.calls, "store.after")
	return s.after, s.afterErr
}

func senderFixture(t *testing.T) (*Sender, *[]string, *senderDisplay, *senderController, *senderClipboard, *senderStore) {
	t.Helper()
	calls := []string{}
	display := &senderDisplay{calls: &calls}
	controller := &senderController{calls: &calls, err: errors.New("synthetic controller failure")}
	clip := &senderClipboard{calls: &calls, old: "old clipboard", setError: map[int]error{}}
	store := &senderStore{
		calls:  &calls,
		latest: 100,
		after:  []domain.Message{{ID: 101, ThreadID: 49, Address: "+82 10-1234-5678", Body: "안녕하세요 😀", Direction: domain.DirectionOutgoing}},
	}
	sender, err := NewSender(display, controller, clip, store, SenderConfig{
		ConversationReadyDelay: time.Millisecond,
		ClipboardSyncDelay:     2 * time.Millisecond,
		SendSettleDelay:        3 * time.Millisecond,
		VerificationTimeout:    50 * time.Millisecond,
		VerificationInterval:   time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	sender.wait = func(_ context.Context, delay time.Duration) error {
		switch delay {
		case time.Millisecond:
			calls = append(calls, "wait.ready")
		case 2 * time.Millisecond:
			calls = append(calls, "wait.sync")
		case 3 * time.Millisecond:
			calls = append(calls, "wait.settle")
		default:
			t.Fatalf("unexpected wait %s", delay)
		}
		return nil
	}
	return sender, &calls, display, controller, clip, store
}

func TestSenderRunsVerifiedSequenceAndRestoresClipboard(t *testing.T) {
	sender, calls, _, _, _, _ := senderFixture(t)
	result, err := sender.Send(context.Background(), "01012345678", "안녕하세요 😀")
	if err != nil {
		t.Fatal(err)
	}
	if result != (domain.SendResult{MessageID: 101, ThreadID: 49}) {
		t.Fatalf("result=%+v", result)
	}
	want := []string{
		"display.start", "conversation.open", "wait.ready", "composer.tap",
		"clipboard.read", "clipboard.set", "wait.sync", "paste", "wait.settle",
		"store.latest", "send.tap", "store.after", "clipboard.restore",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls=%q want=%q", *calls, want)
	}
}

func TestSenderStopsDisplayAfterControllerFailure(t *testing.T) {
	sender, calls, display, controller, _, _ := senderFixture(t)
	controller.failAt = "paste"
	_, err := sender.Send(context.Background(), "01012345678", "안녕하세요 😀")
	if !errors.Is(err, controller.err) || display.stops != 1 {
		t.Fatalf("err=%v stops=%d", err, display.stops)
	}
	want := []string{
		"display.start", "conversation.open", "wait.ready", "composer.tap",
		"clipboard.read", "clipboard.set", "wait.sync", "paste", "display.stop", "clipboard.restore",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls=%q want=%q", *calls, want)
	}
}

func TestSenderStopsAtClipboardAndBaselineFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*senderClipboard, *senderStore)
		wantLast  string
	}{
		{"read", func(c *senderClipboard, _ *senderStore) { c.readErr = errors.New("read failed") }, "clipboard.read"},
		{"set", func(c *senderClipboard, _ *senderStore) { c.setError[1] = errors.New("set failed") }, "clipboard.set"},
		{"baseline", func(_ *senderClipboard, s *senderStore) { s.latestErr = errors.New("latest failed") }, "clipboard.restore"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender, calls, _, _, clip, store := senderFixture(t)
			tc.configure(clip, store)
			if _, err := sender.Send(context.Background(), "01012345678", "안녕하세요 😀"); err == nil {
				t.Fatal("expected send error")
			}
			if got := (*calls)[len(*calls)-1]; got != tc.wantLast {
				t.Fatalf("last call=%q calls=%q", got, *calls)
			}
			for _, forbidden := range []string{"send.tap", "store.after"} {
				if containsCall(*calls, forbidden) {
					t.Fatalf("unexpected %s after failure: %q", forbidden, *calls)
				}
			}
		})
	}
}

func TestSenderJoinsPrimaryAndClipboardRestoreErrorsWithoutPrivateValues(t *testing.T) {
	const recipient = "01012345678"
	const body = "private message body"
	sender, _, _, controller, clip, store := senderFixture(t)
	controller.failAt = "send.tap"
	clip.setError[2] = errors.New("restore failed")
	store.after[0].Body = body
	_, err := sender.Send(context.Background(), recipient, body)
	if !errors.Is(err, controller.err) || !errors.Is(err, clip.setError[2]) {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), recipient) || strings.Contains(err.Error(), body) {
		t.Fatalf("send error leaked private values: %q", err)
	}
}

func TestSenderSerializesClipboardTransactions(t *testing.T) {
	sender, _, _, _, _, _ := senderFixture(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	sender.wait = func(_ context.Context, delay time.Duration) error {
		if delay == time.Millisecond {
			entered <- struct{}{}
			<-release
		}
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = sender.Send(context.Background(), "01012345678", "안녕하세요 😀")
	}()
	<-entered
	go func() {
		defer wg.Done()
		_, _ = sender.Send(context.Background(), "01012345678", "안녕하세요 😀")
	}()
	select {
	case <-entered:
		t.Fatal("second send entered while first transaction was active")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	wg.Wait()
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}
