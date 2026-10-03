package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WorkerTaskTimeout is the maximum time allowed for a single worker task
const WorkerTaskTimeout = 30 * time.Second

// workerPool manages a pool of worker goroutines for asynchronous task processing
type workerPool struct {
	workers int            // Number of active workers
	queue   chan task      // Channel for receiving tasks
	wg      sync.WaitGroup // WaitGroup for graceful shutdown synchronization
	once    sync.Once      // Ensure Start is only called once
}

// task represents a unit of work to be processed by the worker pool
type task struct {
	jobID    string          // Registry ID tracked by GET /jobs
	deviceID string          // Target device identifier for the task
	taskType string          // Type of task to execute (see taskType constants)
	params   [][]interface{} // Parameters for parameter-setting tasks
	paths    []string        // Parameter paths for getParameterValues tasks (wake)
}

// Start initializes the worker pool by launching all worker goroutines
func (wp *workerPool) Start() {
	// Create a specified number of worker goroutines
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)   // Increment WaitGroup counter
		go wp.worker() // Start a worker goroutine
	}
}

// Stop gracefully shuts down the worker pool by closing queue and waiting for completion
func (wp *workerPool) Stop() {
	wp.once.Do(func() { // Ensure Stop is only executed once
		close(wp.queue) // Close the task queue to signal workers to stop
		wp.wg.Wait()    // Wait for all workers to finish processing
	})
}

// worker is the goroutine that processes tasks from the queue
func (wp *workerPool) worker() {
	defer wp.wg.Done() // Signal completion when goroutine exits

	// Process tasks from queue until channel is closed
	for t := range wp.queue {
		// Create context with timeout for each task to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), WorkerTaskTimeout)

		if t.jobID != "" {
			jobRegistryInstance.start(t.jobID)
		}

		var err error
		// Execute appropriate function based on task type
		switch t.taskType {
		case taskTypeSetParams:
			err = setParameterValues(ctx, t.deviceID, t.params)
		case taskTypeApplyChanges, taskTypeRefreshWLAN:
			err = refreshWLANConfig(ctx, t.deviceID)
		case taskTypeWake:
			err = getParameterValuesLive(ctx, t.deviceID, t.paths)
		default:
			err = fmt.Errorf("unknown task type: %s", t.taskType)
		}

		// Release context resources
		cancel()

		if t.jobID != "" {
			jobRegistryInstance.finish(t.jobID, err)
		}

		if err != nil {
			// Log any errors encountered during task execution
			logger.Error("Worker task failed",
				zap.String("deviceID", t.deviceID),
				zap.String("taskType", t.taskType),
				zap.Error(err),
			)
			continue
		}

		// Invalidate the device cache only after the task has actually been
		// applied. setParameterValues uses ?connection_request, so by the
		// time we get here the CPE has already been poked (or the request
		// failed and the task sits queued for the next inform). Clearing at
		// submit time instead would let a read racing the write re-cache the
		// pre-change snapshot for the full TTL — the change then appears to
		// take ~30s even though it landed in ~2s.
		deviceCacheInstance.clear(t.deviceID)
	}
}

// Submit adds a new task to the worker pool queue for asynchronous processing.
// It records the task in the job registry (visible via GET /jobs) and returns
// the generated job ID plus false if the queue is full.
//
// paths is only used by taskTypeWake (the getParameterValues refresh); other
// task types ignore it.
func (wp *workerPool) Submit(deviceID, taskType string, params [][]interface{}, paths ...string) (string, bool) {
	id := jobRegistryInstance.add(taskType, deviceID, len(params))
	select {
	case wp.queue <- task{jobID: id, deviceID: deviceID, taskType: taskType, params: params, paths: paths}:
		return id, true
	default:
		logger.Warn("Worker pool queue full, task dropped",
			zap.String("deviceID", deviceID),
			zap.String("taskType", taskType),
		)
		jobRegistryInstance.finish(id, errWorkerQueueFull)
		return id, false
	}
}
