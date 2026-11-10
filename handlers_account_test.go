package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlers_account_test.go covers the router web login endpoints
// (GET/PUT /account/{ip}) and their pure extraction/build helpers.

// mockDeviceTreeWithAccounts returns a device document exposing every
// account family, including a ZTE duplicate base (to exercise de-dupe)
// and instances with only some fields populated.
func mockDeviceTreeWithAccounts() string {
	lastInform := time.Now().UTC().Format(time.RFC3339)
	zteInfo := `{
        "AdminEnable": {"_value": true},
        "AdminName": {"_value": "admin"},
        "AdminPassword": {"_value": "zte-pass"},
        "UserEnable": {"_value": false},
        "UserName": {"_value": "user"},
        "UserPassword": {"_value": "user-pass"}
    }`
	return fmt.Sprintf(`{
        "_id": "%s",
        "_lastInform": "%s",
        "InternetGatewayDevice": {
            "X_ZTE-COM_UserInterface": {
                "X_ZTE-COM_WebUserInfo": %s
            },
            "UserInterface": {
                "X_ZTE-COM_WebUserInfo": %s,
                "X_HW_WebUserInfo": {
                    "1": {"Enable": {"_value": false}},
                    "2": {"Enable": {"_value": true}, "UserName": {"_value": "admin"}, "Password": {"_value": "hw-pass"}}
                }
            },
            "DeviceInfo": {
                "X_FH_Account": {
                    "X_FH_WebUserInfo": {
                        "Enable": {"_value": true},
                        "WebSuperUsername": {"_value": "telecomadmin"},
                        "WebSuperPassword": {"_value": "fh-super"},
                        "WebUsername": {"_value": "useradmin"},
                        "WebPassword": {"_value": "fh-user"}
                    }
                }
            },
            "User": {
                "1": {"Enable": {"_value": true}, "Username": {"_value": "admin"}, "Password": {"_value": "std-pass"}},
                "2": {"Enable": {"_value": false}},
                "3": {"_object": true}
            }
        }
    }`, mockDeviceID, lastInform, zteInfo, zteInfo)
}

// accountMockHandler serves the device-id projection, the full tree, and
// accepts queued task submissions.
func accountMockHandler(deviceJSON string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("projection"), "_id") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockDeviceResponseWithLastInform()))
			return
		}
		if strings.Contains(r.URL.Query().Get("query"), mockDeviceID) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[" + deviceJSON + "]"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func decodeAccountResponse(t *testing.T, body []byte) AccountResponse {
	t.Helper()
	var env Response
	require.NoError(t, json.Unmarshal(body, &env))
	dataBytes, err := json.Marshal(env.Data)
	require.NoError(t, err)
	var resp AccountResponse
	require.NoError(t, json.Unmarshal(dataBytes, &resp))
	return resp
}

func accountByVendorLabel(accounts []AccountEntry, vendor, label string) (AccountEntry, bool) {
	for _, a := range accounts {
		if a.Vendor == vendor && a.Label == label {
			return a, true
		}
	}
	return AccountEntry{}, false
}

// --- collectAccounts ---

func TestCollectAccounts_AllVendors(t *testing.T) {
	tree := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(mockDeviceTreeWithAccounts()), &tree))

	accounts := collectAccounts(tree)
	// ZTE duplicates collapse to 2, Huawei 2, FiberHome 2, TR-098 2.
	require.Len(t, accounts, 8)

	zteAdmin, ok := accountByVendorLabel(accounts, accountVendorZTE, "Admin")
	require.True(t, ok)
	assert.Equal(t, accountZTEBasePrimary, zteAdmin.Path)
	assert.Equal(t, "admin", zteAdmin.Username)
	assert.Equal(t, "zte-pass", zteAdmin.Password)
	require.NotNil(t, zteAdmin.Enabled)
	assert.True(t, *zteAdmin.Enabled)

	hw2, ok := accountByVendorLabel(accounts, accountVendorHuawei, "Account 2")
	require.True(t, ok)
	assert.Equal(t, accountHuaweiRoot+".2", hw2.Path)
	assert.Equal(t, "hw-pass", hw2.Password)

	fhSuper, ok := accountByVendorLabel(accounts, accountVendorFiberHome, "Super admin")
	require.True(t, ok)
	assert.Equal(t, "telecomadmin", fhSuper.Username)
	assert.Equal(t, "fh-super", fhSuper.Password)

	std2, ok := accountByVendorLabel(accounts, accountVendorTR098, "User 2")
	require.True(t, ok)
	assert.Empty(t, std2.Username)
	require.NotNil(t, std2.Enabled)
	assert.False(t, *std2.Enabled)

	// Instance 3 is metadata-only and must not become a blank row.
	_, ok = accountByVendorLabel(accounts, accountVendorTR098, "User 3")
	assert.False(t, ok)
}

