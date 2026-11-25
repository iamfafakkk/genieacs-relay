package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// handlers_provisions.go covers the GenieACS script collections the admin
// panel manages: provision scripts and virtual parameters. Both are NBI
// resources whose body is raw JavaScript, not JSON:
//
//	GET    /api/v1/genieacs/provisions                  — list provision scripts
//	PUT    /api/v1/genieacs/provisions/{name}           — create/update a provision
//	DELETE /api/v1/genieacs/provisions/{name}           — delete a provision
//	GET    /api/v1/genieacs/virtual-parameters          — list virtual parameters
//	PUT    /api/v1/genieacs/virtual-parameters/{name}   — create/update a virtual parameter
//	DELETE /api/v1/genieacs/virtual-parameters/{name}   — delete a virtual parameter
//
// The NBI has no single-resource GET for these collections, so the relay
// lists the whole collection and the UI picks the script by name.

const (
	collectionProvisions        = "provisions"
	collectionVirtualParameters = "virtual_parameters"
)

// scriptNameRegex matches provision and virtual parameter names. Virtual
// parameter names cannot contain a period, which this pattern excludes.
var scriptNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ScriptEntry is one provision / virtual parameter script.
//
// @Description A GenieACS provision or virtual parameter script.
type ScriptEntry struct {
	Name   string `json:"name"`
	Script string `json:"script"`
}

// ProvisionsResponse is the shape returned by GET /provisions.
//
// @Description All provision scripts in GenieACS.
type ProvisionsResponse struct {
	Provisions []ScriptEntry `json:"provisions"`
}

// VirtualParametersResponse is the shape returned by GET /virtual-parameters.
//
// @Description All virtual parameters in GenieACS.
type VirtualParametersResponse struct {
	VirtualParameters []ScriptEntry `json:"virtual_parameters"`
}

// ScriptOperationResponse is the shape returned by PUT/DELETE script calls.
//
// @Description Script operation response.
type ScriptOperationResponse struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

var (
	_ = ScriptEntry{}
	_ = ProvisionsResponse{}
	_ = VirtualParametersResponse{}
	_ = ScriptOperationResponse{}
)

// listProvisionsHandler returns every provision script in GenieACS.
//
//	@Summary		List provision scripts
//	@Description	Returns every provision script known to GenieACS (NBI `GET /provisions/`).
//	@Tags			Provisioning
//	@Produce		json
//	@Success		200	{object}	Response{data=ProvisionsResponse}
//	@Failure		401	{object}	Response
//	@Failure		500	{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/provisions [get]
func listProvisionsHandler(w http.ResponseWriter, r *http.Request) {
	scripts, err := scriptsNBI(r.Context(), collectionProvisions)
	if err != nil {
		logger.Error("Provisions list failed", zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrScriptsListFailed)
		return
	}
	sendResponse(w, http.StatusOK, ProvisionsResponse{Provisions: scripts})
}

// listVirtualParametersHandler returns every virtual parameter in GenieACS.
//
//	@Summary		List virtual parameters
//	@Description	Returns every virtual parameter known to GenieACS (NBI `GET /virtual_parameters/`).
//	@Tags			Provisioning
//	@Produce		json
//	@Success		200	{object}	Response{data=VirtualParametersResponse}
//	@Failure		401	{object}	Response
//	@Failure		500	{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/virtual-parameters [get]
func listVirtualParametersHandler(w http.ResponseWriter, r *http.Request) {
	scripts, err := scriptsNBI(r.Context(), collectionVirtualParameters)
	if err != nil {
		logger.Error("Virtual parameters list failed", zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrScriptsListFailed)
		return
	}
	sendResponse(w, http.StatusOK, VirtualParametersResponse{VirtualParameters: scripts})
}

// putProvisionHandler creates or replaces a provision script.
//
//	@Summary		Create or update a provision script
//	@Description	Creates or replaces the provision script (NBI `PUT /provisions/{name}`). The request body is the JavaScript source sent as-is.
//	@Tags			Provisioning
//	@Accept			plain
//	@Produce		json
//	@Param			name	path		string	true	"Provision name (alphanumeric, underscore, hyphen, 1-64 chars)"
//	@Param			body	body		string	true	"Provision JavaScript source"
//	@Success		202		{object}	Response{data=ScriptOperationResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/provisions/{name} [put]
func putProvisionHandler(w http.ResponseWriter, r *http.Request) {
	putScriptHandler(w, r, collectionProvisions, MsgProvisionSaved)
}

// deleteProvisionHandler removes a provision script.
//
//	@Summary		Delete a provision script
//	@Description	Removes the provision script from GenieACS (NBI `DELETE /provisions/{name}`). Deleting a missing provision still returns success.
//	@Tags			Provisioning
//	@Produce		json
//	@Param			name	path		string	true	"Provision name (alphanumeric, underscore, hyphen, 1-64 chars)"
//	@Success		202		{object}	Response{data=ScriptOperationResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/provisions/{name} [delete]
func deleteProvisionHandler(w http.ResponseWriter, r *http.Request) {
	deleteScriptHandler(w, r, collectionProvisions, MsgProvisionDeleted)
}

