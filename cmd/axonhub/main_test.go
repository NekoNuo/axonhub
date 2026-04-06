package main

import (
	"context"
	"testing"
	"time"
)

func TestRunStartupCleanupAsync_DetachesFromStartupContext(t *testing.T) {
	t.Parallel()

	parentCtx, cancelParent := context.WithCancel(context.Background())
	started := make(chan context.Context, 1)
	unblock := make(chan struct{})
	returned := make(chan struct{})

	go func() {
		runStartupCleanupAsync(parentCtx, func(ctx context.Context) error {
			started <- ctx
			<-unblock
			return nil
		})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("expected startup cleanup launcher to return immediately")
	}

	cancelParent()

	var cleanupCtx context.Context
	select {
	case cleanupCtx = <-started:
	case <-time.After(time.Second):
		t.Fatal("expected startup cleanup to run asynchronously")
	}

	if cleanupCtx.Err() != nil {
		t.Fatalf("expected detached cleanup context, got error: %v", cleanupCtx.Err())
	}

	if _, ok := cleanupCtx.Deadline(); !ok {
		t.Fatal("expected cleanup context to have its own timeout")
	}

	close(unblock)
}
