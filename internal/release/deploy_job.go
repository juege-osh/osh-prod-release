package release

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type deployJob struct {
	releaseID     string
	target        string
	dispatchSince time.Time
	cancel        context.CancelFunc
	cancelled     atomic.Bool
}

type deployJobRegistry struct {
	mu        sync.Mutex
	job       *deployJob
	cancelled map[string]struct{}
}

func (r *deployJobRegistry) set(job *deployJob) {
	r.mu.Lock()
	r.job = job
	r.mu.Unlock()
}

func (r *deployJobRegistry) get(releaseID string) *deployJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job == nil || r.job.releaseID != releaseID {
		return nil
	}
	return r.job
}

func (r *deployJobRegistry) clear(releaseID string) {
	r.mu.Lock()
	if r.job != nil && r.job.releaseID == releaseID {
		r.job = nil
	}
	r.mu.Unlock()
}

func (r *deployJobRegistry) markDispatchSince(releaseID string, since time.Time) {
	r.mu.Lock()
	if r.job != nil && r.job.releaseID == releaseID {
		r.job.dispatchSince = since
	}
	r.mu.Unlock()
}

func (r *deployJobRegistry) markCancelled(releaseID string) {
	r.mu.Lock()
	if r.cancelled == nil {
		r.cancelled = map[string]struct{}{}
	}
	r.cancelled[releaseID] = struct{}{}
	r.mu.Unlock()
}

func (r *deployJobRegistry) isCancelled(releaseID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job != nil && r.job.releaseID == releaseID && r.job.cancelled.Load() {
		return true
	}
	_, ok := r.cancelled[releaseID]
	return ok
}

func (r *deployJobRegistry) clearCancelled(releaseID string) {
	r.mu.Lock()
	delete(r.cancelled, releaseID)
	r.mu.Unlock()
}

func (r *deployJobRegistry) stop(releaseID string) (*deployJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job == nil || r.job.releaseID != releaseID {
		r.markCancelledLocked(releaseID)
		return nil, false
	}
	j := r.job
	j.cancelled.Store(true)
	r.markCancelledLocked(releaseID)
	if j.cancel != nil {
		j.cancel()
	}
	return j, true
}

func (r *deployJobRegistry) markCancelledLocked(releaseID string) {
	if r.cancelled == nil {
		r.cancelled = map[string]struct{}{}
	}
	r.cancelled[releaseID] = struct{}{}
}
