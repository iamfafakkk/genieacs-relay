package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlers_provisions_test.go covers the provision script and virtual
// parameter endpoints (list/put/delete) plus the shared scriptsNBI and
// scriptNBICall NBI helpers.

// scriptMockHandler records the last script request and replies with the
// configured status.
type scriptRecord struct {
	method      string
	path        string
	body        string
	contentType string
}

func scriptMockHandler(t *testing.T, status int, listBody string, rec *scriptRecord) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			// Collection listing: GET /{collection}/
			if status >= http.StatusBadRequest {
				w.WriteHeader(status)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(listBody))
			return
		}
		if rec != nil {
			rec.method = r.Method
			rec.path = r.URL.Path
			rec.contentType = r.Header.Get("Content-Type")
			b, _ := io.ReadAll(r.Body)
			rec.body = string(b)
		}
		w.WriteHeader(status)
	}
}

// ============================================================
// scriptsNBI
// ============================================================

func TestScriptsNBI_SuccessSortsAndSkipsInvalidEntries(t *testing.T) {
	list := `[{"_id":"zeta","script":"z"},{"_id":"alpha","script":"a"},{"script":"no-id"},{"_id":"mid"}]`
	setupTestServer(t, scriptMockHandler(t, http.StatusOK, list, nil))

	scripts, err := scriptsNBI(t.Context(), collectionProvisions)
	require.NoError(t, err)
	require.Len(t, scripts, 3)
	assert.Equal(t, "alpha", scripts[0].Name)
	assert.Equal(t, "a", scripts[0].Script)
	assert.Equal(t, "mid", scripts[1].Name)
	assert.Equal(t, "", scripts[1].Script)
	assert.Equal(t, "zeta", scripts[2].Name)
}

func TestScriptsNBI_StatusFailure(t *testing.T) {
	setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))
	_, err := scriptsNBI(t.Context(), collectionProvisions)
	assert.Error(t, err)
}

func TestScriptsNBI_UnmarshalFailure(t *testing.T) {
	setupTestServer(t, scriptMockHandler(t, http.StatusOK, "not-json", nil))
	_, err := scriptsNBI(t.Context(), collectionVirtualParameters)
	assert.Error(t, err)
}

func TestScriptsNBI_TransportFailure(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{Transport: &failingTransport{}}
	t.Cleanup(func() { httpClient = originalClient })

	_, err := scriptsNBI(t.Context(), collectionProvisions)
	assert.Error(t, err)
}

func TestScriptsNBI_ReadFailure(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{Transport: &readFailBodyTransport{}}
	t.Cleanup(func() { httpClient = originalClient })

	_, err := scriptsNBI(t.Context(), collectionProvisions)
	assert.Error(t, err)
}

func TestScriptsNBI_NewRequestFailure(t *testing.T) {
	originalBase := geniesBaseURL
	geniesBaseURL = "http://example.com/\x7f"
	t.Cleanup(func() { geniesBaseURL = originalBase })

	_, err := scriptsNBI(t.Context(), collectionProvisions)
	assert.Error(t, err)
}

// readFailBodyTransport returns a 200 with a body that fails to read.
type readFailBodyTransport struct{}

func (r *readFailBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       &errorReader{},
	}, nil
}

// ============================================================
// scriptNBICall
// ============================================================

func TestScriptNBICall_PutSendsRawScript(t *testing.T) {
	var rec scriptRecord
	setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	err := scriptNBICall(t.Context(), http.MethodPut, collectionProvisions, "my-provision", []byte("declare(\"x\");"))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, rec.method)
	assert.Equal(t, "/provisions/my-provision", rec.path)
	assert.Equal(t, "declare(\"x\");", rec.body)
	assert.Equal(t, "text/plain; charset=utf-8", rec.contentType)
}

func TestScriptNBICall_DeleteIsBodyless(t *testing.T) {
	var rec scriptRecord
	setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	err := scriptNBICall(t.Context(), http.MethodDelete, collectionVirtualParameters, "vp", nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.method)
	assert.Equal(t, "/virtual_parameters/vp", rec.path)
	assert.Equal(t, "", rec.body)
	assert.Equal(t, "", rec.contentType)
}

func TestScriptNBICall_NotFoundIsSuccess(t *testing.T) {
	setupTestServer(t, scriptMockHandler(t, http.StatusNotFound, "", nil))
	assert.NoError(t, scriptNBICall(t.Context(), http.MethodDelete, collectionProvisions, "gone", nil))
}

