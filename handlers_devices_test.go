package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handlers_devices_test.go covers M4 (listDevicesHandler) and
// M5 (searchDevicesHandler) plus their pure helper functions.

// --- buildDevicesListFilter ---

func TestBuildDevicesListFilter_Empty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices", nil)
	filter := buildDevicesListFilter(r)
	assert.Empty(t, filter)
}

func TestBuildDevicesListFilter_Model(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?model=F670L", nil)
	filter := buildDevicesListFilter(r)
	require.Contains(t, filter, "InternetGatewayDevice.DeviceInfo.ModelName._value")
	clause := filter["InternetGatewayDevice.DeviceInfo.ModelName._value"].(map[string]interface{})
	assert.Equal(t, "F670L", clause["$regex"])
}

func TestBuildDevicesListFilter_PPPoEUsername(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?pppoe_username=cust-001", nil)
	filter := buildDevicesListFilter(r)
	or, ok := filter["$or"].([]map[string]interface{})
	require.True(t, ok, "pppoe_username must produce a top-level $or")
	assert.Contains(t, or[0],
		"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username._value")
}

func TestBuildDevicesListFilter_Search(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?search=F670L", nil)
	filter := buildDevicesListFilter(r)
	or, ok := filter["$or"].([]map[string]interface{})
	require.True(t, ok, "search must produce a top-level $or")
	// model + serial + MAC + PPPoE instance grid + _id
	require.Len(t, or, 3+wanConnectionDeviceInstances*wanPPPConnectionInstances+1)
	assert.Equal(t, map[string]interface{}{"$regex": "F670L"}, or[0]["InternetGatewayDevice.DeviceInfo.ModelName._value"])
}

func TestBuildDevicesListFilter_SearchWithOtherFilter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?model=F670L&search=cust", nil)
	filter := buildDevicesListFilter(r)
	and, ok := filter["$and"].([]map[string]interface{})
	require.True(t, ok, "search + model must be combined with $and")
	require.Len(t, and, 2)
	assert.Contains(t, and[0], "InternetGatewayDevice.DeviceInfo.ModelName._value")
	assert.Contains(t, and[1], "$or")
}

func TestBuildDevicesListFilter_SearchQuotesRegex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?search=a.b", nil)
	filter := buildDevicesListFilter(r)
	or := filter["$or"].([]map[string]interface{})
	clause := or[0]["InternetGatewayDevice.DeviceInfo.ModelName._value"].(map[string]interface{})
	assert.Equal(t, `a\.b`, clause["$regex"])
}

func TestBuildDevicesListFilter_OnlineWithStaleThreshold(t *testing.T) {
	original := staleThreshold
	staleThreshold = 600 * 1000 * 1000 * 1000 // 10 minutes in ns
	t.Cleanup(func() { staleThreshold = original })

	r := httptest.NewRequest(http.MethodGet, "/devices?online=true", nil)
	filter := buildDevicesListFilter(r)
	require.Contains(t, filter, "_lastInform")
}

func TestBuildDevicesListFilter_OnlineNoThreshold(t *testing.T) {
	original := staleThreshold
	staleThreshold = 0
	t.Cleanup(func() { staleThreshold = original })

	r := httptest.NewRequest(http.MethodGet, "/devices?online=true", nil)
	filter := buildDevicesListFilter(r)
	assert.NotContains(t, filter, "_lastInform")
}

func TestBuildDevicesListFilter_AllFilters(t *testing.T) {
	original := staleThreshold
	staleThreshold = 600 * 1000 * 1000 * 1000
	t.Cleanup(func() { staleThreshold = original })

	r := httptest.NewRequest(http.MethodGet,
		"/devices?model=F670L&pppoe_username=cust&online=true", nil)
	filter := buildDevicesListFilter(r)
	assert.Len(t, filter, 3)
}

// --- parseDevicesPagination ---

func TestParseDevicesPagination_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices", nil)
	rr := httptest.NewRecorder()
	page, pageSize, ok := parseDevicesPagination(rr, r)
	assert.True(t, ok)
	assert.Equal(t, 1, page)
	assert.Equal(t, DefaultDevicesPageSize, pageSize)
}

func TestParseDevicesPagination_Explicit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?page=3&page_size=20", nil)
	rr := httptest.NewRecorder()
	page, pageSize, ok := parseDevicesPagination(rr, r)
	assert.True(t, ok)
	assert.Equal(t, 3, page)
	assert.Equal(t, 20, pageSize)
}

