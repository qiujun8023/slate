package job

import (
	"context"
	"errors"
	"testing"
)

func TestRunOnceReturnsJobError(t *testing.T) {
	want := errors.New("push failed")
	jobs["test-fail"] = Job{Run: func(context.Context) error { return want }}
	jobs["test-ok"] = Job{Run: func(context.Context) error { return nil }}
	t.Cleanup(func() { delete(jobs, "test-fail"); delete(jobs, "test-ok") })
	t.Setenv("SLATE_JOB_RUN_ONCE", "1")

	if err := Run(t.Context(), "test-fail"); !errors.Is(err, want) {
		t.Fatalf("Run(test-fail) = %v, want %v", err, want)
	}
	if err := Run(t.Context(), "test-ok"); err != nil {
		t.Fatalf("Run(test-ok) = %v", err)
	}
}
