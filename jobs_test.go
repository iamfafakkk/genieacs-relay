package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestJobRegistry_Lifecycle(t *testing.T) {
	origRegistry := jobRegistryInstance
	jobRegistryInstance = newJobRegistry(MaxTrackedJobs)
	t.Cleanup(func() { jobRegistryInstance = origRegistry })

	id := jobRegistryInstance.add(TaskTypeSetParams, "dev-1", 2)
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
