package main

import (
	"net/http"
	"strconv"
)

// listJobsHandler returns the in-memory worker job history, newest first.
// Jobs live only for the process lifetime; on restart the list is empty
// because the task queue itself is in-memory.
//
//	@Summary		List worker jobs
//	@Description	Returns the most recent asynchronous worker jobs (setParameterValues / applyChanges / refreshWLAN) with their current status, so the admin panel can monitor progress. Newest first, capped at 200.
//	@Tags			Jobs
//	@Produce		json
//	@Param			limit	query		int	false	"Max jobs to return (default 100, max 200)"	example(100)
//	@Success		200		{object}	Response{data=JobListResponse}
//	@Failure		401		{object}	Response
//	@Failure		429		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/jobs [get]
func listJobsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > MaxTrackedJobs {
		limit = MaxTrackedJobs
	}

	all := jobRegistryInstance.list(MaxTrackedJobs)
	jobs := all
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}

	sendResponse(w, http.StatusOK, JobListResponse{
		Jobs:   jobs,
		Active: jobRegistryInstance.active(),
		Count:  len(all),
	})
}