func TestCollectAccounts_EmptyTree(t *testing.T) {
	assert.Empty(t, collectAccounts(nil))
	assert.Empty(t, collectAccounts(map[string]interface{}{}))
	assert.NotNil(t, collectAccounts(nil))
}

func TestDedupeAccounts_MergesMirroredZTE(t *testing.T) {
	tree := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(mockDeviceTreeWithAccounts()), &tree))
	// Only the ZTE families produce entries out of this subtree.
	subtree := map[string]interface{}{
		"InternetGatewayDevice": tree["InternetGatewayDevice"].(map[string]interface{}),
	}
	accounts := collectZTEAccounts(subtree)
	require.Len(t, accounts, 4, "two bases x two roles")
	assert.Len(t, dedupeAccounts(accounts), 2)
}

// --- lookupObject ---

func TestLookupObject(t *testing.T) {
	tree := map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": map[string]interface{}{"_value": "leaf"},
			},
			"S": "scalar",
		},
	}
	_, ok := lookupObject(nil, "A")
	assert.False(t, ok)
	_, ok = lookupObject(tree, "")
	assert.False(t, ok)
	_, ok = lookupObject(tree, "A.Missing")
	assert.False(t, ok)
	_, ok = lookupObject(tree, "A.S")
	assert.False(t, ok, "a scalar is not an object")
	_, ok = lookupObject(tree, "A.S.X")
	assert.False(t, ok, "cannot descend through a scalar")
	node, ok := lookupObject(tree, "A.B")
	assert.True(t, ok)
	assert.Contains(t, node, "C")
}

// --- validateAccountRequest ---

func TestValidateAccountRequest(t *testing.T) {
	assert.Equal(t, "", validateAccountRequest(&SetAccountRequest{Password: "secret"}))
	assert.Equal(t, "", validateAccountRequest(&SetAccountRequest{Username: "admin", Password: "secret"}))
	assert.Equal(t, ErrAccountPasswordRequired, validateAccountRequest(&SetAccountRequest{}))
	assert.Equal(t, ErrAccountPasswordTooLong,
		validateAccountRequest(&SetAccountRequest{Password: strings.Repeat("x", AccountMaxFieldLength+1)}))
	assert.Equal(t, ErrAccountUsernameTooLong,
		validateAccountRequest(&SetAccountRequest{Username: strings.Repeat("x", AccountMaxFieldLength+1), Password: "p"}))
	assert.Equal(t, ErrAccountUsernameWhitespace,
		validateAccountRequest(&SetAccountRequest{Username: "ad min", Password: "p"}))
}

func TestValidateAccountRequest_RowMode(t *testing.T) {
	row := func(mut func(*SetAccountRequest)) *SetAccountRequest {
		req := &SetAccountRequest{Path: accountFiberHomeBase, Label: accountLabelAdmin}
		mut(req)
		return req
	}
	assert.Equal(t, "", validateAccountRequest(row(func(r *SetAccountRequest) { r.Username = "admin" })))
	assert.Equal(t, "", validateAccountRequest(row(func(r *SetAccountRequest) { r.Password = "secret" })))
	enabled := true
	assert.Equal(t, "", validateAccountRequest(row(func(r *SetAccountRequest) { r.Enabled = &enabled })))
	assert.Equal(t, ErrAccountNoFields, validateAccountRequest(row(func(*SetAccountRequest) {})))
}

// --- resolveAccountFields ---

