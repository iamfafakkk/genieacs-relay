package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// --- Worker Pool Tests ---

func TestWorker_TaskFailure(t *testing.T) {
	// Create a buffered logger to capture log output
	var buffer bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buffer), zap.DebugLevel)
	testLogger := zap.New(core)

	// Ensure logger is set before starting the test
	// This is important because other tests may have left logger in an unexpected state
	originalLogger := logger
	logger = testLogger

	// Setup mock server that returns error
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	mockServer := httptest.NewServer(mockHandler)
	t.Cleanup(mockServer.Close)

	originalBaseURL := geniesBaseURL
	originalNBIKey := nbiAuthKey
	originalHTTPClient := httpClient

	geniesBaseURL = mockServer.URL
	nbiAuthKey = "mock-nbi-key"
	httpClient = mockServer.Client() // Important: use mock server's client

	// Restore all state at the end
	t.Cleanup(func() {
		geniesBaseURL = originalBaseURL
		nbiAuthKey = originalNBIKey
		httpClient = originalHTTPClient
		logger = originalLogger
	})

	// Create worker pool and process task
	wp := &workerPool{
		workers: 1,
		queue:   make(chan task, 10),
		wg:      sync.WaitGroup{},
	}

	// Verify logger is set correctly before starting
	if logger != testLogger {
		t.Fatal("Logger was not set correctly")
	}

	wp.Start()
	wp.Submit(mockDeviceID, taskTypeSetParams, [][]interface{}{{"some.param", "value", "xsd:string"}})
	time.Sleep(300 * time.Millisecond) // Give worker time to process the task
	wp.Stop()

	logOutput := buffer.String()
	if logOutput == "" {
		t.Logf("Logger pointer: %p, testLogger pointer: %p", logger, testLogger)
		t.Logf("Logger is nil: %v", logger == nil)
	}
	assert.Contains(t, logOutput, "Worker task failed", "Expected log to contain 'Worker task failed', got: %s", logOutput)
}

func TestWorkerPool_EdgeCases(t *testing.T) {
	t.Run("Unknown task type", func(t *testing.T) {
		var buffer bytes.Buffer
		encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		core := zapcore.NewCore(encoder, zapcore.AddSync(&buffer), zap.InfoLevel)
		testLogger := zap.New(core)
		originalLogger := logger
		logger = testLogger
		defer func() { logger = originalLogger }()

		wp := &workerPool{
			workers: 1,
			queue:   make(chan task, 1),
			wg:      sync.WaitGroup{},
		}
		wp.Start()
		wp.Submit("test-device", "unknown_task_type", nil)
		time.Sleep(100 * time.Millisecond) // Give worker time to process
		wp.Stop()

		assert.Contains(t, buffer.String(), "Worker task failed")
	})
}

// TestWorker_ClearsCacheAfterTask verifies the device cache is invalidated by
// the worker once a task completes, not at submit time. Clearing at submit
// time let a read racing the write re-cache the pre-change snapshot for the
// full TTL, making connection_request writes appear to take ~30s.
func TestWorker_ClearsCacheAfterTask(t *testing.T) {
	logger, _ = zap.NewDevelopment()
	originalHTTPClient := httpClient
	originalBaseURL := geniesBaseURL
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mockServer.Close)
	httpClient = mockServer.Client()
	geniesBaseURL = mockServer.URL
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		geniesBaseURL = originalBaseURL
	})

	originalCache := deviceCacheInstance
	deviceCacheInstance = &deviceCache{data: make(map[string]cachedDeviceData), timeout: DefaultCacheTimeout}
	t.Cleanup(func() { deviceCacheInstance = originalCache })

	deviceCacheInstance.set("cache-device", map[string]interface{}{"stale": true})

	wp := &workerPool{workers: 1, queue: make(chan task, 10), wg: sync.WaitGroup{}}
	wp.Start()
	wp.Submit("cache-device", taskTypeSetParams, [][]interface{}{{"p", "v", "xsd:string"}})
	wp.Stop()

	_, found := deviceCacheInstance.get("cache-device")
	assert.False(t, found, "cache should be cleared once the worker applied the task")
}

// TestWorker_WakeRefreshesTabs verifies the summon task type (taskTypeWake)
// issues a real TR-069 getParameterValues through the worker pool: a POST to
// /devices/{id}/tasks?connection_request carrying the tab-scoped paths.
func TestWorker_WakeRefreshesTabs(t *testing.T) {
	originalHTTPClient := httpClient
	originalBaseURL := geniesBaseURL
	var gotPath, gotQuery, gotBody string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mockServer.Close)
	httpClient = mockServer.Client()
	geniesBaseURL = mockServer.URL
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		geniesBaseURL = originalBaseURL
	})

	wp := &workerPool{workers: 1, queue: make(chan task, 10), wg: sync.WaitGroup{}}
	wp.Start()
	wp.Submit("wake-device", taskTypeWake, nil, "InternetGatewayDevice.WANDevice")
	wp.Stop()

	assert.Equal(t, "/devices/wake-device/tasks", gotPath)
	assert.Equal(t, "connection_request", gotQuery)
	assert.JSONEq(t,
		`{"name":"getParameterValues","parameterNames":["InternetGatewayDevice.WANDevice"]}`,
		gotBody, "summon must refresh (getParameterValues), not just poke")
}

// --- Worker Pool Deadlock Prevention Tests ---
func TestWorkerPoolNonBlockingSubmit(t *testing.T) {
	// Setup logger
	logger, _ = zap.NewDevelopment()
	defer func() { _ = logger.Sync() }()

	t.Run("Submit when queue is full drops task", func(t *testing.T) {
		// Create worker pool with tiny queue (size 1)
		wp := &workerPool{
			workers: 1,
			queue:   make(chan task, 1),
			wg:      sync.WaitGroup{},
		}

		// Don't start workers so queue fills up
		// Fill the queue
		wp.queue <- task{deviceID: "device1", taskType: "test", params: nil}

		// This should NOT block - it should drop the task and log warning
		done := make(chan bool, 1)
		go func() {
			wp.Submit("device2", "test", nil)
			done <- true
		}()

		select {
		case <-done:
			// Success - Submit returned without blocking
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Submit blocked when queue was full - potential deadlock")
		}
	})

	t.Run("Submit succeeds when queue has space", func(t *testing.T) {
		wp := &workerPool{
			workers: 1,
			queue:   make(chan task, 10),
			wg:      sync.WaitGroup{},
		}

		done := make(chan bool, 1)
		go func() {
			wp.Submit("device1", "test", nil)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Submit blocked when queue had space")
		}

		// Drain the queue
		<-wp.queue
	})
}
