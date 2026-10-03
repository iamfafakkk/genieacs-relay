package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlers_wan_test.go covers the v2.3.0 WAN connection CRUD pure
// functions: coordinate/path building, validation, and param payloads.

func TestWANConnectionObjectName(t *testing.T) {
	obj, ok := wanConnectionObjectName(WANTypePPPoE, 1, 2)
	require.True(t, ok)
	assert.Equal(t, "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.2.WANPPPConnection", obj)

	obj, ok = wanConnectionObjectName(WANTypeDHCP, 3, 1)
	require.True(t, ok)
	assert.Equal(t, "InternetGatewayDevice.WANDevice.3.WANConnectionDevice.1.WANIPConnection", obj)

	// IPCP edits/deletes an existing WANIPConnection instance.
	obj, ok = wanConnectionObjectName(WANTypeIPCP, 1, 1)
	require.True(t, ok)
	assert.Equal(t, "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANIPConnection", obj)

	_, ok = wanConnectionObjectName("bogus", 1, 1)
	assert.False(t, ok)
}

func TestWANConnectionBase(t *testing.T) {
	base, ok := wanConnectionBase(WANTypeStatic, 1, 1, 4)
	require.True(t, ok)
	assert.Equal(t, "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANIPConnection.4.", base)

	_, ok = wanConnectionBase("bogus", 1, 1, 1)
	assert.False(t, ok)
}

func TestValidWANCoordinate(t *testing.T) {
	assert.True(t, validWANCoordinate(1, 1, 1))
	assert.True(t, validWANCoordinate(8, 8, 8))
	assert.False(t, validWANCoordinate(0, 1, 1))
	assert.False(t, validWANCoordinate(1, 9, 1))
	assert.False(t, validWANCoordinate(1, 1, -1))
}

func TestValidateCreateWANConnectionRequest_DefaultsAndErrors(t *testing.T) {
	t.Run("defaults applied", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: "PPPoE", Name: "wan-test"}
		assert.Equal(t, "", validateCreateWANConnectionRequest(&req))
		assert.Equal(t, WANTypePPPoE, req.Type)
		assert.Equal(t, 1, req.WANDevice)
		assert.Equal(t, 1, req.ConnectionDevice)
		require.NotNil(t, req.Enabled)
		assert.True(t, *req.Enabled)
	})

	t.Run("missing name", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: WANTypePPPoE}
		assert.Equal(t, ErrWANInvalidName, validateCreateWANConnectionRequest(&req))
	})

	t.Run("invalid type", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: "lte", Name: "wan-test"}
		assert.Equal(t, ErrWANInvalidType, validateCreateWANConnectionRequest(&req))
	})

	t.Run("ipcp rejected on create", func(t *testing.T) {
		// IPCP is an addressing mode of a WANIPConnection, not a create type.
		req := CreateWANConnectionRequest{Type: WANTypeIPCP, Name: "wan-test"}
		assert.Equal(t, ErrWANInvalidType, validateCreateWANConnectionRequest(&req))
	})

	t.Run("invalid coordinate", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: WANTypeDHCP, Name: "wan-test", WANDevice: 9}
		assert.Equal(t, ErrWANInvalidCoordinate, validateCreateWANConnectionRequest(&req))
	})

	t.Run("invalid static IP", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: WANTypeStatic, Name: "wan-test", ExternalIPAddress: "not-an-ip"}
		assert.Equal(t, ErrWANInvalidIP, validateCreateWANConnectionRequest(&req))
	})

	t.Run("username whitespace", func(t *testing.T) {
		req := CreateWANConnectionRequest{Type: WANTypePPPoE, Name: "wan-test", Username: "bad user"}
		assert.Equal(t, ErrPPPoEUsernameWhitespace, validateCreateWANConnectionRequest(&req))
	})
}

func TestBuildWANCreateSeedParams(t *testing.T) {
	t.Run("pppoe", func(t *testing.T) {
		enabled := true
		req := CreateWANConnectionRequest{
			Type: WANTypePPPoE, Name: "5_INTERNET_R_VID_10",
			Enabled: &enabled, Username: "cust", Password: "secret",
		}
		params := buildWANCreateSeedParams(req)
		require.Len(t, params, 4)
		// Leaf-relative names, NOT fully-qualified paths — GenieACS compiles
		// these into the AddObject instance alias.
		assert.Equal(t, "Name", params[0][0])
		assert.Equal(t, "5_INTERNET_R_VID_10", params[0][1])
		assert.Equal(t, "Enable", params[1][0])
		assert.Equal(t, true, params[1][1])
		assert.Equal(t, "Username", params[2][0])
		assert.Equal(t, "Password", params[3][0])
	})

	t.Run("static addressing", func(t *testing.T) {
		enabled := true
		req := CreateWANConnectionRequest{
			Type: WANTypeStatic, Name: "wan-static",
			Enabled: &enabled, ExternalIPAddress: "203.0.113.5",
		}
		params := buildWANCreateSeedParams(req)
		found := false
		for _, p := range params {
			if p[0] == "AddressingType" {
				assert.Equal(t, "Static", p[1])
				found = true
			}
		}
		assert.True(t, found, "AddressingType must be seeded")
	})
}

