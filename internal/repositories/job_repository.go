// Package repositories provides storage for domain entities.
package repositories

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"hls-downloader/internal/entities"
)

// maxFinishedJobs caps how many jobs are retained in memory before the
// oldest finished ones are evicted.
const maxFinishedJobs = 200

// JobRepository is a thread-safe in-memory store of download jobs.
type JobRepository struct {
	mu      sync.Mutex
	jobs    map[string]*entities.Job
	counter int
}

func NewJobRepository() *JobRepository {
	return &JobRepository{jobs: map[string]*entities.Job{}}
}

// Create assigns a new ID, stores the job as "queued" and evicts the
// oldest finished jobs when the store exceeds maxFinishedJobs.
func (r *JobRepository) Create(url, filename string) *entities.Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counter++
	j := &entities.Job{
		ID:        fmt.Sprintf("job-%d", r.counter),
		URL:       url,
		Filename:  filename,
		Status:    "queued",
		StartedAt: time.Now(),
	}
	r.jobs[j.ID] = j
	r.evictLocked()
	return j
}

// Status returns the current status of a job, or false if unknown.
func (r *JobRepository) Status(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return "", false
	}
	return j.Status, true
}

// List returns all known jobs.
func (r *JobRepository) List() []*entities.Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	list := make([]*entities.Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		list = append(list, j)
	}
	return list
}

// SetStatus updates a job's status and error message. The end time is
// recorded for terminal states (done, error, cancelled).
func (r *JobRepository) SetStatus(id, status, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	j, ok := r.jobs[id]
	if !ok {
		return
	}
	j.Status = status
	j.Error = errMsg
	if status != "queued" && status != "running" {
		j.EndedAt = time.Now()
	}
}

// evictLocked deletes the oldest finished jobs when the store exceeds
// maxFinishedJobs. Caller must hold r.mu. Running jobs are never evicted.
func (r *JobRepository) evictLocked() {
	if len(r.jobs) <= maxFinishedJobs {
		return
	}
	finished := make([]*entities.Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		if j.Status == "done" || j.Status == "error" || j.Status == "cancelled" {
			finished = append(finished, j)
		}
	}
	sort.Slice(finished, func(a, b int) bool {
		return finished[a].EndedAt.Before(finished[b].EndedAt)
	})
	for len(r.jobs) > maxFinishedJobs && len(finished) > 0 {
		delete(r.jobs, finished[0].ID)
		finished = finished[1:]
	}
}
