//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/pairing"
	"github.com/galaxytty/galaxytty/internal/protocol"
	"github.com/galaxytty/galaxytty/internal/remote"
)

func TestRealHelperSequenceSyncReadOnly(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_DEVICE_TEST") != "1" {
		t.Skip("explicit read-only Helper device test opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	credentials, err := pairing.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	client, err := remote.NewDiscoveredClient(
		remote.Config{Credentials: credentials},
		remote.NewDiscovery(4*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	runContext, stop := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(runContext) }()
	defer func() {
		stop()
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("Helper client shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Helper client did not stop")
		}
	}()
	if err := client.WaitConnected(ctx); err != nil {
		t.Fatal(err)
	}
	response, err := client.Request(ctx, protocol.TypeSyncRequest, map[string]any{
		"fromSequence":    1,
		"throughSequence": 1,
		"afterMessageId":  int64(math.MaxInt64),
		"limit":           1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Type != protocol.TypeSyncMessage {
		t.Fatalf("response type=%s", response.Type)
	}
	var payload struct {
		FromSequence    uint64            `json:"fromSequence"`
		ThroughSequence uint64            `json:"throughSequence"`
		Complete        bool              `json:"complete"`
		Items           []json.RawMessage `json:"items"`
	}
	if err := response.DecodePayload(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.FromSequence != 1 || payload.ThroughSequence != 1 || len(payload.Items) > 1 {
		t.Fatalf("invalid structural sync response: range=%d-%d count=%d", payload.FromSequence, payload.ThroughSequence, len(payload.Items))
	}
	t.Logf("read-only sequence sync complete=%t item_count=%d", payload.Complete, len(payload.Items))
}
