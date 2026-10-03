package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// handlers_wan.go contains the v2.3.0 WAN connection CRUD endpoints,
// complementing the read-only GET /wan/{ip} in handlers_inspection.go:
//
//	POST   /api/v1/genieacs/wan/{ip}                                            — Add
//	PUT    /api/v1/genieacs/wan/{type}/{wan_device}/{connection_device}/{instance}/{ip}  — Edit
//	DELETE /api/v1/genieacs/wan/{type}/{wan_device}/{connection_device}/{instance}/{ip}  — Delete
//
// Add uses TR-069 AddObject to create a new WANPPPConnection (type
// pppoe) or WANIPConnection (type dhcp/static) instance, then writes the
// requested fields. Edit/Delete address an existing instance by its
// (type, wan_device, connection_device, instance) coordinate — the same
// coordinate the read endpoint now returns.
//
// **Vendor path note:** uses the TR-098 standard path family. TR-181
// (`Device.PPP.Interface.{n}` / `Device.IP.Interface.{n}`) auto-detect is
// a future enhancement (see V2.2.0-DESIGN.md §Phase 5).

// WAN connection type identifiers.
const (
	WANTypePPPoE  = "pppoe"
	WANTypeDHCP   = "dhcp"
	WANTypeStatic = "static"
	// WANTypeIPCP is not a distinct object — it maps to a WANIPConnection
	// with AddressingType=IPCP. The read endpoint surfaces it as "ipcp",
	// so edit/delete must accept it.
	WANTypeIPCP = "ipcp"
)

// wanConnectionObjectName returns the TR-098 partial object path for the
// WAN connection table. No trailing dot: GenieACS builds the
// AddObject/DeleteObject alias itself as `${objectName}.[...]`, so a
// trailing dot yields an empty segment and Path.parse rejects it
// ("Invalid parameter path"). See genieacs lib/cwmp.ts case "addObject".
// The boolean is false for an unknown type.
func wanConnectionObjectName(connType string, wanDevice, connectionDevice int) (string, bool) {
	switch connType {
	case WANTypePPPoE:
		return fmt.Sprintf(
			"InternetGatewayDevice.WANDevice.%d.WANConnectionDevice.%d.WANPPPConnection",
			wanDevice, connectionDevice), true
	case WANTypeDHCP, WANTypeStatic, WANTypeIPCP:
		return fmt.Sprintf(
			"InternetGatewayDevice.WANDevice.%d.WANConnectionDevice.%d.WANIPConnection",
			wanDevice, connectionDevice), true
	}
	return "", false
}

// wanConnectionBase returns the fully-qualified instance path (with
// trailing dot) used as the prefix for SetParameterValues leaf names.
// The boolean is false for an unknown type.
func wanConnectionBase(connType string, wanDevice, connectionDevice, instance int) (string, bool) {
	obj, ok := wanConnectionObjectName(connType, wanDevice, connectionDevice)
	if !ok {
		return "", false
	}
	return obj + "." + strconv.Itoa(instance) + ".", true
}

// validWANCoordinate reports whether all three instance numbers are in
// the 1-8 safety range used across the WAN endpoints.
func validWANCoordinate(wanDevice, connectionDevice, instance int) bool {
	inRange := func(n int) bool { return n >= 1 && n <= PPPoEMaxWANInstance }
	return inRange(wanDevice) && inRange(connectionDevice) && inRange(instance)
}

// --- Add ---