func TestValidateUpdateWANConnectionRequest(t *testing.T) {
	username := "cust@isp"
	assert.Equal(t, "", validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{Username: &username}, WANTypePPPoE))
	assert.Equal(t, ErrWANFieldWrongType, validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{Username: &username}, WANTypeDHCP))

	badIP := "1.2.3"
	assert.Equal(t, ErrWANInvalidIP, validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{DefaultGateway: &badIP}, WANTypeDHCP))

	badAddr := "pppoe"
	assert.Equal(t, ErrWANInvalidAddressing, validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{AddressingType: &badAddr}, WANTypeDHCP))
}

func TestBuildUpdateWANConnectionParams(t *testing.T) {
	enabled := false
	user := "newuser"
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1."
	params := buildUpdateWANConnectionParams(UpdateWANConnectionRequest{Enabled: &enabled, Username: &user}, base, nil)
	require.Len(t, params, 2)
	assert.Equal(t, base+"Enable", params[0][0])
	assert.Equal(t, base+"Username", params[1][0])
}

// vendorTree builds a minimal tree exposing a single vendor param leaf.
func vendorTree(base, leaf string, val interface{}) map[string]interface{} {
	// base is a dotted path (may have a trailing dot); build nested maps for
	// everything but the leaf.
	parts := strings.Split(strings.TrimSuffix(base, ".")+"."+leaf, ".")
	var tree map[string]interface{} = map[string]interface{}{}
	cur := tree
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = map[string]interface{}{"_value": val}
			break
		}
		next := map[string]interface{}{}
		cur[p] = next
		cur = next
	}
	return tree
}

func TestBuildUpdateWANConnectionParams_NAT(t *testing.T) {
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.2.WANPPPConnection.1."
	on := true
	params := buildUpdateWANConnectionParams(UpdateWANConnectionRequest{NATEnabled: &on}, base, nil)
	require.Len(t, params, 1)
	assert.Equal(t, base+"NATEnabled", params[0][0])
	assert.Equal(t, true, params[0][1])
	assert.Equal(t, XSDBoolean, params[0][2])
}

func TestBuildUpdateWANConnectionParams_HuaweiVLAN(t *testing.T) {
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.2.WANPPPConnection.1."
	tree := vendorTree(base, ParamHuaweiVLAN, 90)

	// Enable with an id → single collapsed X_HW_VLAN=90.
	on := true
	id := 90
	params := buildUpdateWANConnectionParams(
		UpdateWANConnectionRequest{VLANEnabled: &on, VLANID: &id}, base, tree)
	require.Len(t, params, 1)
	assert.Equal(t, base+ParamHuaweiVLAN, params[0][0])
	assert.Equal(t, 90, params[0][1])
	assert.Equal(t, XSDUnsignedInt, params[0][2])

	// Disable → X_HW_VLAN=0 even though an id was sent.
	off := false
	params = buildUpdateWANConnectionParams(
		UpdateWANConnectionRequest{VLANEnabled: &off, VLANID: &id}, base, tree)
	require.Len(t, params, 1)
	assert.Equal(t, 0, params[0][1])

	// Re-enable with no id → reuses the CPE's current X_HW_VLAN.
	params = buildUpdateWANConnectionParams(
		UpdateWANConnectionRequest{VLANEnabled: &on}, base, tree)
	require.Len(t, params, 1)
	assert.Equal(t, 90, params[0][1])
}

func TestBuildUpdateWANConnectionParams_ZTEVLAN(t *testing.T) {
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1."
	tree := vendorTree(base, ParamZTEVLANEnable, true)
	// ZTE also exposes the id leaf; merge both trees.
	more := vendorTree(base, ParamZTEVLANID, 90)
	merge(more, tree)

	on := true
	id := 90
	params := buildUpdateWANConnectionParams(
		UpdateWANConnectionRequest{VLANEnabled: &on, VLANID: &id}, base, tree)
	require.Len(t, params, 2)
	assert.Equal(t, base+ParamZTEVLANEnable, params[0][0])
	assert.Equal(t, true, params[0][1])
	assert.Equal(t, base+ParamZTEVLANID, params[1][0])
	assert.Equal(t, 90, params[1][1])
}

func TestBuildUpdateWANConnectionParams_VLANNoVendorParam(t *testing.T) {
	// CPE exposes neither vendor shape → no VLAN params written (no-op).
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1."
	on := true
	params := buildUpdateWANConnectionParams(UpdateWANConnectionRequest{VLANEnabled: &on}, base, map[string]interface{}{})
	assert.Empty(t, params)
}

