package testutils

import (
	"context"
	"testing"
	"time"
)

var (
	ConnectTimeout = 30 * time.Second
)

func WithTimeout(t *testing.T, f func() string) {
	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
	defer cancel()
	lastErr := ""
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out: " + lastErr)
		case <-time.After(10 * time.Millisecond):
			lastErr = f()
			if lastErr == "" {
				return
			}
		}
	}
}
