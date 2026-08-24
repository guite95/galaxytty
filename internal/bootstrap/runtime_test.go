package bootstrap

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/protocol"
	"github.com/galaxytty/galaxytty/internal/samsung"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

func TestHelperWaitsForHelloAndOwnsClientLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		hello, err := protocol.NewEnvelope(protocol.TypeHello, "", 0, protocol.HelloPayload{
			DeviceID: "synthetic", DeviceName: "Galaxy Test",
		})
		if err != nil {
			serverDone <- err
			return
		}
		if err := protocol.WriteFrame(conn, hello); err != nil {
			serverDone <- err
			return
		}
		for {
			envelope, err := protocol.ReadFrame(conn)
			if err != nil {
				serverDone <- err
				return
			}
			if envelope.Type != protocol.TypePing {
				continue
			}
			pong, err := protocol.NewEnvelope(protocol.TypePong, envelope.RequestID, 0, map[string]any{"ok": true})
			if err != nil {
				serverDone <- err
				return
			}
			if err := protocol.WriteFrame(conn, pong); err != nil {
				serverDone <- err
				return
			}
		}
	}()

	runtime, err := Helper(context.Background(), config.Default(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	status := runtime.Service.Status(context.Background())
	if status.Device != "Galaxy Test" || status.Connection != domain.ConnectionWireless {
		t.Fatalf("status=%+v", status)
	}
	if messages, failures := runtime.Service.SubscribeMessages(context.Background()); messages == nil || failures == nil {
		t.Fatal("helper runtime did not expose real-time event streams")
	}
	if err := runtime.Service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverDone:
		if err == nil || !strings.Contains(err.Error(), "EOF") {
			t.Fatalf("server error after shutdown=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("helper client connection remained open after shutdown")
	}
}

func TestMockBuildsApplicationWithThreadAwareSending(t *testing.T) {
	runtime, err := Mock(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := runtime.Service.SendToConversation(ctx, 2, "synthetic hello"); err != nil {
		t.Fatal(err)
	}
	messages, err := runtime.Service.Messages(ctx, 2, domain.MessageQuery{})
	if err != nil || messages[len(messages)-1].ThreadID != 2 {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
}

type syntheticADB struct {
	mu     sync.Mutex
	sent   bool
	wakes  int
	sleeps int
}

func (*syntheticADB) Devices(context.Context) ([]adb.Device, error) {
	return []adb.Device{{Target: "USB123", State: "device", Connection: domain.ConnectionUSB}}, nil
}

func (b *syntheticADB) Shell(_ context.Context, target string, args ...string) ([]byte, error) {
	if target != "USB123" {
		return nil, fmt.Errorf("unexpected target %q", target)
	}
	key := strings.Join(args, " ")
	switch key {
	case "getprop ro.product.manufacturer":
		return []byte("samsung\n"), nil
	case "getprop ro.product.model":
		return []byte("SM-A376N\n"), nil
	case "getprop ro.serialno":
		return []byte("HARDWARE123\n"), nil
	case "pm path com.samsung.android.messaging":
		return []byte("package:/synthetic/base.apk\n"), nil
	case "dumpsys display":
		return []byte("Display Id=0\n  Display State=OFF\nDisplay Id=18\n  Display State=ON\n"), nil
	}
	if len(args) > 0 && args[0] == "content" {
		projection := argumentAfter(args, "--projection")
		where := argumentAfter(args, "--where")
		if projection == "_id" {
			return []byte("Row: 0 _id=100\n"), nil
		}
		b.mu.Lock()
		sent := b.sent
		b.mu.Unlock()
		if strings.Contains(where, "_id > 100") && sent {
			return []byte("Row: 0 _id=101, thread_id=49, address=01012345678, date=1700000000000, type=2, read=1, body=synthetic hello\n"), nil
		}
		return []byte("No result found.\n"), nil
	}
	if len(args) >= 6 && args[0] == "input" && args[3] == "tap" && args[4] == "1004" && args[5] == "955" {
		b.mu.Lock()
		b.sent = true
		b.mu.Unlock()
		return nil, nil
	}
	if key == "input -d 18 keyevent 224" {
		b.mu.Lock()
		b.wakes++
		b.mu.Unlock()
		return nil, nil
	}
	if key == "input -d 0 keyevent 223" {
		b.mu.Lock()
		b.sleeps++
		b.mu.Unlock()
		return nil, nil
	}
	if len(args) > 0 && (args[0] == "am" || args[0] == "input") {
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected shell args %q", args)
}

type lazyDisplay struct {
	starts int
	stops  int
}

func (d *lazyDisplay) Start(context.Context) (domain.VirtualDisplay, error) {
	d.starts++
	return domain.VirtualDisplay{AndroidDisplayID: 18, Width: 1080, Height: 1920}, nil
}
func (d *lazyDisplay) SyncClipboard(context.Context) error { return nil }
func (d *lazyDisplay) Stop(context.Context) error          { d.stops++; return nil }
func (d *lazyDisplay) Healthy(context.Context) bool        { return d.starts > d.stops }

type runtimeClipboard struct{ value string }

func (c *runtimeClipboard) Read(context.Context) (string, error)      { return c.value, nil }
func (c *runtimeClipboard) Set(_ context.Context, value string) error { c.value = value; return nil }

func TestRealBuildsLazyVerifiedSenderAndInitializesPolling(t *testing.T) {
	display := &lazyDisplay{}
	backend := &syntheticADB{}
	cfg := config.Default()
	runtime, err := realWithDependencies(context.Background(), cfg, "", backend, realDependencies{
		newDisplay:   func(scrcpy.Config) (domain.VirtualDisplayManager, error) { return display, nil },
		newClipboard: func() domain.Clipboard { return &runtimeClipboard{value: "old"} },
		senderTimings: func(config.Config) samsung.SenderConfig {
			return samsung.SenderConfig{
				ConversationReadyDelay: time.Nanosecond,
				ClipboardSyncDelay:     time.Nanosecond,
				SendSettleDelay:        time.Nanosecond,
				VerificationTimeout:    time.Second,
				VerificationInterval:   time.Millisecond,
				UseIntentBody:          true,
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if display.starts != 0 {
		t.Fatal("real runtime eagerly started scrcpy display")
	}
	status := runtime.Service.Status(context.Background())
	if status.Label != "USB" || status.Connection != domain.ConnectionUSB {
		t.Fatalf("status=%+v", status)
	}
	result, err := runtime.Service.SendToAddress(context.Background(), "01012345678", "synthetic hello")
	if err != nil || result != (domain.SendResult{MessageID: 101, ThreadID: 49}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if display.starts != 1 {
		t.Fatalf("display starts=%d", display.starts)
	}
	backend.mu.Lock()
	wakes, sleeps := backend.wakes, backend.sleeps
	backend.mu.Unlock()
	if wakes != 1 || sleeps != 1 {
		t.Fatalf("power lifecycle wakes=%d sleeps=%d", wakes, sleeps)
	}
	if messages, err := runtime.Service.Poll(context.Background(), 0); err != nil || len(messages) != 1 || messages[0].Direction != domain.DirectionOutgoing {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	if messages, err := runtime.Service.Poll(context.Background(), 0); err != nil || len(messages) != 0 {
		t.Fatalf("second poll messages=%+v err=%v", messages, err)
	}
	if err := runtime.Service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Service.Shutdown(context.Background()); err != nil || display.stops != 1 {
		t.Fatalf("second shutdown err=%v stops=%d", err, display.stops)
	}
}

func argumentAfter(args []string, name string) string {
	for index := range args {
		if args[index] == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