func TestValidateUpdateWANConnectionRequest_VLANID(t *testing.T) {
	bad := 5000
	assert.Equal(t, ErrWANInvalidVLANID,
		validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{VLANID: &bad}, WANTypePPPoE))
	ok := 90
	assert.Equal(t, "",
		validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{VLANID: &ok}, WANTypePPPoE))
}

// merge shallow-merges src into dst recursively (test helper).
func merge(dst, src map[string]interface{}) {
	for k, v := range src {
		if m, ok := v.(map[string]interface{}); ok {
			if dm, ok := dst[k].(map[string]interface{}); ok {
				merge(dm, m)
				continue
			}
		}
		dst[k] = v
	}
}

func TestCanonicalAddressingType(t *testing.T) {
	assert.Equal(t, "Static", canonicalAddressingType("static"))
	assert.Equal(t, "IPCP", canonicalAddressingType("IPCP"))
	assert.Equal(t, "DHCP", canonicalAddressingType("dhcp"))
	assert.Equal(t, "DHCP", canonicalAddressingType("whatever"))
}

// The read model must expose the coordinate CRUD addresses instances by.
func TestBuildWanConnectionsResponse_IncludesCoordinates(t *testing.T) {
	tree := map[string]interface{}{
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"2": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"3": map[string]interface{}{
							"WANPPPConnection": map[string]interface{}{
								"4": map[string]interface{}{
									"ConnectionStatus": map[string]interface{}{"_value": "Connected"},
								},
							},
						},
					},
				},
			},
		},
	}
	resp := buildWanConnectionsResponse(tree, "dev", "10.0.0.1")
	require.Len(t, resp.WANConnections, 1)
	assert.Equal(t, 2, resp.WANConnections[0].WANDevice)
	assert.Equal(t, 3, resp.WANConnections[0].ConnectionDevice)
	assert.Equal(t, 4, resp.WANConnections[0].Instance)
}

func TestResolveServiceListParam(t *testing.T) {
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1."
	hw := vendorTree(base, ParamHuaweiServiceList, "INTERNET")
	assert.Equal(t, ParamHuaweiServiceList, resolveServiceListParam(hw, base))

	zte := vendorTree(base, ParamZTEServiceList, "INTERNET")
	assert.Equal(t, ParamZTEServiceList, resolveServiceListParam(zte, base))

	assert.Equal(t, "", resolveServiceListParam(map[string]interface{}{}, base))
}

func TestJoinServiceTokens(t *testing.T) {
	assert.Equal(t, "INTERNET_TR069", joinServiceTokens(ParamZTEServiceList, []string{"INTERNET", "TR069"}))
	assert.Equal(t, "TR069,INTERNET", joinServiceTokens(ParamCMCCServiceList, []string{"TR069", "INTERNET"}))
	assert.Equal(t, "", joinServiceTokens(ParamZTEServiceList, nil))
}

func TestBuildUpdateWANConnectionParams_ServiceList(t *testing.T) {
	base := "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1."
	tree := vendorTree(base, ParamHuaweiServiceList, "INTERNET")
	// Huawei accepts a single tag; a multi-token request is reduced to the
	// first token (a joined value wedges its task queue).
	tokens := []string{"TR069", "INTERNET"}
	params := buildUpdateWANConnectionParams(UpdateWANConnectionRequest{ServiceList: &tokens}, base, tree)
	require.Len(t, params, 1)
	assert.Equal(t, base+ParamHuaweiServiceList, params[0][0])
	assert.Equal(t, "TR069", params[0][1])
	assert.Equal(t, XSDString, params[0][2])

	// ZTE joins with '_'.
	zte := vendorTree(base, ParamZTEServiceList, "INTERNET")
	params = buildUpdateWANConnectionParams(UpdateWANConnectionRequest{ServiceList: &tokens}, base, zte)
	require.Len(t, params, 1)
	assert.Equal(t, "TR069_INTERNET", params[0][1])

	// CPE exposes no service-list param → no-op.
	params = buildUpdateWANConnectionParams(UpdateWANConnectionRequest{ServiceList: &tokens}, base, map[string]interface{}{})
	assert.Empty(t, params)
}

func TestValidateUpdateWANConnectionRequest_ServiceList(t *testing.T) {
	bad := []string{"TR069,INTERNET"}
	assert.Equal(t, ErrWANInvalidServiceList,
		validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{ServiceList: &bad}, WANTypePPPoE))
	ok := []string{"TR069", "INTERNET"}
	assert.Equal(t, "",
		validateUpdateWANConnectionRequest(&UpdateWANConnectionRequest{ServiceList: &ok}, WANTypePPPoE))
}
