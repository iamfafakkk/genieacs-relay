package main

import (
	"net/http"

	"go.uber.org/zap"
)

// handlers_lifecycle.go contains the v2.2.0 CPE lifecycle endpoints:
//
//	POST /api/v1/genieacs/factory-reset/{ip}  — H6: TR-069 FactoryReset RPC
//	POST /api/v1/genieacs/wake/{ip}           — H2: ConnectionRequest wake-up
//
// Both are thin wrappers over the generic TR-069 RPC dispatchers in
// tr069.go. The handlers themselves only do: extract device ID from
// IP, dispatch the RPC, return 202 on success. No vendor-specific
// path detection because the underlying TR-069 RPCs are dialect-agnostic.

// WakeRequest is the optional body for POST /wake/{ip}. Paths are dot-less
// TR-069 parameter subtrees (e.g. "InternetGatewayDevice.WANDevice") the
// getParameterValues refresh should cover — the admin panel sends the subtree
// backing the list (or tab) whose Summon button was pressed. A trailing dot
// faults ("Invalid parameter path"). Empty falls back to the backend's
// Overview subtrees.
//
// @Description Parameter subtrees to refresh on summon. Omit to refresh the backend default (DeviceInfo + WANDevice).
type WakeRequest struct {
	Paths []string `json:"paths,omitempty"`
}

// factoryResetDeviceHandler triggers a TR-069 FactoryReset RPC against
// the CPE. Destructive — wipes all locally-stored config (PPPoE creds,
// WLAN, port-forward rules, etc) and reboots the device. The CPE is
// unreachable for 60-180 seconds during the reset cycle, then rejoins
// the ACS in a fresh provisioning state. Used by RMA flows and
// customer-requested "reset my modem to factory" support tickets.
//
//	@Summary		Factory reset CPE
//	@Description	Triggers a TR-069 FactoryReset RPC against the CPE identified by IP. Destructive — the CPE will lose its current PPPoE credentials, WLAN config, port-forward rules, and any other locally-stored state, then rejoin the ACS in a fresh provisioning state. The device is unreachable for 60-180 seconds after the task is applied.
//	@Tags			Lifecycle
//	@Produce		json
//	@Param			ip	path		string	true	"Device IP address"	example(192.168.1.1)
//	@Success		202	{object}	Response{data=MessageResponse}
//	@Failure		400	{object}	Response
//	@Failure		401	{object}	Response
//	@Failure		404	{object}	Response
//	@Failure		429	{object}	Response
//	@Failure		500	{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/factory-reset/{ip} [post]
func factoryResetDeviceHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	if err := factoryResetDevice(r.Context(), deviceID); err != nil {
		logger.Error("FactoryReset task submission failed",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrFactoryResetFailed)
		return
	}
	// Clear cached device tree — the post-reset device tree will look
	// completely different and stale cached values would be misleading.
	deviceCacheInstance.clear(deviceID)
	sendResponse(w, http.StatusAccepted, MessageResponse{
		Message: MsgFactoryResetSubmitted,
	})
}

// wakeDeviceHandler fires a TR-069 getParameterValues against the CPE with
// `?connection_request`, scoped to the parameter subtrees the caller asked
// for (one per admin-panel tab). This is the "summon" action: it both wakes
// the device (connection_request) and refreshes the values the panel shows,
// instead of poking the CPE and leaving the tree stale.
//
// Dispatched through the worker pool (like refreshWLAN) so the request does
// not block and the wake appears in GET /jobs alongside other async work.
//
//	@Summary		Summon CPE (refresh + wake)
//	@Description	Submits a TR-069 getParameterValues task with ?connection_request, scoped to the given parameter subtrees, so the CPE both wakes immediately and refreshes the values the admin panel reads. Dispatched asynchronously via the worker pool; monitor progress in GET /jobs.
//	@Tags			Lifecycle
//	@Accept			json
//	@Produce		json
//	@Param			ip		path		string			true	"Device IP address"	example(192.168.1.1)
//	@Param			body	body		WakeRequest		false	"Parameter subtrees to refresh (defaults to the whole tree)"
//	@Success		202		{object}	Response{data=MessageResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		404		{object}	Response
//	@Failure		429		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/wake/{ip} [post]
func wakeDeviceHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}

	// Body is optional: an empty request falls back to the Overview subtrees.
	// Validate each caller-supplied path before forwarding it to the NBI.
	var req WakeRequest
	if r.ContentLength != 0 {
		if !ParseJSONRequest(w, r, &req) {
			return
		}
		for _, p := range req.Paths {
			if err := validateTRParamPath(p); err != nil {
				sendError(w, r, http.StatusBadRequest, ErrCodeValidation, formatInvalidParamPath(p))
				return
			}
		}
	}
	if len(req.Paths) == 0 {
		// No body → refresh the data the Overview tab reads. Deliberately not
		// the whole tree: a full-tree getParameterValues blocks ~25s on the
		// observed HG8145V5, uncomfortably close to WorkerTaskTimeout (30s).
		req.Paths = []string{
			PathInternetGatewayDevice + ".DeviceInfo",
			PathInternetGatewayDevice + ".WANDevice.1",
		}
	}

	if _, ok := taskWorkerPool.Submit(deviceID, taskTypeWake, nil, req.Paths...); !ok {
		sendError(w, r, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, ErrWorkerPoolBusy)
		return
	}
	sendResponse(w, http.StatusAccepted, MessageResponse{
		Message: MsgWakeDispatched,
	})
}
