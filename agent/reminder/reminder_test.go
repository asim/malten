package reminder

import (
	"context"
	"testing"
	"time"
)

func TestReminderStopsWithServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, func(context.Context, string, string, string, ...string) error {
			t.Error("published after shutdown")
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent did not stop")
	}
}