func TestResolveAccountFields(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		label  string
		want   accountFields
		wantOK bool
	}{
		{"ZTE primary admin", accountZTEBasePrimary, accountLabelAdmin, accountFields{"AdminEnable", "AdminName", "AdminPassword"}, true},
		{"ZTE alternate user", accountZTEBaseAlternate, accountLabelUser, accountFields{"UserEnable", "UserName", "UserPassword"}, true},
		{"Huawei instance", accountHuaweiRoot + ".2", accountLabelAccountFmt + "2", accountFields{"Enable", "UserName", "Password"}, true},
		{"FiberHome super", accountFiberHomeBase, accountLabelSuperAdmin, accountFields{"Enable", "WebSuperUsername", "WebSuperPassword"}, true},
		{"FiberHome admin", accountFiberHomeBase, accountLabelAdmin, accountFields{"Enable", "WebUsername", "WebPassword"}, true},
		{"TR-098 instance", accountTR098Root + ".1", accountLabelUserFmt + "1", accountFields{"Enable", "Username", "Password"}, true},
		{"ZTE unknown role", accountZTEBasePrimary, "Operator", accountFields{}, false},
		{"FiberHome unknown role", accountFiberHomeBase, "Operator", accountFields{}, false},
		{"Huawei non-numeric", accountHuaweiRoot + ".x", "Account x", accountFields{}, false},
		{"TR-098 zero instance", accountTR098Root + ".0", "User 0", accountFields{}, false},
		{"TR-098 bare root", accountTR098Root, accountLabelUserFmt + "1", accountFields{}, false},
		{"unknown path", "InternetGatewayDevice.DeviceInfo.Manufacturer", accountLabelAdmin, accountFields{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveAccountFields(tc.path, tc.label)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- buildAccountRowParameterValues ---

func TestBuildAccountRowParameterValues_AllFields(t *testing.T) {
	enabled := false
	req := &SetAccountRequest{
		Path:     accountFiberHomeBase,
		Label:    accountLabelSuperAdmin,
		Username: "telecomadmin",
		Password: "new-pass",
		Enabled:  &enabled,
	}
	fields, ok := resolveAccountFields(req.Path, req.Label)
	require.True(t, ok)
	values := buildAccountRowParameterValues(req, fields)
	require.Len(t, values, 3)
	assert.Equal(t, accountFiberHomeBase+".Enable", values[0][0])
	assert.Equal(t, false, values[0][1])
	assert.Equal(t, XSDBoolean, values[0][2])
	assert.Equal(t, accountFiberHomeBase+".WebSuperUsername", values[1][0])
	assert.Equal(t, accountFiberHomeBase+".WebSuperPassword", values[2][0])
}

func TestBuildAccountRowParameterValues_Subsets(t *testing.T) {
	fields := accountFields{"Enable", "Username", "Password"}
	assert.Empty(t, buildAccountRowParameterValues(&SetAccountRequest{Path: accountTR098Root + ".1"}, fields))
	assert.Len(t, buildAccountRowParameterValues(&SetAccountRequest{Path: accountTR098Root + ".1", Username: "admin"}, fields), 1)
	assert.Len(t, buildAccountRowParameterValues(&SetAccountRequest{Path: accountTR098Root + ".1", Password: "pass"}, fields), 1)
	enabled := true
	assert.Len(t, buildAccountRowParameterValues(&SetAccountRequest{Path: accountTR098Root + ".1", Enabled: &enabled}, fields), 1)
}

// --- buildAccountParameterValues ---

func TestBuildAccountParameterValues_AllVendors(t *testing.T) {
	tree := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(mockDeviceTreeWithAccounts()), &tree))

	values := buildAccountParameterValues(tree, "admin", "new-pass")
	// ZTE 2 bases x 2 accounts x 2 fields, Huawei 2x2, FiberHome 2x2, TR-098 3 instances x 2.
	assert.Len(t, values, 8+4+4+6)

	paths := make(map[string]interface{}, len(values))
	for _, v := range values {
		paths[v[0].(string)] = v[1]
	}
	assert.Equal(t, "new-pass", paths[accountZTEBasePrimary+".AdminPassword"])
	assert.Equal(t, "admin", paths[accountZTEBaseAlternate+".AdminName"])
	assert.Equal(t, "new-pass", paths[accountHuaweiRoot+".1.Password"])
	assert.Equal(t, "new-pass", paths[accountFiberHomeBase+".WebSuperPassword"])
	assert.Equal(t, "admin", paths[accountFiberHomeBase+".WebUsername"])
	assert.Equal(t, "new-pass", paths[accountTR098Root+".3.Password"])
}

func TestBuildAccountParameterValues_PasswordOnly(t *testing.T) {
	tree := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(mockDeviceTreeWithAccounts()), &tree))

	values := buildAccountParameterValues(tree, "", "new-pass")
	assert.Len(t, values, 4+2+2+3)
	for _, v := range values {
		assert.Equal(t, "new-pass", v[1])
	}
}

