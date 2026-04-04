package indexer

import "context"

// Request represents a request to index a repository.
type Request struct {
	RepoDir string
	TaskID  string // non-empty for reindex API requests
	Repo    string // e.g. "github.com/org/repo", used to find matching mirror entry
}

// Queue is a dual-priority channel-based queue for index requests.
type Queue struct {
	high chan Request // cap: 16, from /api/reindex
	low  chan Request // cap: 64, from periodic mirror/fetch
}

// NewQueue creates a new index queue.
func NewQueue() *Queue {
	return &Queue{
		high: make(chan Request, 16),
		low:  make(chan Request, 64),
	}
}

// PushHigh adds a high-priority request. Returns false if the queue is full.
func (q *Queue) PushHigh(req Request) bool {
	select {
	case q.high <- req:
		return true
	default:
		return false
	}
}

// PushLow adds a low-priority request, dropping it silently if the queue is full.
func (q *Queue) PushLow(req Request) {
	select {
	case q.low <- req:
	default:
	}
}

// HighLen returns the current number of items in the high-priority queue.
func (q *Queue) HighLen() int { return len(q.high) }

// LowLen returns the current number of items in the low-priority queue.
func (q *Queue) LowLen() int { return len(q.low) }

// Next blocks until a request is available or ctx is cancelled.
// High-priority requests are preferred over low-priority ones.
func (q *Queue) Next(ctx context.Context) (Request, bool) {
	select {
	case req := <-q.high:
		return req, true
	default:
		select {
		case req := <-q.high:
			return req, true
		case req := <-q.low:
			return req, true
		case <-ctx.Done():
			return Request{}, false
		}
	}
}
