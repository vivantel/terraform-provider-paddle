package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// waitForEventContaining's retry loop is pure and testable with a stub
// lister — no real Paddle API needed to verify it terminates correctly
// on found/not-found/error.

func TestWaitForEventContaining_FoundOnFirstAttempt(t *testing.T) {
	calls := 0
	lister := func(_ context.Context, _ []string) ([]client.Event, error) {
		calls++
		return []client.Event{{ID: "evt_1", Data: []byte(`{"id":"pro_target"}`)}}, nil
	}
	found, err := waitForEventContaining(context.Background(), lister, "product.created", "pro_target", 5, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("found = false, want true")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should stop retrying once found)", calls)
	}
}

func TestWaitForEventContaining_FoundAfterRetries(t *testing.T) {
	calls := 0
	lister := func(_ context.Context, _ []string) ([]client.Event, error) {
		calls++
		if calls < 3 {
			return nil, nil
		}
		return []client.Event{{ID: "evt_1", Data: []byte(`{"id":"pro_target"}`)}}, nil
	}
	found, err := waitForEventContaining(context.Background(), lister, "product.created", "pro_target", 5, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("found = false, want true")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestWaitForEventContaining_NeverFound(t *testing.T) {
	calls := 0
	lister := func(_ context.Context, _ []string) ([]client.Event, error) {
		calls++
		return nil, nil
	}
	found, err := waitForEventContaining(context.Background(), lister, "product.created", "pro_target", 3, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (should exhaust all attempts)", calls)
	}
}

func TestWaitForEventContaining_ListerError(t *testing.T) {
	wantErr := errors.New("boom")
	lister := func(_ context.Context, _ []string) ([]client.Event, error) {
		return nil, wantErr
	}
	_, err := waitForEventContaining(context.Background(), lister, "product.created", "pro_target", 3, time.Millisecond)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