func TestBuildAccountParameterValues_NoKnownTree(t *testing.T) {
	assert.Empty(t, buildAccountParameterValues(nil, "admin", "pass"))
	assert.Empty(t, buildAccountParameterValues(map[string]interface{}{}, "admin", "pass"))
}

// --- GET /account/{ip} ---

func TestGetAccountHandler_Success(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/account/"+mockDeviceIP, nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	resp := decodeAccountResponse(t, rr.Body.Bytes())
	assert.Equal(t, mockDeviceID, resp.DeviceID)
	assert.Equal(t, mockDeviceIP, resp.IP)
	assert.Len(t, resp.Accounts, 8)
}

func TestGetAccountHandler_NoKnownTree(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(`{"_id":"`+mockDeviceID+`"}`))
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/account/"+mockDeviceIP, nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	resp := decodeAccountResponse(t, rr.Body.Bytes())
	assert.NotNil(t, resp.Accounts)
	assert.Empty(t, resp.Accounts)
}

func TestGetAccountHandler_DeviceNotFound(t *testing.T) {
	mock := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, router := setupTestServer(t, mock)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/account/192.0.2.99", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetAccountHandler_TreeReadFailed(t *testing.T) {
	deviceCacheInstance.clearAll()
	mock := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("projection"), "_id") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockDeviceResponseWithLastInform()))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, router := setupTestServer(t, mock)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/account/"+mockDeviceIP, nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrAccountReadFailed)
}

// --- PUT /account/{ip} ---

func TestSetAccountHandler_Success(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	body := `{"username":"admin","password":"new-pass"}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgAccountUpdated)
	assert.NotContains(t, rr.Body.String(), "new-pass")
}

func TestSetAccountHandler_PasswordOnly(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{"password":"new-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusAccepted, rr.Code)
}

func TestSetAccountHandler_Unsupported(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(`{"_id":"`+mockDeviceID+`"}`))
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{"password":"new-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrCodeAccountUnsupported)
}

func TestSetAccountHandler_RowUpdate(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	body := fmt.Sprintf(`{"path":%q,"label":%q,"username":"root","password":"new-pass","enabled":false}`,
		accountHuaweiRoot+".2", accountLabelAccountFmt+"2")
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)
	assert.Contains(t, rr.Body.String(), MsgAccountUpdated)
}

func TestSetAccountHandler_RowEnabledOnly(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	body := fmt.Sprintf(`{"path":%q,"label":%q,"enabled":true}`,
		accountFiberHomeBase, accountLabelSuperAdmin)
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusAccepted, rr.Code)
}

func TestSetAccountHandler_RowUnknownPath(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	// Shaped like a TR-098 instance but absent from the tree.
	body := fmt.Sprintf(`{"path":%q,"label":"User 9","password":"x"}`, accountTR098Root+".9")
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrAccountPathUnknown)
}

func TestSetAccountHandler_RowUnknownRole(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	body := fmt.Sprintf(`{"path":%q,"label":"Operator","password":"x"}`, accountFiberHomeBase)
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrAccountPathUnknown)
}

func TestSetAccountHandler_RowNoFields(t *testing.T) {
	deviceCacheInstance.clearAll()
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	body := fmt.Sprintf(`{"path":%q,"label":%q}`, accountHuaweiRoot+".2", accountLabelAccountFmt+"2")
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), ErrAccountNoFields)
}

func TestSetAccountHandler_Validation(t *testing.T) {
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSetAccountHandler_BadJSON(t *testing.T) {
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSetAccountHandler_DeviceNotFound(t *testing.T) {
	mock := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, router := setupTestServer(t, mock)
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/192.0.2.99", strings.NewReader(`{"password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestSetAccountHandler_TreeReadFailed(t *testing.T) {
	deviceCacheInstance.clearAll()
	mock := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("projection"), "_id") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockDeviceResponseWithLastInform()))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, router := setupTestServer(t, mock)
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{"password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestSetAccountHandler_WorkerPoolFull(t *testing.T) {
	deviceCacheInstance.clearAll()
	originalPool := taskWorkerPool
	t.Cleanup(func() { taskWorkerPool = originalPool })
	_, router := setupTestServer(t, accountMockHandler(mockDeviceTreeWithAccounts()))
	taskWorkerPool.Stop()
	taskWorkerPool = &workerPool{queue: make(chan task, 0), wg: sync.WaitGroup{}}

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/genieacs/account/"+mockDeviceIP, strings.NewReader(`{"password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}