func TestParseDevicesPagination_BadPage(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?page=0", nil)
	rr := httptest.NewRecorder()
	_, _, ok := parseDevicesPagination(rr, r)
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestParseDevicesPagination_NonNumericPage(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?page=abc", nil)
	rr := httptest.NewRecorder()
	_, _, ok := parseDevicesPagination(rr, r)
	assert.False(t, ok)
}

func TestParseDevicesPagination_BadPageSize(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?page_size=999", nil)
	rr := httptest.NewRecorder()
	_, _, ok := parseDevicesPagination(rr, r)
	assert.False(t, ok)
}

func TestParseDevicesPagination_NonNumericPageSize(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/devices?page_size=xyz", nil)
	rr := httptest.NewRecorder()
	_, _, ok := parseDevicesPagination(rr, r)
	assert.False(t, ok)
}

// --- buildDevicesSearchFilter ---

func TestBuildDevicesSearchFilter_MAC(t *testing.T) {
	filter := buildDevicesSearchFilter("AA:BB:CC:DD:EE:FF", "", "")
	assert.Equal(t, "AA:BB:CC:DD:EE:FF",
		filter["InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.MACAddress._value"])
}

func TestBuildDevicesSearchFilter_Serial(t *testing.T) {
	filter := buildDevicesSearchFilter("", "ZTEGCFLN12345678", "")
	// Serial search uses $or: match TR-069 param OR _id suffix
	orClauses, ok := filter["$or"].([]map[string]interface{})
	require.True(t, ok, "$or clause must be present")
	require.Len(t, orClauses, 2)
	assert.Equal(t, "ZTEGCFLN12345678",
		orClauses[0]["InternetGatewayDevice.DeviceInfo.SerialNumber._value"])
	idClause := orClauses[1]["_id"].(map[string]interface{})
	assert.Equal(t, "ZTEGCFLN12345678", idClause["$regex"])
}

func TestBuildDevicesSearchFilter_PPPoE(t *testing.T) {
	filter := buildDevicesSearchFilter("", "", "cust-001")
	// GenieACS matches parameter paths exactly (no wildcard), and the
	// PPPoE Username instance varies per CPE (WANConnectionDevice up to
	// 3, WANPPPConnection up to 8), so the filter ORs over the grid.
	orClauses, ok := filter["$or"].([]map[string]interface{})
	require.True(t, ok, "$or clause must be present")
	require.Len(t, orClauses, wanConnectionDeviceInstances*wanPPPConnectionInstances)
	assert.Contains(t, orClauses[0],
		"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username._value")
	last := orClauses[len(orClauses)-1]
	assert.Contains(t, last, fmt.Sprintf(
		"InternetGatewayDevice.WANDevice.1.WANConnectionDevice.%d.WANPPPConnection.%d.Username._value",
		wanConnectionDeviceInstances, wanPPPConnectionInstances))
	for _, c := range orClauses {
		for _, v := range c {
			assert.Equal(t, "cust-001", v) // exact match, search endpoint
		}
	}
}

func TestBuildDevicesSearchFilter_Precedence(t *testing.T) {
	// MAC wins when all 3 are provided
	filter := buildDevicesSearchFilter("AA", "BB", "CC")
	assert.Len(t, filter, 1)
	assert.Contains(t, filter, "InternetGatewayDevice.LANDevice.1.LANEthernetInterfaceConfig.1.MACAddress._value")
}

// --- deviceSummaryFromTree ---

func TestDeviceSummaryFromTree_Full(t *testing.T) {
	doc := map[string]interface{}{
		"_id":         "device-001",
		"_lastInform": "2026-04-14T11:35:21Z",
		"InternetGatewayDevice": map[string]interface{}{
			"DeviceInfo": map[string]interface{}{
				"Manufacturer": map[string]interface{}{"_value": "ZTE"},
				"ModelName":    map[string]interface{}{"_value": "F670L"},
				"SerialNumber": map[string]interface{}{"_value": "SN-12345"},
			},
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANPPPConnection": map[string]interface{}{
								"1": map[string]interface{}{
									"ExternalIPAddress": map[string]interface{}{"_value": "203.0.113.45"},
								},
							},
						},
					},
				},
			},
			"LANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"LANEthernetInterfaceConfig": map[string]interface{}{
						"1": map[string]interface{}{
							"MACAddress": map[string]interface{}{"_value": "AA:BB:CC:DD:EE:FF"},
						},
					},
				},
			},
		},
	}
	d := deviceSummaryFromTree(doc)
	assert.Equal(t, "device-001", d.DeviceID)
	assert.Equal(t, "2026-04-14T11:35:21Z", d.LastInform)
	assert.Equal(t, "ZTE", d.Manufacturer)
	assert.Equal(t, "F670L", d.Model)
	assert.Equal(t, "SN-12345", d.Serial)
	assert.Equal(t, "203.0.113.45", d.IP)
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", d.MAC)
}

