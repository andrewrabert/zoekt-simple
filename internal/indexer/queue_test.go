package indexer

import (
	"context"
	"testing"
)

func TestQueueHighPriorityFirst(t *testing.T) {
	q := NewQueue()
	q.PushLow(Request{RepoDir: "low1"})
	q.PushLow(Request{RepoDir: "low2"})
	if !q.PushHigh(Request{RepoDir: "high1"}) {
		t.Fatal("PushHigh returned false on empty high channel")
	}

	ctx := context.Background()
	req, ok := q.Next(ctx)
	if !ok || req.RepoDir != "high1" {
		t.Fatalf("expected high1 first, got %v (ok=%v)", req.RepoDir, ok)
	}
}

func TestQueueLowPriorityWhenNoHigh(t *testing.T) {
	q := NewQueue()
	q.PushLow(Request{RepoDir: "low1"})

	ctx := context.Background()
	req, ok := q.Next(ctx)
	if !ok || req.RepoDir != "low1" {
		t.Fatalf("expected low1, got %v (ok=%v)", req.RepoDir, ok)
	}
}

func TestQueueContextCancellation(t *testing.T) {
	q := NewQueue()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := q.Next(ctx)
	if ok {
		t.Fatal("expected ok=false on cancelled context")
	}
}

func TestQueuePushHighFullReturns503(t *testing.T) {
	q := NewQueue()
	for i := range 16 {
		q.PushHigh(Request{RepoDir: string(rune('a' + i))})
	}
	if q.PushHigh(Request{RepoDir: "overflow"}) {
		t.Fatal("PushHigh should return false when full")
	}
}
