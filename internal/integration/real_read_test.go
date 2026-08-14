//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/doctor"
	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestRealGalaxyReadPath(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_READ_TEST") != "1" {
		t.Skip("explicit real read test opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := config.Default()
	report := doctor.Run(ctx, cfg, "")
	if !report.Ready {
		t.Fatalf("doctor not ready: %s", report.Summary)
	}
	runtime, err := bootstrap.Real(ctx, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Service.Shutdown(context.Background())

	conversations, err := runtime.Service.Conversations(ctx)
	if err != nil || len(conversations) == 0 {
		t.Fatalf("conversation count=%d err=%v", len(conversations), err)
	}
	messages, err := runtime.Service.Messages(ctx, conversations[0].ThreadID, domain.MessageQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.ThreadID != conversations[0].ThreadID || message.Type != domain.MessageSMS {
			t.Fatal("invalid structural SMS mapping")
		}
	}
	if _, err := runtime.Service.Poll(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Service.SendToAddress(ctx, "synthetic", "synthetic"); !errors.Is(err, domain.ErrSendingNotImplemented) {
		t.Fatalf("send err=%v", err)
	}
}
