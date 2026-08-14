package readonly

import (
	"context"
	"errors"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestSenderAlwaysRejects(t *testing.T) {
	result, err := (Sender{}).Send(context.Background(), "synthetic", "synthetic")
	if !errors.Is(err, domain.ErrSendingNotImplemented) {
		t.Fatalf("err=%v", err)
	}
	if result != (domain.SendResult{}) {
		t.Fatalf("result=%+v", result)
	}
}
