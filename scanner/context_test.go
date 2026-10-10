package scanner

import (
	"context"
	"testing"

	"piyazche/config"
)

func TestCancelledContextPreventsWarmup(t *testing.T) {
	s := NewScanner(config.DefaultConfig())
	s.SetIPs([]string{"127.0.0.1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.RunWithContext(ctx); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	if s.GetResults().Count() != 0 {
		t.Fatal("cancelled scanner performed work")
	}
}
