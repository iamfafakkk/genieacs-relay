package main

import (
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// handlers_account.go exposes the CPE web login accounts (the credentials an
// operator types into the router's local admin page). Vendors scatter these
// across different TR-098 extensions, so both the read and write side cover
// every family the provisioning presets touch:
//
//	ZTE        InternetGatewayDevice[.X_ZTE-COM_UserInterface].X_ZTE-COM_WebUserInfo
//	Huawei     InternetGatewayDevice.UserInterface.X_HW_WebUserInfo.{n}
//	FiberHome  InternetGatewayDevice.DeviceInfo.X_FH_Account.X_FH_WebUserInfo
//	TR-098     InternetGatewayDevice.User.{n}
//
// These are distinct from PPPoE credentials (/pppoe/{ip}) and from the TR-069
// ACS auth configured on GenieACS itself.

const (
	accountVendorZTE       = "ZTE"
	accountVendorHuawei    = "Huawei"
	accountVendorFiberHome = "FiberHome"
	accountVendorTR098     = "TR-098"

	accountZTEBasePrimary   = "InternetGatewayDevice.X_ZTE-COM_UserInterface.X_ZTE-COM_WebUserInfo"
	accountZTEBaseAlternate = "InternetGatewayDevice.UserInterface.X_ZTE-COM_WebUserInfo"
	accountHuaweiRoot       = "InternetGatewayDevice.UserInterface.X_HW_WebUserInfo"
	accountFiberHomeBase    = "InternetGatewayDevice.DeviceInfo.X_FH_Account.X_FH_WebUserInfo"
	accountTR098Root        = "InternetGatewayDevice.User"
)

// Role labels double as the row identity the UI sends back on a per-account
// update, because ZTE and FiberHome expose several logins on one object path.
const (
	accountLabelAdmin      = "Admin"
	accountLabelUser       = "User"
	accountLabelSuperAdmin = "Super admin"
	accountLabelAccountFmt = "Account "
	accountLabelUserFmt    = "User "
)

// AccountMaxFieldLength caps username and password writes. Mirrors the TR-098
// string(64) schema used by the other credential endpoints.
const AccountMaxFieldLength = 64

var accountZTEBase = []string{accountZTEBasePrimary, accountZTEBaseAlternate}

// AccountEntry is one web login account parsed from the device tree.
//
// @Description One CPE web login account. Vendor identifies the parameter tree the values came from; Path is the concrete object path so operators can trace it back to the tree.
type AccountEntry struct {
	Vendor   string `json:"vendor" example:"ZTE"`
	Label    string `json:"label" example:"Admin"`
	Path     string `json:"path" example:"InternetGatewayDevice.X_ZTE-COM_UserInterface.X_ZTE-COM_WebUserInfo"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Username string `json:"username,omitempty" example:"admin"`
	Password string `json:"password,omitempty" example:"dsnet354"`
}

// AccountResponse is the shape returned by GET /account/{ip}.
//
// @Description Web login accounts exposed by the CPE. Empty when the CPE exposes no known account tree.
type AccountResponse struct {
	DeviceID string         `json:"device_id"`
	IP       string         `json:"ip"`
	Accounts []AccountEntry `json:"accounts"`
}

// SetAccountRequest is the body shape for PUT /account/{ip}.
//
// @Description Router web login update. Without path/label it writes the username and password to every detected account tree (password required). With path/label it updates that single account; then username and password are optional and enabled toggles the account.
type SetAccountRequest struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// Path/Label select one account object returned by GET /account/{ip}.
	Path  string `json:"path,omitempty"`
	Label string `json:"label,omitempty"`
	// Enabled is a pointer so "false" is distinguishable from "not provided".
	Enabled *bool `json:"enabled,omitempty"`
}

// SetAccountResponse is the shape returned by PUT /account/{ip}.
//
// @Description Router web login credential update response. Never echoes the password.
type SetAccountResponse struct {
	Message  string `json:"message"`
	DeviceID string `json:"device_id"`
	IP       string `json:"ip"`
}

var (
	_ = AccountEntry{}
	_ = AccountResponse{}
	_ = SetAccountRequest{}
	_ = SetAccountResponse{}
)

// getAccountHandler returns every web login account the cached device tree
// exposes. Unknown vendors yield an empty list, not an error, so the UI can
// show a plain empty state.
//
//	@Summary		List CPE web login accounts
//	@Description	Reads the CPE router login accounts from the cached device tree, covering the ZTE, Huawei, FiberHome, and TR-098 account parameter trees. Returns an empty list when no known tree is exposed.
//	@Tags			Inspection
//	@Produce		json
//	@Param			ip	path		string	true	"Device IP address"	example(192.168.1.1)
//	@Success		200	{object}	Response{data=AccountResponse}
//	@Failure		400	{object}	Response
//	@Failure		401	{object}	Response
//	@Failure		404	{object}	Response
//	@Failure		429	{object}	Response
//	@Failure		500	{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/account/{ip} [get]
func getAccountHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	tree, err := getDeviceData(r.Context(), deviceID)
	if err != nil {
		logger.Error("Failed to read device tree for accounts",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrAccountReadFailed)
		return
	}
	sendResponse(w, http.StatusOK, AccountResponse{
		DeviceID: deviceID,
		IP:       getIPParam(r),
		Accounts: collectAccounts(tree),
	})
}

// setAccountHandler writes credentials either to one account (path/label in
// the body) or to every account tree the device exposes, mirroring the default
// provisioning preset's "samakan password admin" step. Only detected trees are
// written so a missing vendor extension cannot fault the whole
// SetParameterValues task.
//
//	@Summary		Set CPE web login credentials
//	@Description	Updates the CPE router web login credentials. With path+label it edits that single account (username, password, and/or enabled); without them it writes the username and password to every detected account tree (ZTE, Huawei, FiberHome, TR-098). Returns 404 ACCOUNT_NOT_SUPPORTED when the CPE exposes no known account tree.
//	@Tags			Provisioning
//	@Accept			json
//	@Produce		json
//	@Param			ip		path		string				true	"Device IP address"	example(192.168.1.1)
//	@Param			body	body		SetAccountRequest	true	"New login credentials"
//	@Success		202		{object}	Response{data=SetAccountResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		404		{object}	Response
//	@Failure		429		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/account/{ip} [put]
func setAccountHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	var req SetAccountRequest
	if !ParseJSONRequest(w, r, &req) {
		return
	}
	if errMsg := validateAccountRequest(&req); errMsg != "" {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, errMsg)
		return
	}

	tree, err := getDeviceData(r.Context(), deviceID)
	if err != nil {
		logger.Error("Failed to read device tree for account update",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrAccountReadFailed)
		return
	}
	parameterValues, errMsg := accountUpdateValues(tree, &req)
	if errMsg != "" {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, errMsg)
		return
	}
	if len(parameterValues) == 0 {
		logger.Info("Account update skipped: no known account tree", zap.String("deviceID", deviceID))
		sendError(w, r, http.StatusNotFound, ErrCodeAccountUnsupported, ErrAccountUnsupported)
		return
	}
	if err := SubmitWLANUpdate(deviceID, parameterValues); err != nil {
		logger.Error("Account update task submission failed",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, ErrAccountDispatchFailed)
		return
	}
	deviceCacheInstance.clear(deviceID)
	sendResponse(w, http.StatusAccepted, SetAccountResponse{
		Message:  MsgAccountUpdated,
		DeviceID: deviceID,
		IP:       getIPParam(r),
	})
}

// validateAccountRequest applies field rules. Pure function. A request naming
// a concrete account (path/label) may update any subset of username, password,
// and enabled; the fleet-wide form still requires a password.
func validateAccountRequest(req *SetAccountRequest) string {
	if req.Path != "" {
		if req.Username == "" && req.Password == "" && req.Enabled == nil {
			return ErrAccountNoFields
		}
	} else if req.Password == "" {
		return ErrAccountPasswordRequired
	}
	if len(req.Password) > AccountMaxFieldLength {
		return ErrAccountPasswordTooLong
	}
	if len(req.Username) > AccountMaxFieldLength {
		return ErrAccountUsernameTooLong
	}
	if strings.ContainsAny(req.Username, " \t\n\r") {
		return ErrAccountUsernameWhitespace
	}
	return ""
}

// accountUpdateValues picks the write set: one concrete account object when
// path/label are given, otherwise every account tree the device exposes.
// A non-empty errMsg means the request names an account this device lacks.
func accountUpdateValues(tree map[string]interface{}, req *SetAccountRequest) ([][]interface{}, string) {
	if req.Path == "" {
		return buildAccountParameterValues(tree, req.Username, req.Password), ""
	}
	fields, ok := resolveAccountFields(req.Path, req.Label)
	if !ok {
		return nil, ErrAccountPathUnknown
	}
	if _, ok := lookupObject(tree, req.Path); !ok {
		return nil, ErrAccountPathUnknown
	}
	return buildAccountRowParameterValues(req, fields), ""
}

// accountFields names the leaf fields one account object exposes. ZTE and
// FiberHome pack several logins into a single object, so the role label picks
// the field trio.
type accountFields struct {
	enableField string
	userField   string
	passField   string
}

// resolveAccountFields maps a GET /account row (path + label) back to its
// concrete leaf names. Pure function.
func resolveAccountFields(path, label string) (accountFields, bool) {
	for _, base := range accountZTEBase {
		if path != base {
			continue
		}
		switch label {
		case accountLabelAdmin:
			return accountFields{"AdminEnable", "AdminName", "AdminPassword"}, true
		case accountLabelUser:
			return accountFields{"UserEnable", "UserName", "UserPassword"}, true
		}
		return accountFields{}, false
	}
	if isAccountInstance(path, accountHuaweiRoot) {
		return accountFields{"Enable", "UserName", "Password"}, true
	}
	if path == accountFiberHomeBase {
		switch label {
		case accountLabelSuperAdmin:
			return accountFields{"Enable", "WebSuperUsername", "WebSuperPassword"}, true
		case accountLabelAdmin:
			return accountFields{"Enable", "WebUsername", "WebPassword"}, true
		}
		return accountFields{}, false
	}
	if isAccountInstance(path, accountTR098Root) {
		return accountFields{"Enable", "Username", "Password"}, true
	}
	return accountFields{}, false
}

// isAccountInstance reports whether path is `root.<positive integer>`.
func isAccountInstance(path, root string) bool {
	_, ok := accountInstance(path, root)
	return ok
}

func accountInstance(path, root string) (int, bool) {
	suffix, found := strings.CutPrefix(path, root+".")
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// buildAccountRowParameterValues builds the write set for a single account.
// Empty username/password mean "leave unchanged"; a nil Enabled means the same.
func buildAccountRowParameterValues(req *SetAccountRequest, fields accountFields) [][]interface{} {
	var values [][]interface{}
	if req.Enabled != nil {
		values = append(values, []interface{}{req.Path + "." + fields.enableField, *req.Enabled, XSDBoolean})
	}
	if req.Username != "" {
		values = append(values, []interface{}{req.Path + "." + fields.userField, req.Username, XSDString})
	}
	if req.Password != "" {
		values = append(values, []interface{}{req.Path + "." + fields.passField, req.Password, XSDString})
	}
	return values
}

// collectAccounts extracts every account entry from the tree, de-duplicated
// so a CPE that mirrors the same login under both ZTE base paths shows it once.
func collectAccounts(tree map[string]interface{}) []AccountEntry {
	accounts := make([]AccountEntry, 0, 8)
	accounts = append(accounts, collectZTEAccounts(tree)...)
	accounts = append(accounts, collectHuaweiAccounts(tree)...)
	accounts = append(accounts, collectFiberHomeAccounts(tree)...)
	accounts = append(accounts, collectTR098Accounts(tree)...)
	return dedupeAccounts(accounts)
}

func collectZTEAccounts(tree map[string]interface{}) []AccountEntry {
	var out []AccountEntry
	for _, base := range accountZTEBase {
		if _, ok := lookupObject(tree, base); !ok {
			continue
		}
		out = appendAccountIfPresent(out, tree, accountVendorZTE, accountLabelAdmin, base, "AdminEnable", "AdminName", "AdminPassword")
		out = appendAccountIfPresent(out, tree, accountVendorZTE, accountLabelUser, base, "UserEnable", "UserName", "UserPassword")
	}
	return out
}

func collectHuaweiAccounts(tree map[string]interface{}) []AccountEntry {
	var out []AccountEntry
	for _, n := range EnumerateInstances(tree, accountHuaweiRoot) {
		base := joinInstance(accountHuaweiRoot, n)
		out = appendAccountIfPresent(out, tree, accountVendorHuawei, accountLabelAccountFmt+intToStr(n), base, "Enable", "UserName", "Password")
	}
	return out
}

func collectFiberHomeAccounts(tree map[string]interface{}) []AccountEntry {
	if _, ok := lookupObject(tree, accountFiberHomeBase); !ok {
		return nil
	}
	var out []AccountEntry
	out = appendAccountIfPresent(out, tree, accountVendorFiberHome, accountLabelSuperAdmin, accountFiberHomeBase, "Enable", "WebSuperUsername", "WebSuperPassword")
	out = appendAccountIfPresent(out, tree, accountVendorFiberHome, accountLabelAdmin, accountFiberHomeBase, "Enable", "WebUsername", "WebPassword")
	return out
}

func collectTR098Accounts(tree map[string]interface{}) []AccountEntry {
	var out []AccountEntry
	for _, n := range EnumerateInstances(tree, accountTR098Root) {
		base := joinInstance(accountTR098Root, n)
		out = appendAccountIfPresent(out, tree, accountVendorTR098, accountLabelUserFmt+intToStr(n), base, "Enable", "Username", "Password")
	}
	return out
}

// appendAccountIfPresent adds one entry only when the object exposes at least
// one account field, so a tree node with metadata alone does not become a
// blank row.
func appendAccountIfPresent(out []AccountEntry, tree map[string]interface{}, vendor, label, base, enableField, userField, passField string) []AccountEntry {
	entry := AccountEntry{Vendor: vendor, Label: label, Path: base}
	if v, ok := LookupBool(tree, base+"."+enableField); ok {
		entry.Enabled = &v
	}
	entry.Username, _ = LookupString(tree, base+"."+userField)
	entry.Password, _ = LookupString(tree, base+"."+passField)
	if entry.Enabled == nil && entry.Username == "" && entry.Password == "" {
		return out
	}
	return append(out, entry)
}

// dedupeAccounts collapses entries that describe the same login. Path is
// intentionally excluded: ZTE CPEs mirror the same account under two bases.
func dedupeAccounts(in []AccountEntry) []AccountEntry {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, a := range in {
		key := a.Vendor + "|" + a.Label + "|" + a.Username + "|" + a.Password
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	return out
}

// buildAccountParameterValues builds the SetParameterValues payload for every
// account tree present in the tree. Pure function; an empty result means the
// CPE exposes no known account object.
func buildAccountParameterValues(tree map[string]interface{}, username, password string) [][]interface{} {
	var values [][]interface{}
	add := func(base, userField, passField string) {
		if username != "" {
			values = append(values, []interface{}{base + "." + userField, username, XSDString})
		}
		values = append(values, []interface{}{base + "." + passField, password, XSDString})
	}

	for _, base := range accountZTEBase {
		if _, ok := lookupObject(tree, base); !ok {
			continue
		}
		add(base, "AdminName", "AdminPassword")
		add(base, "UserName", "UserPassword")
	}
	for _, n := range EnumerateInstances(tree, accountHuaweiRoot) {
		add(joinInstance(accountHuaweiRoot, n), "UserName", "Password")
	}
	if _, ok := lookupObject(tree, accountFiberHomeBase); ok {
		add(accountFiberHomeBase, "WebSuperUsername", "WebSuperPassword")
		add(accountFiberHomeBase, "WebUsername", "WebPassword")
	}
	for _, n := range EnumerateInstances(tree, accountTR098Root) {
		add(joinInstance(accountTR098Root, n), "Username", "Password")
	}
	return values
}

// lookupObject walks a dotted path and returns the nested object node. Used
// where the mere existence of an object matters (vendor account trees), as
// opposed to LookupString which expects a leaf `_value`.
func lookupObject(tree map[string]interface{}, path string) (map[string]interface{}, bool) {
	if tree == nil || path == "" {
		return nil, false
	}
	cursor := interface{}(tree)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			continue
		}
		segment := path[start:i]
		start = i + 1
		m, ok := cursor.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, exists := m[segment]
		if !exists {
			return nil, false
		}
		cursor = next
	}
	node, ok := cursor.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return node, true
}