// TestWanIP_PrefersPPPOverIP covers the production case where a single
// WANConnectionDevice exposes both a WANPPPConnection (customer internet
// WAN) and a WANIPConnection (TR-069 management WAN). The PPP one must
// win even though "WANIPConnection" sorts first alphabetically.
func TestWanIP_PrefersPPPOverIP(t *testing.T) {
	doc := map[string]interface{}{
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANIPConnection": map[string]interface{}{
								"1": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": "10.16.2.48"}},
							},
							"WANPPPConnection": map[string]interface{}{
								"1": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": "10.100.149.64"}},
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, "10.100.149.64", wanIP(doc))
}

// TestWanIP_WhitespacePlaceholder covers CPEs that report a
// whitespace-only ExternalIPAddress (seen on ZTE F670) instead of the
// 0.0.0.0 placeholder. A blank value must not be treated as a real IP.
func TestWanIP_WhitespacePlaceholder(t *testing.T) {
	doc := map[string]interface{}{
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANPPPConnection": map[string]interface{}{
								"2": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": " "}},
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, "", wanIP(doc))
}

// TestWanIP_FallsBackToIPConnection covers devices with no PPPoE (e.g.
// DHCP/static WAN) where the only address is on a WANIPConnection.
func TestWanIP_FallsBackToIPConnection(t *testing.T) {
	doc := map[string]interface{}{
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANIPConnection": map[string]interface{}{
								"1": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": "203.0.113.9"}},
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, "203.0.113.9", wanIP(doc))
}

// TestDeviceSummaryFromTree_ConnectionRequestIPFallback covers CPEs
// whose WAN ExternalIPAddress is blank but that still report a
// management address via ManagementServer.ConnectionRequestURL.
func TestDeviceSummaryFromTree_ConnectionRequestIPFallback(t *testing.T) {
	doc := map[string]interface{}{
		"_id": "device-cru",
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANPPPConnection": map[string]interface{}{
								"2": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": " "}},
							},
						},
					},
				},
			},
			"ManagementServer": map[string]interface{}{
				"ConnectionRequestURL": map[string]interface{}{
					"_value": "http://10.100.185.223:58000/8b519e96d6658b23",
				},
			},
		},
	}
	assert.Equal(t, "10.100.185.223", deviceSummaryFromTree(doc).IP)
}

// TestDeviceSummaryFromTree_ConnectionRequestIPNotUsedWhenWANPresent
// ensures the WAN ExternalIPAddress still wins when it exists.
func TestDeviceSummaryFromTree_ConnectionRequestIPNotUsedWhenWANPresent(t *testing.T) {
	doc := map[string]interface{}{
		"_id": "device-both",
		"InternetGatewayDevice": map[string]interface{}{
			"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"WANConnectionDevice": map[string]interface{}{
						"1": map[string]interface{}{
							"WANPPPConnection": map[string]interface{}{
								"1": map[string]interface{}{"ExternalIPAddress": map[string]interface{}{"_value": "203.0.113.45"}},
							},
						},
					},
				},
			},
			"ManagementServer": map[string]interface{}{
				"ConnectionRequestURL": map[string]interface{}{"_value": "http://10.16.2.48:58000/x"},
			},
		},
	}
	assert.Equal(t, "203.0.113.45", deviceSummaryFromTree(doc).IP)
}

func TestDeviceSummaryFromTree_Empty(t *testing.T) {
	d := deviceSummaryFromTree(map[string]interface{}{})
	assert.Empty(t, d.DeviceID)
}

func TestDeviceSummaryFromTree_IDFallback(t *testing.T) {
	// When DeviceInfo params are not yet populated, model and serial
	// should be extracted from the _id (OUI-ProductClass-Serial).
	d := deviceSummaryFromTree(map[string]interface{}{
		"_id":         "347839-F670L-ZTEKQB8PH641353",
		"_lastInform": "2026-07-20T11:53:08Z",
	})
	assert.Equal(t, "347839-F670L-ZTEKQB8PH641353", d.DeviceID)
	assert.Equal(t, "F670L", d.Model)
	assert.Equal(t, "ZTEKQB8PH641353", d.Serial)
	assert.Equal(t, "2026-07-20T11:53:08Z", d.LastInform)
}

