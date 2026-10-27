package dynamic

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A caller whose context ends while another holds the content lock must return without
// running its change.
func TestMutateGivesUpWhenCancelled(t *testing.T) {
	e := &Engine{locks: map[string]chan struct{}{}}
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = e.Mutate(context.Background(), "c", func() error { close(held); <-release; return nil })
	}()
	<-held
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	ran := false
	err := e.Mutate(ctx, "c", func() error { ran = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || ran {
		t.Fatalf("err=%v ran=%v, want deadline exceeded without running", err, ran)
	}
}
