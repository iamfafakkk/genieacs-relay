package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Job status values surfaced by GET /jobs.
const (
	JobStatusQueued  = "queued"
	JobStatusRunning = "running"
	JobStatusSuccess = "success"
	JobStatusFailed  = "failed"
)

// MaxTrackedJobs bounds the in-memory job history (newest kept).
const MaxTrackedJobs = 200

// errWorkerQueueFull marks a job that never got a worker slot.
var errWorkerQueueFull = errors.New("worker pool queue full")

// Job is one unit of work handed to the worker pool. workerPool.Submit
// creates it; the worker mutates status as the task progresses.
type Job struct {
	ID             string     `json:"id"`
	Type           string     `json:"type"`
	DeviceID       string     `json:"device_id"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	ParameterCount int        `json:"parameter_count"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMs     int64      `json:"duration_ms"`
}

// jobRegistry keeps the most recent jobs in memory, newest first. It is
// deliberately process-local: the task queue itself is in-memory, so a
// restarted relay has no in-flight work to report.
//
// version increments on every mutation so a WebSocket subscriber can push
// only when something actually changed.
type jobRegistry struct {
	mu      sync.Mutex
	jobs    []*Job
	max     int
	version uint64
}

var jobSeq uint64

// jobPrefix is a random per-process token. The registry is in-memory and the
// counter resets on every restart, so IDs must not repeat across restarts —
// the admin panel remembers announced IDs in localStorage to avoid replaying
// history, and a reused "job-1" would be silently suppressed as already seen.
var jobPrefix = newJobPrefix()

func newJobPrefix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// jobRegistryInstance is the process-wide registry the HTTP handler reads and
// the worker pool writes to.
var jobRegistryInstance = newJobRegistry(MaxTrackedJobs)

func newJobRegistry(max int) *jobRegistry {
	return &jobRegistry{max: max}
}

// add records a new queued job and returns its generated ID.
func (r *jobRegistry) add(jobType, deviceID string, paramCount int) string {
	id := fmt.Sprintf("job-%s-%d", jobPrefix, atomic.AddUint64(&jobSeq, 1))
	j := &Job{
		ID:             id,
		Type:           jobType,
		DeviceID:       deviceID,
		Status:         JobStatusQueued,
		ParameterCount: paramCount,
		CreatedAt:      time.Now(),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append([]*Job{j}, r.jobs...)
	if len(r.jobs) > r.max {
		r.jobs = r.jobs[:r.max]
	}
	r.version++
	return id
}

// start marks a job as picked up by a worker.
func (r *jobRegistry) start(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j := r.find(id); j != nil {
		now := time.Now()
		j.Status = JobStatusRunning
		j.StartedAt = &now
		r.version++
	}
}

// finish marks a job terminal and records the total wall-clock duration.
func (r *jobRegistry) finish(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.find(id)
	if j == nil {
		return
	}
	now := time.Now()
	j.FinishedAt = &now
	j.DurationMs = now.Sub(j.CreatedAt).Milliseconds()
	if err != nil {
		j.Status = JobStatusFailed
		j.Error = err.Error()
	} else {
		j.Status = JobStatusSuccess
	}
	r.version++
}

// find returns the job with the given ID. Caller must hold r.mu.
func (r *jobRegistry) find(id string) *Job {
	for _, j := range r.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// list returns up to limit jobs (newest first) as copies, safe to marshal.
func (r *jobRegistry) list(limit int) []Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyLocked(limit)
}

// copyLocked returns up to limit jobs as copies. Caller must hold r.mu.
func (r *jobRegistry) copyLocked(limit int) []Job {
	n := len(r.jobs)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Job, 0, n)
	for _, j := range r.jobs[:n] {
		out = append(out, *j)
	}
	return out
}

// active counts jobs that are queued or running.
func (r *jobRegistry) active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeLocked()
}

// activeLocked counts queued/running jobs. Caller must hold r.mu.
func (r *jobRegistry) activeLocked() int {
	count := 0
	for _, j := range r.jobs {
		if j.Status == JobStatusQueued || j.Status == JobStatusRunning {
			count++
		}
	}
	return count
}

// snapshot returns a consistent view (jobs + counters + version) under a
// single lock acquisition, so a WebSocket push never mixes two moments.
func (r *jobRegistry) snapshot(limit int) (JobListResponse, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return JobListResponse{
		Jobs:   r.copyLocked(limit),
		Active: r.activeLocked(),
		Count:  len(r.jobs),
	}, r.version
}

// currentVersion returns just the mutation counter, without copying jobs.
func (r *jobRegistry) currentVersion() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version
}

// recordSyncJob records an already-completed synchronous operation in the
// registry so it appears in GET /jobs next to worker-pool jobs. The work
// has already happened by the time this is called, so the job is created
// and immediately finished (queued -> running -> success/failed) in one
// shot. Returns the job ID.
func recordSyncJob(jobType, deviceID string, paramCount int, err error) string {
	id := jobRegistryInstance.add(jobType, deviceID, paramCount)
	jobRegistryInstance.start(id)
	jobRegistryInstance.finish(id, err)
	return id
}

// JobListResponse is the GET /api/v1/genieacs/jobs payload.
//
// @Description Current in-memory worker jobs, newest first.
type JobListResponse struct {
	Jobs   []Job `json:"jobs"`
	Active int   `json:"active" example:"2"`
	Count  int   `json:"count" example:"37"`
}