func TestDeviceSummaryFromTree_IDFallback_ParamsOverride(t *testing.T) {
	// When DeviceInfo params ARE populated, they take precedence over _id.
	d := deviceSummaryFromTree(map[string]interface{}{
		"_id": "347839-F670L-ZTEKQB8PH641353",
		"InternetGatewayDevice": map[string]interface{}{
			"DeviceInfo": map[string]interface{}{
				"ModelName":    map[string]interface{}{"_value": "ZXHN F670L"},
				"SerialNumber": map[string]interface{}{"_value": "ZTEKQB8PH641353"},
			},
		},
	})
	assert.Equal(t, "ZXHN F670L", d.Model) // full name from params, not _id
	assert.Equal(t, "ZTEKQB8PH641353", d.Serial)
}

func TestDeviceSummaryFromTree_MalformedID(t *testing.T) {
	// _id not a string → should be skipped without panicking
	d := deviceSummaryFromTree(map[string]interface{}{
		"_id":         float64(123),
		"_lastInform": float64(456),
	})
	assert.Empty(t, d.DeviceID)
	assert.Empty(t, d.LastInform)
}

// --- dominantVendorNamespace ---

func TestDominantVendorNamespace(t *testing.T) {
	cases := map[string]struct {
		doc  map[string]interface{}
		want string
	}{
		"none → TR-098": {
			doc:  map[string]interface{}{"InternetGatewayDevice": map[string]interface{}{"DeviceInfo": map[string]interface{}{}}},
			want: "TR-098",
		},
		"huawei": {
			doc: map[string]interface{}{"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"X_HW_VLAN":        map[string]interface{}{"_value": 90},
					"X_HW_SERVICELIST": map[string]interface{}{"_value": "INTERNET"},
					"X_GponInterafceConfig": map[string]interface{}{
						"RxPower": map[string]interface{}{"_value": "-20"},
					},
				},
			}},
			want: "X_HW",
		},
		"zte wins by count": {
			doc: map[string]interface{}{"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"X_ZTE-COM_VLANID":     map[string]interface{}{"_value": 90},
					"X_ZTE-COM_VLANEnable": map[string]interface{}{"_value": true},
					"X_HW_VLAN":            map[string]interface{}{"_value": 90},
				},
			}},
			want: "X_ZTE-COM",
		},
		"tie breaks lexically": {
			doc: map[string]interface{}{"WANDevice": map[string]interface{}{
				"1": map[string]interface{}{
					"X_HW_VLAN":      map[string]interface{}{"_value": 1},
					"X_ZTE-COM_VLAN": map[string]interface{}{"_value": 2},
				},
			}},
			want: "X_HW",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, dominantVendorNamespace(tc.doc))
		})
	}
}

func TestDeviceSummaryFromTree_ParamSet(t *testing.T) {
	d := deviceSummaryFromTree(map[string]interface{}{
		"_id": "device-001",
		"WANDevice": map[string]interface{}{
			"1": map[string]interface{}{
				"X_HW_VLAN": map[string]interface{}{"_value": 90},
			},
		},
	})
	assert.Equal(t, "X_HW", d.ParamSet)
}

// --- listDevicesHandler ---

// devicesMockHandler handles the GenieACS devices NBI query for
// list/search tests. Returns the supplied devices array.
func devicesMockHandler(devices []map[string]interface{}, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/devices/") && r.URL.Path != "/devices/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		if status >= 400 {
			return
		}
		body, _ := json.Marshal(devices)
		_, _ = w.Write(body)
	}
}

func TestListDevicesHandler_Success(t *testing.T) {
	devices := []map[string]interface{}{
		{
			"_id":         "device-001",
			"_lastInform": "2026-04-14T11:00:00Z",
		},
		{
			"_id":         "device-002",
			"_lastInform": "2026-04-14T11:01:00Z",
		},
	}
	_, router := setupTestServer(t, devicesMockHandler(devices, http.StatusOK))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/devices?page=1&page_size=10", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "device-001")
	assert.Contains(t, rr.Body.String(), "device-002")
}

