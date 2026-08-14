package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
)

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

type syntheticADB struct{}

func (syntheticADB) Devices(context.Context) ([]adb.Device, error) {
	return []adb.Device{{Target: "USB123", State: "device", Connection: domain.ConnectionUSB}}, nil
}

func (syntheticADB) Shell(_ context.Context, target string, args ...string) ([]byte, error) {
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
	}
	if len(args) > 0 && args[0] == "content" {
		return []byte("No result found.\n"), nil
	}
	return nil, fmt.Errorf("unexpected shell args %q", args)
}

func TestRealBuildsReadOnlyApplicationAndInitializesPolling(t *testing.T) {
	runtime, err := realWithBackend(context.Background(), config.Default(), "", syntheticADB{})
	if err != nil {
		t.Fatal(err)
	}
	status := runtime.Service.Status(context.Background())
	if status.Label != "USB" || status.Connection != domain.ConnectionUSB {
		t.Fatalf("status=%+v", status)
	}
	if _, err := runtime.Service.SendToAddress(context.Background(), "synthetic", "synthetic"); !errors.Is(err, domain.ErrSendingNotImplemented) {
		t.Fatalf("send err=%v", err)
	}
	if messages, err := runtime.Service.Poll(context.Background(), 0); err != nil || len(messages) != 0 {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	if err := runtime.Service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