// putVirtualParameterHandler creates or replaces a virtual parameter script.
//
//	@Summary		Create or update a virtual parameter
//	@Description	Creates or replaces the virtual parameter script (NBI `PUT /virtual_parameters/{name}`). The request body is the JavaScript source sent as-is.
//	@Tags			Provisioning
//	@Accept			plain
//	@Produce		json
//	@Param			name	path		string	true	"Virtual parameter name (alphanumeric, underscore, hyphen, 1-64 chars, no periods)"
//	@Param			body	body		string	true	"Virtual parameter JavaScript source"
//	@Success		202		{object}	Response{data=ScriptOperationResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/virtual-parameters/{name} [put]
func putVirtualParameterHandler(w http.ResponseWriter, r *http.Request) {
	putScriptHandler(w, r, collectionVirtualParameters, MsgVirtualParameterSaved)
}

// deleteVirtualParameterHandler removes a virtual parameter script.
//
//	@Summary		Delete a virtual parameter
//	@Description	Removes the virtual parameter from GenieACS (NBI `DELETE /virtual_parameters/{name}`). Deleting a missing virtual parameter still returns success.
//	@Tags			Provisioning
//	@Produce		json
//	@Param			name	path		string	true	"Virtual parameter name (alphanumeric, underscore, hyphen, 1-64 chars, no periods)"
//	@Success		202		{object}	Response{data=ScriptOperationResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/virtual-parameters/{name} [delete]
func deleteVirtualParameterHandler(w http.ResponseWriter, r *http.Request) {
	deleteScriptHandler(w, r, collectionVirtualParameters, MsgVirtualParameterDeleted)
}

// putScriptHandler implements PUT for both script collections.
func putScriptHandler(w http.ResponseWriter, r *http.Request, collection, successMsg string) {
	name := chi.URLParam(r, "name")
	if !scriptNameRegex.MatchString(name) {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrScriptNameInvalid)
		return
	}
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxPresetBodySize))
	if err != nil || strings.TrimSpace(string(bodyBytes)) == "" {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrScriptBodyRequired)
		return
	}
	if err := scriptNBICall(r.Context(), http.MethodPut, collection, name, bodyBytes); err != nil {
		logger.Error("Script PUT failed",
			zap.String("collection", collection), zap.String("name", name), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrScriptSaveFailed)
		return
	}
	sendResponse(w, http.StatusAccepted, ScriptOperationResponse{Message: successMsg, Name: name})
}

// deleteScriptHandler implements DELETE for both script collections.
func deleteScriptHandler(w http.ResponseWriter, r *http.Request, collection, successMsg string) {
	name := chi.URLParam(r, "name")
	if !scriptNameRegex.MatchString(name) {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrScriptNameInvalid)
		return
	}
	if err := scriptNBICall(r.Context(), http.MethodDelete, collection, name, nil); err != nil {
		logger.Error("Script DELETE failed",
			zap.String("collection", collection), zap.String("name", name), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrScriptDeleteFailed)
		return
	}
	sendResponse(w, http.StatusAccepted, ScriptOperationResponse{Message: successMsg, Name: name})
}

// scriptsNBI lists a GenieACS script collection and returns name/script
// pairs sorted by name. Entries without a usable `_id` are skipped.
func scriptsNBI(ctx context.Context, collection string) ([]ScriptEntry, error) {
	urlQ := fmt.Sprintf("%s/%s/", geniesBaseURL, collection)
	//nolint:gosec // G107: URL built from trusted internal config + fixed collection name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlQ, http.NoBody)
	if err != nil {
		return nil, err
	}
	if nbiAuth && nbiAuthKey != "" {
		req.Header.Set(HeaderXAPIKey, nbiAuthKey)
	}
	resp, err := nbiDo(req)
	if err != nil {
		return nil, err
	}
	defer safeClose(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GenieACS %s query failed with status %s: %s", collection, resp.Status, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]ScriptEntry, 0, len(raw))
	for _, doc := range raw {
		name, _ := doc["_id"].(string)
		if name == "" {
			continue
		}
		script, _ := doc["script"].(string)
		out = append(out, ScriptEntry{Name: name, Script: script})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// scriptNBICall executes one script NBI request. PUT bodies are the raw
// JavaScript source; DELETEs are bodyless.
func scriptNBICall(ctx context.Context, method, collection, name string, body []byte) error {
	urlQ := fmt.Sprintf("%s/%s/%s", geniesBaseURL, collection, url.PathEscape(name))
	var bodyReader io.Reader = http.NoBody
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	//nolint:gosec // G107: URL built from trusted internal config + fixed collection name
	req, err := http.NewRequestWithContext(ctx, method, urlQ, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	if nbiAuth && nbiAuthKey != "" {
		req.Header.Set(HeaderXAPIKey, nbiAuthKey)
	}
	// gosec G704: the name is regex-validated in the handler AND
	// path-escaped here, so the SSRF taint is already sanitized.
	//nolint:gosec // G704: validated + path-escaped name
	resp, err := nbiDo(req)
	if err != nil {
		return err
	}
	defer safeClose(resp.Body)
	// Deleting an already-absent script is idempotent success; anything
	// else at 4xx/5xx is a real failure.
	if resp.StatusCode >= http.StatusBadRequest &&
		!(method == http.MethodDelete && resp.StatusCode == http.StatusNotFound) {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %s: %s", resp.Status, string(respBody))
	}
	return nil
}
