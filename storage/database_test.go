package storage

import (
	"context"
	"testing"
	"time"
)

func TestDbCtxTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond)

	select {
	case <-ctx.Done():
	default:
		t.Error("expected context to be done after timeout")
	}
}
