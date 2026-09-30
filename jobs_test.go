package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestJobRegistry_Lifecycle(t *testing.T) {
	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	id := jobRegistryInstance.add(TaskTypeSetParams, "dev-1", 2)
	// IDs must be unique across process restarts, otherwise the panel's
	// localStorage "already announced" list would suppress new jobs that
	// reuse a counter value (e.g. after an air hot-reload).
	assert.Regexp(t, `^job-[0-9a-f]{8}-[0-9]+$`, id)
	jobs := jobRegistryInstance.list(10)
	require.Len(t, jobs, 1)
	assert.Equal(t, JobStatusQueued, jobs[0].Status)
	assert.Equal(t, 1, jobRegistryInstance.active())

	jobRegistryInstance.start(id)
	jobRegistryInstance.finish(id, nil)

	jobs = jobRegistryInstance.list(10)
	require.Len(t, jobs, 1)
	assert.Equal(t, JobStatusSuccess, jobs[0].Status)
	assert.NotNil(t, jobs[0].FinishedAt)
	assert.Equal(t, 0, jobRegistryInstance.active())
}

func TestJobRegistry_QueueFullMarksFailed(t *testing.T) {
	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	// Zero-capacity queue with no workers → Submit must drop the task.
	wp := &workerPool{workers: 0, queue: make(chan task), wg: sync.WaitGroup{}}

	id, ok := wp.Submit("dev-2", TaskTypeSetParams, nil)
	assert.False(t, ok)

	job := jobRegistryInstance.find(id)
	require.NotNil(t, job)
	assert.Equal(t, JobStatusFailed, job.Status)
	assert.Contains(t, job.Error, "queue full")
}

// TestWorkerPool_RecordsJob verifies a real worker run leaves a success job
// behind — the data the /jobs page and sonner notifications rely on.
func TestWorkerPool_RecordsJob(t *testing.T) {
	logger, _ = zap.NewDevelopment()

	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	origHTTPClient := httpClient
	origBaseURL := geniesBaseURL
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mockServer.Close)
	httpClient = mockServer.Client()
	geniesBaseURL = mockServer.URL
	t.Cleanup(func() {
		httpClient = origHTTPClient
		geniesBaseURL = origBaseURL
	})

	wp := &workerPool{workers: 1, queue: make(chan task, 10), wg: sync.WaitGroup{}}
	wp.Start()
	wp.Submit("dev-3", TaskTypeSetParams, [][]interface{}{{"p", "v", "xsd:string"}})
	wp.Stop()

	jobs := jobRegistryInstance.list(10)
	require.Len(t, jobs, 1)
	assert.Equal(t, JobStatusSuccess, jobs[0].Status)
	assert.True(t, jobs[0].DurationMs >= 0)
}

func TestListJobsHandler(t *testing.T) {
	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	jobRegistryInstance.add(TaskTypeRefreshWLAN, "dev-4", 0)

	rr := httptest.NewRecorder()
	listJobsHandler(rr, httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/jobs", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
	var body Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	raw, err := json.Marshal(body.Data)
	require.NoError(t, err)
	var list JobListResponse
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Equal(t, 1, list.Count)
	assert.Equal(t, 1, list.Active)
	require.Len(t, list.Jobs, 1)
	assert.Equal(t, "dev-4", list.Jobs[0].DeviceID)
}

func TestJobsStreamHandler_PushesOnChange(t *testing.T) {
	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	srv := httptest.NewServer(http.HandlerFunc(jobsStreamHandler))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	readState := func() JobListResponse {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var msg JobListResponse
		require.NoError(t, conn.ReadJSON(&msg))
		return msg
	}

	// Initial snapshot on connect.
	first := readState()
	assert.Equal(t, 0, first.Count)

	// A new job must be pushed without the client asking.
	jobRegistryInstance.add(TaskTypeSetParams, "dev-ws", 1)

	deadline := time.Now().Add(3 * time.Second)
	var got JobListResponse
	for time.Now().Before(deadline) {
		got = readState()
		if got.Count == 1 {
			break
		}
	}
	require.Equal(t, 1, got.Count, "expected the pushed snapshot to include the new job")
	require.Len(t, got.Jobs, 1)
	assert.Equal(t, "dev-ws", got.Jobs[0].DeviceID)
}