func TestScriptNBICall_PutNotFoundFails(t *testing.T) {
	setupTestServer(t, scriptMockHandler(t, http.StatusNotFound, "", nil))
	err := scriptNBICall(t.Context(), http.MethodPut, collectionProvisions, "p", []byte("x"))
	assert.Error(t, err)
}

func TestScriptNBICall_StatusFailure(t *testing.T) {
	setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))
	err := scriptNBICall(t.Context(), http.MethodPut, collectionProvisions, "p", []byte("x"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestScriptNBICall_TransportFailure(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{Transport: &failingTransport{}}
	t.Cleanup(func() { httpClient = originalClient })

	assert.Error(t, scriptNBICall(t.Context(), http.MethodPut, collectionProvisions, "p", []byte("x")))
}

func TestScriptNBICall_NewRequestFailure(t *testing.T) {
	originalBase := geniesBaseURL
	geniesBaseURL = "http://example.com/\x7f"
	t.Cleanup(func() { geniesBaseURL = originalBase })

	assert.Error(t, scriptNBICall(t.Context(), http.MethodPut, collectionProvisions, "p", []byte("x")))
}

// ============================================================
// handlers
// ============================================================

func TestListProvisionsHandler_Success(t *testing.T) {
	list := `[{"_id":"default","script":"declare(\"ok\",true);"}]`
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, list, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/provisions", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"provisions"`)
	assert.Contains(t, rr.Body.String(), `"default"`)
}

func TestListProvisionsHandler_Failure(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/provisions", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptsListFailed)
}

func TestListVirtualParametersHandler_Success(t *testing.T) {
	list := `[{"_id":"getMAC","script":"return 1;"}]`
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, list, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/virtual-parameters", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"virtual_parameters"`)
	assert.Contains(t, rr.Body.String(), `"getMAC"`)
}

func TestListVirtualParametersHandler_Failure(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/virtual-parameters", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPutProvisionHandler_Success(t *testing.T) {
	var rec scriptRecord
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/provisions/fw",
		strings.NewReader("declare(\"ok\",true);"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgProvisionSaved)
	assert.Equal(t, "declare(\"ok\",true);", rec.body)
}

func TestPutProvisionHandler_InvalidName(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/provisions/bad.name",
		strings.NewReader("x"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptNameInvalid)
}

func TestPutProvisionHandler_EmptyBody(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/provisions/fw",
		strings.NewReader("   \n\t "))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptBodyRequired)
}

func TestPutProvisionHandler_BodyReadError(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/provisions/fw", nil)
	req.Body = io.NopCloser(&errorReader{})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptBodyRequired)
}

func TestPutProvisionHandler_NBIFailure(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/provisions/fw",
		strings.NewReader("x"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptSaveFailed)
}

func TestDeleteProvisionHandler_Success(t *testing.T) {
	var rec scriptRecord
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/provisions/fw", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgProvisionDeleted)
	assert.Equal(t, "/provisions/fw", rec.path)
}

func TestDeleteProvisionHandler_InvalidName(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/provisions/bad.name", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDeleteProvisionHandler_NBIFailure(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/provisions/fw", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrScriptDeleteFailed)
}

func TestPutVirtualParameterHandler_Success(t *testing.T) {
	var rec scriptRecord
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/virtual-parameters/getMAC",
		strings.NewReader("return 1;"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgVirtualParameterSaved)
	assert.Equal(t, "/virtual_parameters/getMAC", rec.path)
	assert.Equal(t, "return 1;", rec.body)
}

func TestPutVirtualParameterHandler_InvalidName(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/genieacs/virtual-parameters/bad.name",
		strings.NewReader("x"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDeleteVirtualParameterHandler_Success(t *testing.T) {
	var rec scriptRecord
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", &rec))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/virtual-parameters/getMAC", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgVirtualParameterDeleted)
	assert.Equal(t, "/virtual_parameters/getMAC", rec.path)
}

func TestDeleteVirtualParameterHandler_InvalidName(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusOK, "", nil))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/virtual-parameters/bad.name", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDeleteVirtualParameterHandler_NBIFailure(t *testing.T) {
	_, router := setupTestServer(t, scriptMockHandler(t, http.StatusInternalServerError, "", nil))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/genieacs/virtual-parameters/getMAC", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
