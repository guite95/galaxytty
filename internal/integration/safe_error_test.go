//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestSafeProviderDiagnosticPreservesKnownErrorClass(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "permission", err: fmt.Errorf("query failed: %w", domain.ErrProviderPermissionDenied), want: domain.ErrProviderPermissionDenied.Error()},
		{name: "provider output", err: fmt.Errorf("query failed: %w", domain.ErrProviderOutput), want: domain.ErrProviderOutput.Error()},
		{name: "offline", err: fmt.Errorf("query failed: %w", domain.ErrOffline), want: domain.ErrOffline.Error()},
		{name: "unauthorized", err: fmt.Errorf("query failed: %w", domain.ErrUnauthorized), want: domain.ErrUnauthorized.Error()},
		{name: "canceled", err: fmt.Errorf("query failed: %w", context.Canceled), want: context.Canceled.Error()},
		{name: "deadline", err: fmt.Errorf("query failed: %w", context.DeadlineExceeded), want: context.DeadlineExceeded.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeProviderDiagnostic(tc.err); got != tc.want {
				t.Fatalf("diagnostic=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestSafeProviderDiagnosticRedactsUnknownDetails(t *testing.T) {
	privateDetail := "private-target private-provider-stderr"
	got := safeProviderDiagnostic(errors.New(privateDetail))
	if got != "provider query failed" {
		t.Fatalf("diagnostic=%q", got)
	}
}
