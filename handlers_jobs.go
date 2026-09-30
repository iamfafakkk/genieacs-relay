package main

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
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
	limit := parseJobLimit(r.URL.Query().Get("limit"))
	resp, _ := jobRegistryInstance.snapshot(limit)
	sendResponse(w, http.StatusOK, resp)
}

// parseJobLimit clamps the ?limit query to [1, MaxTrackedJobs], defaulting to 100.
func parseJobLimit(raw string) int {
	limit := 100
	if raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > MaxTrackedJobs {
		limit = MaxTrackedJobs
	}
	return limit
}

// wsUpgrader upgrades /jobs/ws. CheckOrigin is enforced against the same
// allow-list the REST CORS middleware uses, so a random page cannot open a
// socket with the operator's credentials.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     wsOriginAllowed,
}

// wsOriginAllowed mirrors corsAllowedOrigins (loaded from CORS_ALLOWED_ORIGINS).
// Non-browser clients send no Origin header and are allowed through.
func wsOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(corsAllowedOrigins) == 1 && corsAllowedOrigins[0] == "*" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	for _, allowed := range corsAllowedOrigins {
		if allowed == origin || allowed == u.Host {
			return true
		}
	}
	return false
}

// jobsStreamHandler pushes the job list over a WebSocket whenever it changes.
//
// Push-on-change (versioned registry snapshot) keeps idle traffic at zero
// instead of the 4s poll the admin panel used before. A ping every 30s keeps
// intermediaries from dropping an idle connection.
//
//	@Summary		Stream worker jobs (WebSocket)
//	@Description	Upgrades to a WebSocket and pushes the current job list (JSON, same shape as GET /jobs) every time a job is queued, starts, or finishes. Sends a ping every 30s. No authentication beyond the API-key middleware that guards this route.
//	@Tags			Jobs
//	@Security		ApiKeyAuth
//	@Router			/jobs/ws [get]
func jobsStreamHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an error response.
		logger.Warn("jobs WS upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Drain client frames (we ignore them) so close/control frames are
	// processed and the read side does not stall.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	write := func() bool {
		resp, _ := jobRegistryInstance.snapshot(MaxTrackedJobs)
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(resp) == nil
	}

	// Initial full state so a fresh subscriber renders immediately.
	if !write() {
		return
	}
	lastVersion := currentJobsVersion()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		case <-ticker.C:
			if v := currentJobsVersion(); v != lastVersion {
				if !write() {
					return
				}
				lastVersion = v
			}
		}
	}
}

// currentJobsVersion is the registry's mutation counter.
func currentJobsVersion() uint64 {
	return jobRegistryInstance.currentVersion()
}