// CreateWANConnectionRequest is the body shape for POST /wan/{ip}.
//
// @Description Create a WAN connection instance. type is required (pppoe, dhcp, or static) and name is required (the operator label that GenieACS uses to seed and distinguish the new instance). wan_device / connection_device default to 1. For pppoe, username and password are seeded; for dhcp/static, the addressing fields are seeded.
type CreateWANConnectionRequest struct {
	Type              string `json:"type"`
	WANDevice         int    `json:"wan_device,omitempty"`
	ConnectionDevice  int    `json:"connection_device,omitempty"`
	Name              string `json:"name,omitempty"`
	Enabled           *bool  `json:"enabled,omitempty"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	ExternalIPAddress string `json:"external_ip_address,omitempty"`
	SubnetMask        string `json:"subnet_mask,omitempty"`
	DefaultGateway    string `json:"default_gateway,omitempty"`
	DNSServers        string `json:"dns_servers,omitempty"`
}

// WANConnectionMutationResponse is returned by all three CRUD endpoints.
//
// @Description WAN connection mutation response — echoes the coordinate that was acted on. For Add, instance is always 0: GenieACS assigns the index and does not report it back, so re-read GET /wan/{ip} to discover it.
type WANConnectionMutationResponse struct {
	Message          string `json:"message"`
	DeviceID         string `json:"device_id"`
	IP               string `json:"ip"`
	Type             string `json:"type"`
	WANDevice        int    `json:"wan_device"`
	ConnectionDevice int    `json:"connection_device"`
	Instance         int    `json:"instance"`
}

var (
	_ = CreateWANConnectionRequest{}
	_ = WANConnectionMutationResponse{}
)

// createWANConnectionHandler creates a new WAN connection instance via
// AddObject, seeding its fields through the AddObject alias.
//
//	@Summary		Add a WAN connection
//	@Description	Creates a new WANPPPConnection (type pppoe) or WANIPConnection (type dhcp/static) on the CPE via TR-069 AddObject. The connection name and addressing/credential fields are seeded through the AddObject alias, so no follow-up SetParameterValues is needed. GenieACS assigns the instance index, so the response returns instance 0 — re-read GET /wan/{ip} to discover it.
//	@Tags			Provisioning
//	@Accept			json
//	@Produce		json
//	@Param			ip		path		string						true	"Device IP address"	example(192.168.1.1)
//	@Param			body	body		CreateWANConnectionRequest	true	"WAN connection to create"
//	@Success		202		{object}	Response{data=WANConnectionMutationResponse}
//	@Failure		400		{object}	Response
//	@Failure		401		{object}	Response
//	@Failure		404		{object}	Response
//	@Failure		429		{object}	Response
//	@Failure		500		{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/wan/{ip} [post]
func createWANConnectionHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	var req CreateWANConnectionRequest
	if !ParseJSONRequest(w, r, &req) {
		return
	}
	if errMsg := validateCreateWANConnectionRequest(&req); errMsg != "" {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, errMsg)
		return
	}

	objName, _ := wanConnectionObjectName(req.Type, req.WANDevice, req.ConnectionDevice)
	seedParams := buildWANCreateSeedParams(req)
	if _, err := addObject(r.Context(), deviceID, objName, seedParams); err != nil {
		recordSyncJob(TaskTypeWANAdd, deviceID, len(seedParams), err)
		logger.Error("WAN AddObject task failed",
			zap.String("deviceID", deviceID), zap.String("type", req.Type), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrWANCreateFailed)
		return
	}
	recordSyncJob(TaskTypeWANAdd, deviceID, len(seedParams), nil)
	deviceCacheInstance.clear(deviceID)

	AuditLogWithFields(AuditEventWANCreate, GetClientIP(r), deviceID, map[string]interface{}{
		"type":              req.Type,
		"wan_device":        req.WANDevice,
		"connection_device": req.ConnectionDevice,
	})

	sendResponse(w, http.StatusAccepted, WANConnectionMutationResponse{
		Message:          MsgWANConnectionCreated,
		DeviceID:         deviceID,
		IP:               getIPParam(r),
		Type:             req.Type,
		WANDevice:        req.WANDevice,
		ConnectionDevice: req.ConnectionDevice,
		Instance:         0,
	})
}

// validateCreateWANConnectionRequest applies field rules and fills
// defaults. Pure function.
func validateCreateWANConnectionRequest(req *CreateWANConnectionRequest) string {
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.Type == WANTypeIPCP {
		// IPCP is an addressing mode of a WANIPConnection, not a
		// standalone create type; use dhcp and set addressing_type later.
		return ErrWANInvalidType
	}
	if _, ok := wanConnectionObjectName(req.Type, 1, 1); !ok {
		return ErrWANInvalidType
	}
	// A unique Name discriminates the new instance from existing siblings
	// in the AddObject alias, so it is required.
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > PPPoEMaxFieldLength {
		return ErrWANInvalidName
	}
	if req.WANDevice == 0 {
		req.WANDevice = 1
	}
	if req.ConnectionDevice == 0 {
		req.ConnectionDevice = 1
	}
	if !validWANCoordinate(req.WANDevice, req.ConnectionDevice, 1) {
		return ErrWANInvalidCoordinate
	}
	if req.Enabled == nil {
		def := true
		req.Enabled = &def
	}
	if len(req.Username) > PPPoEMaxFieldLength {
		return ErrPPPoEUsernameTooLong
	}
	if strings.ContainsAny(req.Username, " \t\n\r") {
		return ErrPPPoEUsernameWhitespace
	}
	if len(req.Password) > PPPoEMaxFieldLength {
		return ErrPPPoEPasswordTooLong
	}
	for _, ipField := range []string{req.ExternalIPAddress, req.SubnetMask, req.DefaultGateway} {
		if ipField != "" && net.ParseIP(ipField) == nil {
			return ErrWANInvalidIP
		}
	}
	return ""
}

// buildWANCreateSeedParams constructs the AddObject alias attributes that
// seed the new instance. Names are leaf-relative to the objectName (NOT
// fully-qualified paths) because GenieACS's `addObject` compiles them into
// an instance selector. Name is always included and must be unique so the
// new instance is distinguishable from existing siblings. Pure function.
func buildWANCreateSeedParams(req CreateWANConnectionRequest) [][]interface{} {
	values := [][]interface{}{
		{"Name", req.Name},
		{"Enable", *req.Enabled, XSDBoolean},
	}

	if req.Type == WANTypePPPoE {
		if req.Username != "" {
			values = append(values, []interface{}{"Username", req.Username})
		}
		if req.Password != "" {
			values = append(values, []interface{}{"Password", req.Password})
		}
		return values
	}

	addressing := "DHCP"
	if req.Type == WANTypeStatic {
		addressing = "Static"
	}
	values = append(values, []interface{}{"AddressingType", addressing})
	optional := map[string]string{
		"ExternalIPAddress": req.ExternalIPAddress,
		"SubnetMask":        req.SubnetMask,
		"DefaultGateway":    req.DefaultGateway,
		"DNSServers":        req.DNSServers,
	}
	for path, val := range optional {
		if val != "" {
			values = append(values, []interface{}{path, val})
		}
	}
	return values
}

// --- Edit ---

// UpdateWANConnectionRequest is the body shape for PUT on a WAN
// instance. All fields are optional (partial update).
//
// @Description Update an existing WAN connection instance. All fields are optional — only the provided fields are written. Includes vendor NAT/VLAN extensions where the CPE exposes them.
type UpdateWANConnectionRequest struct {
	Enabled           *bool   `json:"enabled,omitempty"`
	Username          *string `json:"username,omitempty"`
	Password          *string `json:"password,omitempty"`
	AddressingType    *string `json:"addressing_type,omitempty"`
	ExternalIPAddress *string `json:"external_ip_address,omitempty"`
	SubnetMask        *string `json:"subnet_mask,omitempty"`
	DefaultGateway    *string `json:"default_gateway,omitempty"`
	DNSServers        *string `json:"dns_servers,omitempty"`
	// Vendor extensions. NAT maps to the standard NATEnabled; VLAN maps to
	// Huawei X_HW_VLAN or ZTE X_ZTE-COM_VLANEnable/_VLANID. The handler
	// resolves which vendor params the CPE exposes before writing.
	NATEnabled  *bool `json:"nat_enabled,omitempty"`
	VLANEnabled *bool `json:"vlan_enabled,omitempty"`
	VLANID      *int  `json:"vlan_id,omitempty"`
	// ServiceList replaces the vendor service-list as a set of service
	// tokens (e.g. ["INTERNET","TR069"]). The write path joins them with
	// the CPE's own separator (Huawei '_', everyone else ','). A non-nil
	// empty slice clears the list. Only written when the CPE exposes a
	// service-list param; see resolveServiceListParam.
	ServiceList *[]string `json:"service_list,omitempty"`
}

var _ = UpdateWANConnectionRequest{}

// updateWANConnectionHandler edits fields on an existing WAN connection
// instance via SetParameterValues.
//
//	@Summary		Edit a WAN connection
//	@Description	Updates fields on an existing WAN connection instance via TR-069 SetParameterValues. Partial update — only provided fields are written. The instance is addressed by type + wan_device + connection_device + instance (as returned by GET /wan/{ip}).
//	@Tags			Provisioning
//	@Accept			json
//	@Produce		json
//	@Param			type				path		string						true	"Connection type: pppoe, dhcp, or static"	example(pppoe)
//	@Param			wan_device			path		int							true	"WANDevice instance"						example(1)
//	@Param			connection_device	path		int							true	"WANConnectionDevice instance"				example(1)
//	@Param			instance			path		int							true	"Connection instance"						example(1)
//	@Param			ip					path		string						true	"Device IP address"							example(192.168.1.1)
//	@Param			body				body		UpdateWANConnectionRequest	true	"Fields to update"
//	@Success		202					{object}	Response{data=WANConnectionMutationResponse}
//	@Failure		400					{object}	Response
//	@Failure		401					{object}	Response
//	@Failure		404					{object}	Response
//	@Failure		429					{object}	Response
//	@Failure		500					{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/wan/{type}/{wan_device}/{connection_device}/{instance}/{ip} [put]
func updateWANConnectionHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	connType, wanDevice, connectionDevice, instance, ok := parseWANCoordinate(w, r)
	if !ok {
		return
	}
	base, _ := wanConnectionBase(connType, wanDevice, connectionDevice, instance)

	var req UpdateWANConnectionRequest
	if !ParseJSONRequest(w, r, &req) {
		return
	}
	if !hasUpdateWANFields(req) {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrUpdateFieldRequired)
		return
	}
	if errMsg := validateUpdateWANConnectionRequest(&req, connType); errMsg != "" {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, errMsg)
		return
	}

	// NAT/VLAN are vendor extensions with no standard path, so resolve
	// which concrete params this CPE actually exposes before writing.
	var tree map[string]interface{}
	if req.NATEnabled != nil || req.VLANEnabled != nil || req.VLANID != nil || req.ServiceList != nil {
		t, err := getDeviceData(r.Context(), deviceID)
		if err != nil {
			logger.Error("Failed to read device tree for WAN vendor params",
				zap.String("deviceID", deviceID), zap.Error(err))
			sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrWanReadFailed)
			return
		}
		tree = t
	}

	updateParams := buildUpdateWANConnectionParams(req, base, tree)
	if err := setParameterValues(r.Context(), deviceID, updateParams); err != nil {
		recordSyncJob(TaskTypeWANUpdate, deviceID, len(updateParams), err)
		logger.Error("WAN update task submission failed",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrWANDispatchFailed)
		return
	}
	recordSyncJob(TaskTypeWANUpdate, deviceID, len(updateParams), nil)
	deviceCacheInstance.clear(deviceID)

	AuditLogWithFields(AuditEventWANUpdate, GetClientIP(r), deviceID, map[string]interface{}{
		"type":              connType,
		"wan_device":        wanDevice,
		"connection_device": connectionDevice,
		"instance":          instance,
	})

	sendResponse(w, http.StatusAccepted, WANConnectionMutationResponse{
		Message:          MsgWANConnectionUpdated,
		DeviceID:         deviceID,
		IP:               getIPParam(r),
		Type:             connType,
		WANDevice:        wanDevice,
		ConnectionDevice: connectionDevice,
		Instance:         instance,
	})
}

// hasUpdateWANFields reports whether at least one field was provided.
func hasUpdateWANFields(req UpdateWANConnectionRequest) bool {
	return req.Enabled != nil || req.Username != nil || req.Password != nil ||
		req.AddressingType != nil || req.ExternalIPAddress != nil ||
		req.SubnetMask != nil || req.DefaultGateway != nil || req.DNSServers != nil ||
		req.NATEnabled != nil || req.VLANEnabled != nil || req.VLANID != nil ||
		req.ServiceList != nil
}

// validateUpdateWANConnectionRequest applies field rules for a partial
// update. Pure function.
func validateUpdateWANConnectionRequest(req *UpdateWANConnectionRequest, connType string) string {
	if req.Username != nil && strings.ContainsAny(*req.Username, " \t\n\r") {
		return ErrPPPoEUsernameWhitespace
	}
	if req.Username != nil && len(*req.Username) > PPPoEMaxFieldLength {
		return ErrPPPoEUsernameTooLong
	}
	if req.Password != nil && len(*req.Password) > PPPoEMaxFieldLength {
		return ErrPPPoEPasswordTooLong
	}
	if req.Username != nil && connType != WANTypePPPoE {
		return ErrWANFieldWrongType
	}
	if req.Password != nil && connType != WANTypePPPoE {
		return ErrWANFieldWrongType
	}
	for _, v := range []*string{req.ExternalIPAddress, req.SubnetMask, req.DefaultGateway} {
		if v != nil && *v != "" && net.ParseIP(*v) == nil {
			return ErrWANInvalidIP
		}
	}
	if req.AddressingType != nil {
		at := strings.ToLower(strings.TrimSpace(*req.AddressingType))
		if at != "dhcp" && at != "static" && at != "ipcp" {
			return ErrWANInvalidAddressing
		}
	}
	if req.VLANID != nil && (*req.VLANID < MinVLANID || *req.VLANID > MaxVLANID) {
		return ErrWANInvalidVLANID
	}
	if req.ServiceList != nil {
		if len(joinServiceTokens(ParamZTEServiceList, *req.ServiceList)) > MaxServiceListLength {
			return ErrWANServiceListTooLong
		}
		for _, tok := range *req.ServiceList {
			if !validServiceToken(tok) {
				return ErrWANInvalidServiceList
			}
		}
	}
	return ""
}

// validServiceToken reports whether a service token is a plain identifier
// (letters/digits/hyphen). Keeps ',' and '_' out of tokens so they can't
// corrupt the vendor's list separator.
func validServiceToken(tok string) bool {
	if tok == "" || len(tok) > 32 {
		return false
	}
	for _, r := range tok {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// buildUpdateWANConnectionParams constructs the SetParameterValues
// payload for a partial update. tree is the device tree, used to resolve
// vendor NAT/VLAN params (may be nil when none of those fields are set).
// Pure function.
func buildUpdateWANConnectionParams(req UpdateWANConnectionRequest, base string, tree map[string]interface{}) [][]interface{} {
	var values [][]interface{}
	add := func(path string, val interface{}, typ string) {
		values = append(values, []interface{}{base + path, val, typ})
	}
	if req.Enabled != nil {
		add("Enable", *req.Enabled, XSDBoolean)
	}
	if req.Username != nil {
		add("Username", *req.Username, XSDString)
	}
	if req.Password != nil {
		add("Password", *req.Password, XSDString)
	}
	if req.AddressingType != nil {
		add("AddressingType", canonicalAddressingType(*req.AddressingType), XSDString)
	}
	if req.ExternalIPAddress != nil {
		add("ExternalIPAddress", *req.ExternalIPAddress, XSDString)
	}
	if req.SubnetMask != nil {
		add("SubnetMask", *req.SubnetMask, XSDString)
	}
	if req.DefaultGateway != nil {
		add("DefaultGateway", *req.DefaultGateway, XSDString)
	}
	if req.DNSServers != nil {
		add("DNSServers", *req.DNSServers, XSDString)
	}

	// Vendor NAT/VLAN. NAT is standard NATEnabled. VLAN has two shapes:
	// ZTE splits enable + id, Huawei uses a single X_HW_VLAN where 0 means
	// "no tag". The tree tells us which one this CPE exposes; if it exposes
	// neither, VLAN edits are best-effort no-ops (nothing to write).
	if req.NATEnabled != nil {
		add(ParamNATEnabled, *req.NATEnabled, XSDBoolean)
	}
	if req.VLANEnabled != nil || req.VLANID != nil {
		zteEnable := hasParam(tree, base, ParamZTEVLANEnable)
		zteID := hasParam(tree, base, ParamZTEVLANID)
		hw := hasParam(tree, base, ParamHuaweiVLAN)
		switch {
		case zteEnable || zteID:
			if req.VLANEnabled != nil {
				add(ParamZTEVLANEnable, *req.VLANEnabled, XSDBoolean)
			}
			if req.VLANID != nil {
				add(ParamZTEVLANID, *req.VLANID, XSDUnsignedInt)
			}
		case hw:
			// Collapse enable+id into one value: a disabled VLAN (or a
			// missing id) becomes 0. Re-enabling without an explicit id
			// reuses the CPE's current VLAN id.
			id := 0
			if req.VLANID != nil {
				id = *req.VLANID
			} else if req.VLANEnabled != nil && *req.VLANEnabled {
				if cur, ok := LookupInt(tree, base+ParamHuaweiVLAN); ok {
					id = cur
				}
			}
			if req.VLANEnabled != nil && !*req.VLANEnabled {
				id = 0
			}
			add(ParamHuaweiVLAN, id, XSDUnsignedInt)
		}
	}
	if req.ServiceList != nil {
		if leaf := resolveServiceListParam(tree, base); leaf != "" {
			// Huawei's X_HW_SERVICELIST is a single service tag; a
			// multi-token value wedges the CPE's task queue (verified).
			// Only the first token is honored there.
			tokens := *req.ServiceList
			if leaf == ParamHuaweiServiceList && len(tokens) > 1 {
				tokens = tokens[:1]
			}
			add(leaf, joinServiceTokens(leaf, tokens), XSDString)
		}
	}
	return values
}

// joinServiceTokens joins service tokens with the separator the CPE uses
// for the given service-list leaf. Empirically: ZTE X_ZTE-COM_ServiceList
// uses '_' (e.g. INTERNET_TR069_VoIP), CMCC uses ',' (TR069,INTERNET).
// Huawei X_HW_SERVICELIST accepts a single token only and is guarded
// before this is called.
func joinServiceTokens(leaf string, tokens []string) string {
	sep := ","
	if leaf == ParamZTEServiceList {
		sep = "_"
	}
	return strings.Join(tokens, sep)
}

// resolveServiceListParam returns the service-list leaf name this CPE
// exposes under base, or "" if none. Vendor preference order matches the
// read path so both sides agree on which param is authoritative. Handles
// base paths with or without a trailing dot (the read path has none, the
// CRUD builder has one).
func resolveServiceListParam(tree map[string]interface{}, base string) string {
	for _, leaf := range []string{
		ParamHuaweiServiceList,
		ParamZTEServiceList,
		ParamCMCCServiceList,
		ParamCTServiceList,
		ParamCUServiceList,
	} {
		if hasNode(tree, joinParam(base, leaf)) {
			return leaf
		}
	}
	return ""
}

// joinParam appends a leaf to a base path, tolerating a base that already
// ends with '.' (the CRUD builder) or not (the inspection extractors).
func joinParam(base, leaf string) string {
	return strings.TrimSuffix(base, ".") + "." + leaf
}

// hasParam reports whether a leaf name exists under base in the device
// tree. Used to detect which vendor's NAT/VLAN extension a CPE exposes.
func hasParam(tree map[string]interface{}, base, leaf string) bool {
	if tree == nil {
		return false
	}
	_, ok := LookupValue(tree, joinParam(base, leaf))
	return ok
}

// hasNode reports whether a dotted path resolves to a node in the tree,
// even when the parameter carries no `_value` yet (an empty/unreported
// param still exists as a node). LookupValue requires `_value`; this does
// not, so a supported-but-blank param is still detected.
func hasNode(tree map[string]interface{}, path string) bool {
	if tree == nil || path == "" {
		return false
	}
	cursor := interface{}(tree)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			continue
		}
		m, ok := cursor.(map[string]interface{})
		if !ok {
			return false
		}
		next, exists := m[path[start:i]]
		if !exists {
			return false
		}
		cursor = next
		start = i + 1
	}
	_, ok := cursor.(map[string]interface{})
	return ok
}

// canonicalAddressingType maps the lowercase API token to the TR-098
// AddressingType enum value.
func canonicalAddressingType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "static":
		return "Static"
	case "ipcp":
		return "IPCP"
	default:
		return "DHCP"
	}
}

// --- Delete ---

// deleteWANConnectionHandler removes a WAN connection instance via
// DeleteObject.
//
//	@Summary		Delete a WAN connection
//	@Description	Removes a WAN connection instance via TR-069 DeleteObject. The instance is addressed by type + wan_device + connection_device + instance (as returned by GET /wan/{ip}). Deleting a WAN connection drops the customer's link until a new one is provisioned.
//	@Tags			Provisioning
//	@Produce		json
//	@Param			type				path		string	true	"Connection type: pppoe, dhcp, or static"	example(pppoe)
//	@Param			wan_device			path		int		true	"WANDevice instance"						example(1)
//	@Param			connection_device	path		int		true	"WANConnectionDevice instance"				example(1)
//	@Param			instance			path		int		true	"Connection instance"						example(1)
//	@Param			ip					path		string	true	"Device IP address"							example(192.168.1.1)
//	@Success		202					{object}	Response{data=WANConnectionMutationResponse}
//	@Failure		400					{object}	Response
//	@Failure		401					{object}	Response
//	@Failure		404					{object}	Response
//	@Failure		429					{object}	Response
//	@Failure		500					{object}	Response
//	@Security		ApiKeyAuth
//	@Router			/wan/{type}/{wan_device}/{connection_device}/{instance}/{ip} [delete]
func deleteWANConnectionHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := ExtractDeviceIDByIP(w, r)
	if !ok {
		return
	}
	connType, wanDevice, connectionDevice, instance, ok := parseWANCoordinate(w, r)
	if !ok {
		return
	}
	base, _ := wanConnectionBase(connType, wanDevice, connectionDevice, instance)

	// GenieACS parses deleteObject.objectName as a full path; a trailing
	// dot leaves an empty segment and is rejected ("Invalid parameter
	// path"). See genieacs docs/api-reference.rst deleteObject example.
	if err := deleteObject(r.Context(), deviceID, strings.TrimSuffix(base, ".")); err != nil {
		recordSyncJob(TaskTypeWANDelete, deviceID, 0, err)
		logger.Error("WAN DeleteObject task failed",
			zap.String("deviceID", deviceID), zap.Error(err))
		sendError(w, r, http.StatusInternalServerError, ErrCodeGenieACS, ErrWANDeleteFailed)
		return
	}
	recordSyncJob(TaskTypeWANDelete, deviceID, 0, nil)
	deviceCacheInstance.clear(deviceID)

	AuditLogWithFields(AuditEventWANDelete, GetClientIP(r), deviceID, map[string]interface{}{
		"type":              connType,
		"wan_device":        wanDevice,
		"connection_device": connectionDevice,
		"instance":          instance,
	})

	sendResponse(w, http.StatusAccepted, WANConnectionMutationResponse{
		Message:          MsgWANConnectionDeleted,
		DeviceID:         deviceID,
		IP:               getIPParam(r),
		Type:             connType,
		WANDevice:        wanDevice,
		ConnectionDevice: connectionDevice,
		Instance:         instance,
	})
}

// parseWANCoordinate reads and validates the type + instance path
// params, writing a 400 and returning ok=false on any bad value.
func parseWANCoordinate(w http.ResponseWriter, r *http.Request) (connType string, wanDevice, connectionDevice, instance int, ok bool) {
	connType = strings.ToLower(strings.TrimSpace(chi.URLParam(r, "type")))
	if _, valid := wanConnectionObjectName(connType, 1, 1); !valid {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrWANInvalidType)
		return "", 0, 0, 0, false
	}
	var err error
	if wanDevice, err = strconv.Atoi(chi.URLParam(r, "wan_device")); err != nil {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrWANInvalidCoordinate)
		return "", 0, 0, 0, false
	}
	if connectionDevice, err = strconv.Atoi(chi.URLParam(r, "connection_device")); err != nil {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrWANInvalidCoordinate)
		return "", 0, 0, 0, false
	}
	if instance, err = strconv.Atoi(chi.URLParam(r, "instance")); err != nil {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrWANInvalidCoordinate)
		return "", 0, 0, 0, false
	}
	if !validWANCoordinate(wanDevice, connectionDevice, instance) {
		sendError(w, r, http.StatusBadRequest, ErrCodeValidation, ErrWANInvalidCoordinate)
		return "", 0, 0, 0, false
	}
	return connType, wanDevice, connectionDevice, instance, true
}