func TestListDevicesHandler_PPPoEAndRxPower(t *testing.T) {
	devices := []map[string]interface{}{
		{
			"_id":         "device-001",
			"_lastInform": "2026-04-14T11:00:00Z",
			"InternetGatewayDevice": map[string]interface{}{
				"WANDevice": map[string]interface{}{
					"1": map[string]interface{}{
						"X_ZTE-COM_WANPONInterfaceConfig": map[string]interface{}{
							// ZTE ships optics as xsd:string.
							"RXPower": map[string]interface{}{"_value": "-25.08"},
						},
						"WANConnectionDevice": map[string]interface{}{
							"2": map[string]interface{}{
								"WANPPPConnection": map[string]interface{}{
									"3": map[string]interface{}{
										"ExternalIPAddress": map[string]interface{}{"_value": "10.100.149.67"},
										"Username":          map[string]interface{}{"_value": "cust-001@isp"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, router := setupTestServer(t, devicesMockHandler(devices, http.StatusOK))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/devices", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// Non-1.1.1 instances must still resolve (regression: instance pinning).
	assert.Contains(t, rr.Body.String(), "cust-001@isp")
	assert.Contains(t, rr.Body.String(), "10.100.149.67")
	assert.Contains(t, rr.Body.String(), "-25.08")
}

func TestListDevicesHandler_BadPage(t *testing.T) {
	_, router := setupTestServer(t, devicesMockHandler(nil, http.StatusOK))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/devices?page=0", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestListDevicesHandler_NBIError(t *testing.T) {
	_, router := setupTestServer(t, devicesMockHandler(nil, http.StatusInternalServerError))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/devices", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- searchDevicesHandler ---

func TestSearchDevicesHandler_ByMAC(t *testing.T) {
	devices := []map[string]interface{}{
		{"_id": "device-001"},
	}
	_, router := setupTestServer(t, devicesMockHandler(devices, http.StatusOK))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/devices/search?mac=AA:BB:CC:DD:EE:FF", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "device-001")
}

func TestSearchDevicesHandler_ByPPPoE(t *testing.T) {
	devices := []map[string]interface{}{
		{"_id": "device-002"},
	}
	_, router := setupTestServer(t, devicesMockHandler(devices, http.StatusOK))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/devices/search?pppoe_username=cust-001", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestSearchDevicesHandler_NoKey(t *testing.T) {
	_, router := setupTestServer(t, devicesMockHandler(nil, http.StatusOK))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/devices/search", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "mac, serial")
}

func TestSearchDevicesHandler_NotFound(t *testing.T) {
	_, router := setupTestServer(t, devicesMockHandler(nil, http.StatusOK))
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/devices/search?mac=AA:BB:CC:DD:EE:FF", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestSearchDevicesHandler_NBIError(t *testing.T) {
	_, router := setupTestServer(t, devicesMockHandler(nil, http.StatusInternalServerError))
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/genieacs/devices/search?mac=AA:BB:CC:DD:EE:FF", nil)
	req.Header.Set("X-API-Key", mockAPIKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- queryDevicesNBI direct error paths (transport / decode failures) ---

func TestQueryDevicesNBI_TransportFailure(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{Transport: &failingTransport{}}
	t.Cleanup(func() { httpClient = originalClient })

	_, err := queryDevicesNBI(context.Background(), nil, 10, 0)
	assert.Error(t, err)
}

func TestQueryDevicesNBI_BadJSON(t *testing.T) {
	mock := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not-json`))
	})
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)
	originalBase := geniesBaseURL
	originalClient := httpClient
	geniesBaseURL = srv.URL
	httpClient = srv.Client()
	t.Cleanup(func() {
		geniesBaseURL = originalBase
		httpClient = originalClient
	})

	_, err := queryDevicesNBI(context.Background(), nil, 10, 0)
	assert.Error(t, err)
}

func TestQueryDevicesNBI_BodyReadError(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{Transport: &errorBodyTransport{statusCode: http.StatusOK}}
	t.Cleanup(func() { httpClient = originalClient })

	_, err := queryDevicesNBI(context.Background(), nil, 10, 0)
	assert.Error(t, err)
}

func TestQueryDevicesNBI_NewRequestFailure(t *testing.T) {
	// geniesBaseURL with a control character makes NewRequestWithContext fail
	// before the call ever leaves the helper.
	originalBase := geniesBaseURL
	geniesBaseURL = "http://example.com/\x7f"
	t.Cleanup(func() { geniesBaseURL = originalBase })

	_, err := queryDevicesNBI(context.Background(), nil, 10, 0)
	assert.Error(t, err)
}
