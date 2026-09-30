package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OpticalStats describes the optical interface health of a CPE/ONT,
// extracted from whichever vendor-specific TR-069 parameter tree the
// device exposes. Fields use SI/standard units to make ops dashboards
// deterministic across vendors.
type OpticalStats struct {
	DeviceID string `json:"device_id"`

	// TxPowerDBm — optical transmit power in dBm. Normal PON ONU range
	// is roughly -1 to +5 dBm. Lower values may indicate laser bias
	// degradation or power supply issues.
	TxPowerDBm float64 `json:"tx_power_dbm"`

	// RxPowerDBm — optical receive power in dBm. Normal PON ONU range
	// is roughly -8 to -27 dBm depending on splitter ratio + distance.
	// Below -27: marginal. Below -30: no signal (fiber broken/disconnected).
	// Above -8: overloaded (CPE may be too close to OLT or splitter ratio wrong).
	RxPowerDBm float64 `json:"rx_power_dbm"`

	// BiasCurrentMA — laser driver bias current in milliamps. Normal
	// range is roughly 5-30 mA depending on optics module. Rising bias
	// over time indicates laser aging.
	BiasCurrentMA float64 `json:"bias_current_ma,omitempty"`

	// TemperatureC — internal optics module temperature in Celsius.
	// Normal range is 0-80°C. Above 85°C may trigger thermal shutdown.
	TemperatureC float64 `json:"temperature_c,omitempty"`

	// VoltageV — optics module supply voltage in Volts. Normal range
	// is 3.0-3.6V (3.3V nominal). Out-of-range = power supply issue.
	VoltageV float64 `json:"voltage_v,omitempty"`

	// Health — categorical classification derived from RxPowerDBm using
	// configurable thresholds. Values: "good", "warning", "critical",
	// "no_signal", "unknown".
	Health string `json:"health"`

	// Source — which parameter tree the values came from. Useful for
	// debugging and vendor-mix analytics. Possible values:
	// "zte_ct_com_epon", "zte_ct_com_gpon", "huawei_hw_debug",
	// "realtek_epon", "standard_tr181".
	Source string `json:"source"`

	// FetchedAt — RFC3339 timestamp of when the data was read from
	// GenieACS. Combine with `?refresh=true` for guaranteed freshness.
	FetchedAt string `json:"fetched_at"`
}

// errOpticalNotSupported is the sentinel error returned when no known
// vendor parameter tree is found in the device data. Caller (handler)
// translates this to HTTP 404 with error_code OPTICAL_NOT_SUPPORTED.
var errOpticalNotSupported = errors.New("optical interface stats not supported by this device")

// opticalSource is the categorical label for which vendor parameter
// tree the values came from. Stable strings — used in the response
// payload `source` field for analytics.
const (
	opticalSourceZTECTComEpon  = "zte_ct_com_epon"
	opticalSourceZTECTComGpon  = "zte_ct_com_gpon"
	opticalSourceZTEWanPon     = "zte_wan_pon_interface"
	opticalSourceHuaweiGpon    = "huawei_gpon_interface"
	opticalSourceHuaweiHWDbg   = "huawei_hw_debug"
	opticalSourceRealtekEpon   = "realtek_epon"
	opticalSourceStandardTR181 = "standard_tr181"
	opticalSourceVendorGeneric = "vendor_generic"
)

// opticalSupplyVoltageMilliThreshold — raw SupplyVoltage values above
// this count are treated as millivolts and divided by 1000 to yield
// volts. ZTE F670L reports "3244" (mV) where most vendors report "3.244"
// (V). 100 is comfortably above any plausible sane voltage figure and
// well below the millivolt range for a 3.3V-nominal optical module.
const opticalSupplyVoltageMilliThreshold = 100.0

// opticalSubtreePathsToRefresh lists the parameter subtrees we ask
// GenieACS to refresh when the caller passes ?refresh=true. We refresh
// ALL known vendor subtrees because we don't know which the CPE
// supports until we parse the response — and refresh requests against
// unsupported subtrees return harmlessly. Order is best-effort: most
// common (ZTE EPON in Indonesian deployments) first.
var opticalSubtreePathsToRefresh = []string{
	"InternetGatewayDevice.X_CT-COM_EponInterfaceConfig",
	"InternetGatewayDevice.X_CT-COM_GponInterfaceConfig",
	"InternetGatewayDevice.WANDevice.1.X_ZTE-COM_WANPONInterfaceConfig",
	// Huawei GPON ONTs (HG8245H5, HG8145V5, …) expose optics on a
	// WANDevice child whose name carries Huawei's `Interafce` typo.
	// refreshObject only touches the exact instance, but instance 1 is
	// where the production fleet reports it.
	"InternetGatewayDevice.WANDevice.1.X_GponInterafceConfig",
	"InternetGatewayDevice.X_HW_DEBUG.AdminTR069",
	"InternetGatewayDevice.X_Realtek_EponInterfaceConfig",
	"Device.Optical.Interface",
}

// refreshOpticalStats triggers a refreshObject task on each known
// optical parameter subtree, with `?connection_request` so each call
// blocks until the task is applied (or queued). Errors on individual
// subtrees are logged and swallowed — many CPEs only expose one of
// these trees, and a 404 from GenieACS for the wrong vendor tree is
// expected and harmless.
//
// Success contract: as long as at least one subtree returned a 2xx/3xx
// status, the overall call succeeds. Only when ALL subtrees fail (every
// one returned 4xx/5xx or had a transport error) does the function
// return an error.
func refreshOpticalStats(ctx context.Context, deviceID string) error {
	var lastErr error
	successCount := 0
	for _, subtree := range opticalSubtreePathsToRefresh {
		statusCode, err := refreshOneOpticalSubtree(ctx, deviceID, subtree)
		if err != nil {
			lastErr = err
			continue
		}
		if statusCode < http.StatusBadRequest {
			successCount++
			continue
		}
		// 4xx/5xx — record the latest bad status as the surfaceable
		// error in case every subtree fails.
		lastErr = fmt.Errorf("subtree %q: status %d", subtree, statusCode)
	}
	if successCount == 0 {
		if lastErr == nil {
			lastErr = errors.New("no subtrees attempted")
		}
		return fmt.Errorf("optical refresh failed for all known subtrees: %w", lastErr)
	}
	return nil
}

// refreshOneOpticalSubtree posts a single refreshObject task for the
// given subtree and returns the response status code (along with any
// transport error). Extracted from refreshOpticalStats so the linter
// can see the deferred body close — the inline loop variant tripped
// bodyclose due to the early `continue` paths.
func refreshOneOpticalSubtree(ctx context.Context, deviceID, subtree string) (int, error) {
	// GenieACS 1.2.16 rejects refreshObject tasks whose objectName ends
	// with a dot — the task lands with "Invalid parameter path" and
	// sticks in the queue forever, re-executed on every inform, never
	// drained. Observed in real lab against a ZTE F670L when probing
	// for vendor-specific optical paths. Normalize defensively here so
	// a typo in opticalSubtreePathsToRefresh can't poison the task queue.
	subtree = strings.TrimRight(subtree, ".")
	urlQ := fmt.Sprintf("%s/devices/%s/tasks?connection_request", geniesBaseURL, url.PathEscape(deviceID))
	payload := fmt.Sprintf(`{"name": "refreshObject", "objectName": %q}`, subtree)
	resp, err := postJSONRequest(ctx, urlQ, payload)
	if err != nil {
		return 0, err
	}
	defer safeClose(resp.Body)
	return resp.StatusCode, nil
}

// getOpticalStats reads optical interface stats from GenieACS for the
// given device, automatically detecting which vendor parameter tree
// the CPE exposes. Returns errOpticalNotSupported (sentinel) if no
// known tree is found.
//
// Detection order: ZTE CT-COM EPON → ZTE CT-COM GPON → ZTE WAN PON →
// Huawei GPON → Huawei HW_DEBUG → Realtek EPON → standard TR-181 →
// generic vendor scan. The order matches typical Indonesian ISP
// deployment frequency (most ZTE F670L/F660 ONTs in residential PON
// deployments); the generic scan is last so known trees keep their
// Source label.
func getOpticalStats(ctx context.Context, deviceID string) (*OpticalStats, error) {
	deviceData, err := getDeviceData(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	stats := &OpticalStats{
		DeviceID:  deviceID,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Try each vendor extractor in order. First one that returns true
	// wins — sets the source and returns the populated struct.
	if extractZTECTComEpon(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractZTECTComGpon(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractZTEWanPon(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractHuaweiGpon(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractHuaweiHWDebug(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractRealtekEpon(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractStandardTR181(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}
	if extractVendorOptical(deviceData, stats) {
		classifyOpticalHealth(stats)
		return stats, nil
	}

	return nil, errOpticalNotSupported
}

// extractZTECTComEpon reads optical stats from the ZTE CT-COM EPON
// parameter tree (`InternetGatewayDevice.X_CT-COM_EponInterfaceConfig.Stats`).
// This is the most common tree in Indonesian residential PON deployments
// (ZTE F670L, F660, F670Lv9 — all China Telecom OEM-branded).
func extractZTECTComEpon(deviceData map[string]interface{}, stats *OpticalStats) bool {
	statsTree := navigateNested(deviceData,
		"InternetGatewayDevice", "X_CT-COM_EponInterfaceConfig", "Stats")
	if statsTree == nil {
		return false
	}
	stats.Source = opticalSourceZTECTComEpon
	stats.TxPowerDBm = readFloat(statsTree, "TxPower")
	stats.RxPowerDBm = readFloat(statsTree, "RxPower")
	stats.BiasCurrentMA = readFloat(statsTree, "BiasCurrent")
	stats.TemperatureC = readFloat(statsTree, "Temperature")
	stats.VoltageV = readFloat(statsTree, "Voltage")
	return true
}

// extractVendorOptical is the catch-all for vendor optical trees without a
// dedicated extractor. GenieACS exposes every parameter as a flattened path
// (device doc → IGD → WANDevice.N → <tree> → … → RxPower), and GenieACS can
// auto-populate the RxPower/TxPower family by parameter name. So we scan the
// tree for any leaf literally named RXPower/RxPower (and TxPower/TXPower, …)
// instead of enumerating tree names — otherwise every new vendor spelling is
// an OPTICAL_NOT_SUPPORTED 404 until someone adds another pinned extractor.
//
// Placed last in the detection chain so it only runs for trees the dedicated
// extractors don't recognize. Source is labelled vendor_generic since the
// exact tree name isn't tracked here.
func extractVendorOptical(deviceData map[string]interface{}, stats *OpticalStats) bool {
	leaf := func(names ...string) (float64, bool) {
		return FirstLeafFloat(deviceData, names...)
	}
	tx, _ := leaf("TXPower", "TxPower")
	rx, ok := leaf("RXPower", "RxPower")
	if !ok || rx == 0 {
		return false
	}
	stats.Source = opticalSourceVendorGeneric
	stats.TxPowerDBm = tx
	stats.RxPowerDBm = rawRxToDBm(rx)
	if v, ok := leaf("BiasCurrent"); ok {
		stats.BiasCurrentMA = v
	}
	if v, ok := leaf("TransceiverTemperature", "Temperature"); ok {
		stats.TemperatureC = v
	}
	if v, ok := leaf("SupplyVoltage"); ok {
		stats.VoltageV = normalizeSupplyVoltage(v)
	}
	return true
}

// extractZTECTComGpon reads optical stats from the GPON variant of the
// ZTE CT-COM tree. Same shape as EPON but used by GPON ONTs.
func extractZTECTComGpon(deviceData map[string]interface{}, stats *OpticalStats) bool {
	statsTree := navigateNested(deviceData,
		"InternetGatewayDevice", "X_CT-COM_GponInterfaceConfig", "Stats")
	if statsTree == nil {
		return false
	}
	stats.Source = opticalSourceZTECTComGpon
	stats.TxPowerDBm = readFloat(statsTree, "TxPower")
	stats.RxPowerDBm = readFloat(statsTree, "RxPower")
	stats.BiasCurrentMA = readFloat(statsTree, "BiasCurrent")
	stats.TemperatureC = readFloat(statsTree, "Temperature")
	stats.VoltageV = readFloat(statsTree, "Voltage")
	return true
}

// extractZTEWanPon reads optical stats from the ZTE-specific WAN PON
// interface subtree
// (`InternetGatewayDevice.WANDevice.1.X_ZTE-COM_WANPONInterfaceConfig`).
//
// This tree is used by ZTE F670L and newer OEM-agnostic ZTE ONTs that
// don't expose the China-Telecom-branded X_CT-COM_* trees. All values
// are reported as xsd:string — we parse them via the string branch in
// readFloat.
//
// Field layout (observed on a real F670L running V9.0.10P1N12A):
//
//	BiasCurrent            "13.70"  (mA)
//	RXPower                "-25.08" (dBm)
//	TXPower                "2.59"   (dBm)
//	TransceiverTemperature "35.31"  (°C)
//	SupplyVoltage          "3244"   (mV — divided by 1000 for volts)
//
// Note the uppercase `TXPower` / `RXPower` (ZTE convention) vs the
// camelCase `TxPower` / `RxPower` used by X_CT-COM. We try both so the
// same extractor works for mixed firmware lines in the wild.
//
// Some firmware (verified on ZTE F609) instead reports raw SFF-8472 DOM
// registers — RX 41, TX 17864, bias 5200, temp 11461, Vcc 32600 — which
// are unreadable without scaling. isRawDOMTree detects that encoding
// and domScaled applies the per-field register conversion. RX always
// goes through rawRxToDBm, which is sign-safe for both encodings.
func extractZTEWanPon(deviceData map[string]interface{}, stats *OpticalStats) bool {
	root := navigateNested(deviceData,
		"InternetGatewayDevice", "WANDevice", "1", "X_ZTE-COM_WANPONInterfaceConfig")
	if root == nil {
		return false
	}
	raw := isRawDOMTree(root)

	stats.Source = opticalSourceZTEWanPon
	stats.RxPowerDBm = rxDBm(root, "RXPower", "RxPower")
	stats.TxPowerDBm = domScaled(firstNonZeroFloat(root, "TXPower", "TxPower"), "tx", raw)
	stats.BiasCurrentMA = domScaled(readFloat(root, "BiasCurrent"), "bias", raw)
	stats.TemperatureC = domScaled(firstNonZeroFloat(root, "TransceiverTemperature", "Temperature"), "temp", raw)
	if raw {
		stats.VoltageV = domScaled(readFloat(root, "SupplyVoltage"), "vcc", true)
	} else {
		stats.VoltageV = normalizeSupplyVoltage(readFloat(root, "SupplyVoltage"))
	}
	return true
}

// extractHuaweiGpon reads optical stats from the Huawei GPON ONT tree
// (`InternetGatewayDevice.WANDevice.{n}.X_GponInterafceConfig` — note
// Huawei's firmware-side `Interafce` spelling). Used by HG8245H5,
// HG8145V5, and other HG-series GPON ONTs that expose optics as a
// WANDevice child instead of the X_HW_DEBUG tree.
//
// Scans every WANDevice instance because the optics instance varies
// (instance 2 on some firmware lines). RXPower is stored as a bare
// number (xsd:int) and, like the ZTE WAN PON tree, may be either dBm
// (negative) or linear µW (positive) — rawRxToDBm disambiguates by sign.
func extractHuaweiGpon(deviceData map[string]interface{}, stats *OpticalStats) bool {
	devices := navigateNested(deviceData, "InternetGatewayDevice", "WANDevice")
	if devices == nil {
		return false
	}
	var tree map[string]interface{}
	for _, n := range EnumerateInstances(deviceData, "InternetGatewayDevice.WANDevice") {
		inst, ok := devices[strconv.Itoa(n)].(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := inst["X_GponInterafceConfig"].(map[string]interface{}); ok {
			tree = t
			break
		}
	}
	if tree == nil {
		return false
	}
	raw := isRawDOMTree(tree)

	stats.Source = opticalSourceHuaweiGpon
	stats.RxPowerDBm = rxDBm(tree, "RXPower", "RxPower")
	stats.TxPowerDBm = domScaled(firstNonZeroFloat(tree, "TXPower", "TxPower"), "tx", raw)
	stats.BiasCurrentMA = domScaled(firstNonZeroFloat(tree, "BiasCurrent"), "bias", raw)
	stats.TemperatureC = domScaled(firstNonZeroFloat(tree, "TransceiverTemperature", "Temperature"), "temp", raw)
	if raw {
		stats.VoltageV = domScaled(firstNonZeroFloat(tree, "SupplyVoltage"), "vcc", true)
	} else {
		stats.VoltageV = normalizeSupplyVoltage(firstNonZeroFloat(tree, "SupplyVoltage"))
	}
	return true
}

// firstNonZeroFloat tries each key in order and returns the first
// non-zero float value found. Used for vendor trees that report the
// same semantic field under slightly different names across firmware
// revisions.
func firstNonZeroFloat(parent map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if v := readFloat(parent, key); v != 0 {
			return v
		}
	}
	return 0
}

// normalizeSupplyVoltage converts raw voltage readings to volts.
// ZTE F670L reports SupplyVoltage as millivolts (e.g. "3244"); most
// other vendors use volts (e.g. "3.244"). We pick the unit by magnitude:
// anything above 100 is assumed mV and divided by 1000.
func normalizeSupplyVoltage(raw float64) float64 {
	if raw > opticalSupplyVoltageMilliThreshold {
		return raw / 1000.0
	}
	return raw
}

// extractHuaweiHWDebug reads optical stats from the Huawei HW_DEBUG
// parameter tree (`InternetGatewayDevice.X_HW_DEBUG.AdminTR069`).
// Used by HG8245, HG8546, HN8145, and other Huawei HG-series ONTs.
func extractHuaweiHWDebug(deviceData map[string]interface{}, stats *OpticalStats) bool {
	statsTree := navigateNested(deviceData,
		"InternetGatewayDevice", "X_HW_DEBUG", "AdminTR069")
	if statsTree == nil {
		return false
	}
	stats.Source = opticalSourceHuaweiHWDbg
	stats.TxPowerDBm = readFloat(statsTree, "TxPower")
	stats.RxPowerDBm = readFloat(statsTree, "RxPower")
	// Huawei usually doesn't expose bias/temp/voltage in HW_DEBUG.
	// Leave them zero (omitempty in JSON).
	return true
}

// extractRealtekEpon reads optical stats from the Realtek/PMC EPON
// chipset parameter tree. Used by various noname OEM ONTs that bundle
// the Realtek RTL96xx series.
func extractRealtekEpon(deviceData map[string]interface{}, stats *OpticalStats) bool {
	statsTree := navigateNested(deviceData,
		"InternetGatewayDevice", "X_Realtek_EponInterfaceConfig", "Stats")
	if statsTree == nil {
		return false
	}
	stats.Source = opticalSourceRealtekEpon
	stats.TxPowerDBm = readFloat(statsTree, "TxPower")
	stats.RxPowerDBm = readFloat(statsTree, "RxPower")
	stats.BiasCurrentMA = readFloat(statsTree, "BiasCurrent")
	stats.TemperatureC = readFloat(statsTree, "Temperature")
	stats.VoltageV = readFloat(statsTree, "Voltage")
	return true
}

// extractStandardTR181 reads optical stats from the standard TR-181
// `Device.Optical.Interface.1.Stats.*` tree. Rarely supported in
// consumer-grade CPE but present in some enterprise/business gateways.
func extractStandardTR181(deviceData map[string]interface{}, stats *OpticalStats) bool {
	statsTree := navigateNested(deviceData,
		"Device", "Optical", "Interface", "1", "Stats")
	if statsTree == nil {
		return false
	}
	stats.Source = opticalSourceStandardTR181
	stats.TxPowerDBm = readFloat(statsTree, "TxPower")
	stats.RxPowerDBm = readFloat(statsTree, "RxPower")
	stats.BiasCurrentMA = readFloat(statsTree, "BiasCurrent")
	stats.TemperatureC = readFloat(statsTree, "Temperature")
	stats.VoltageV = readFloat(statsTree, "Voltage")
	return true
}

// extractRxPower reads optical receive power (dBm) from whichever
// vendor parameter tree the device exposes. Scans every WANDevice
// instance (optics often live on instance 2, not 1) and every known
// vendor tree, mirroring the detection order in getOpticalStats.
//
// Returns 0 when nothing is found — 0 is the "not reported" sentinel
// (json omitempty), same convention as OpticalStats.
//
// rawRxToDBm applies the two on-wire encodings: most ONTs report dBm
// directly (negative), while Huawei X_GponInterafceConfig and ZTE
// X_CMCC_GponInterfaceConfig report linear µW (positive, e.g. "-16.25
// dBm" arrives as "23.7"). Without the conversion those devices showed
// a bogus positive number and looked "missing" in the UI.
func extractRxPower(doc map[string]interface{}) float64 {
	if wan, ok := doc["InternetGatewayDevice"].(map[string]interface{}); ok {
		if devices, ok := wan["WANDevice"].(map[string]interface{}); ok {
			// Vendor trees name the leaf RXPower (ZTE) or RxPower; scan
			// the whole WAN subtree so instance placement doesn't matter.
			if raw, ok := FirstLeafFloat(devices, "RXPower", "RxPower"); ok && raw != 0 {
				return rawRxToDBm(raw)
			}
		}
	}
	for _, path := range rxPowerAbsolutePaths {
		if v, ok := LookupFloat(doc, path); ok && v != 0 {
			return rawRxToDBm(v)
		}
	}
	return 0
}

// rxPowerAbsolutePaths covers the non-WAN optical trees (Realtek EPON,
// Huawei HW_DEBUG, standard TR-181) that some CPEs expose outside the
// WANDevice hierarchy.
var rxPowerAbsolutePaths = []string{
	"InternetGatewayDevice.X_HW_DEBUG.AdminTR069.RxPower",
	"InternetGatewayDevice.X_Realtek_EponInterfaceConfig.Stats.RxPower",
	"Device.Optical.Interface.1.Stats.RxPower",
}

// rawRxToDBm normalizes a raw optical RX power reading to dBm. Negative
// values are already in dBm and pass through; positive values are
// linear power in units of 0.01 µW (matching the GenieACS virtual
// parameter convention), converted with 10*log10(raw/10000).
func rawRxToDBm(raw float64) float64 {
	if raw < 0 {
		return math.Round(raw*100) / 100
	}
	return math.Round(10*math.Log10(raw/10000)*10) / 10
}

// navigateNested walks a chain of map[string]interface{} keys and
// returns the leaf map, or nil if any step is missing/wrong-typed.
// Convenience for the deeply nested GenieACS device tree.
func navigateNested(root map[string]interface{}, path ...string) map[string]interface{} {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// readFloat extracts a numeric `_value` from a GenieACS parameter
// node. GenieACS stores parameter values as objects shaped like
// `{"_value": 3.14, "_type": "xsd:float", "_timestamp": "..."}`.
//
// The value is typically a numeric (xsd:float / xsd:int), but some
// vendor extensions — notably ZTE's `X_ZTE-COM_WANPONInterfaceConfig`
// subtree — ship everything as xsd:string (e.g. `"_value": "2.59"`).
// We accept both by attempting strconv.ParseFloat on string payloads.
//
// Returns 0 if the key is missing, the node isn't a map, or the value
// can't be coerced to a float. Callers treat 0 as "not reported".
func readFloat(parent map[string]interface{}, key string) float64 {
	node, ok := parent[key].(map[string]interface{})
	if !ok {
		return 0
	}
	switch v := node["_value"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

// rawValueType was replaced by magnitude-based detection; removed.

// isRawDOMTree detects whether a transceiver tree reports raw SFF-8472
// register values rather than pre-scaled physical ones. It keys on TX
// power and temperature magnitude because no physical reading is ever
// that large (Tx dBm ≈ -3..+5, temp ≈ 0..80 °C, Vcc ≈ 3.3 V), whereas
// the raw registers are in the thousands. Some firmware reports these
// as JSON integers, others as decimal strings, so magnitude is a more
// robust discriminator than the JSON type.
//
// Reads direct children via firstNonZeroFloat (not FirstLeafFloat,
// which only matches leaves nested ≥2 levels deep).
func isRawDOMTree(tree map[string]interface{}) bool {
	if tx := firstNonZeroFloat(tree, "TXPower", "TxPower"); tx > 100 || tx < -100 {
		return true
	}
	if temp := firstNonZeroFloat(tree, "TransceiverTemperature", "Temperature"); temp > 100 || temp < -100 {
		return true
	}
	return false
}

// rxDBm reads an RX-power leaf and normalizes it to dBm. RX is always
// run through rawRxToDBm: a negative value is already dBm (passes
// through), while a positive value is linear µW (raw register or
// vendor decimal), which is never a valid dBm reading. 0 (absent leaf)
// stays 0 — the "not reported" sentinel.
func rxDBm(parent map[string]interface{}, keys ...string) float64 {
	if v := firstNonZeroFloat(parent, keys...); v != 0 {
		return rawRxToDBm(v)
	}
	return 0
}

// domScaled converts a raw optical-DOM register reading to its physical
// unit, for CPEs that report the transceiver's raw SFF-8472 values
// instead of pre-scaled ones.
//
// The on-wire encodings differ per field, matching the SFF-8472
// convention GenieACS virtual parameters use for these ZTE F609/F670
// units (verified against the fleet's VirtualParameters.OpticalRXdBm):
//
//	tx:   linear µW (0.1 µW LSB) → dBm via rawRxToDBm
//	bias: 2 µA LSB  → mA   = value/500
//	temp: 1/256 °C LSB → °C = value/256
//	vcc:  100 µV LSB → V   = value/10000
//
// raw=false (the value is already a physical reading) returns as-is,
// so the same helper works for decimal vendors. RX is handled by
// rxDBm instead (sign-based, so it needs no raw flag).
func domScaled(value float64, field string, raw bool) float64 {
	if !raw {
		return value
	}
	switch field {
	case "tx":
		return rawRxToDBm(value)
	case "bias":
		return round2(value / 500)
	case "temp":
		return round2(value / 256)
	case "vcc":
		return round4(value / 10000)
	}
	return value
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// classifyOpticalHealth derives the categorical Health field from
// RxPowerDBm using configurable thresholds. Defaults match typical PON
// ONT operating ranges; tunable per deployment via env vars
// OPTICAL_RX_NO_SIGNAL_DBM, OPTICAL_RX_CRITICAL_DBM, OPTICAL_RX_WARNING_DBM,
// OPTICAL_RX_OVERLOAD_DBM (read at startup, cached in the package vars).
func classifyOpticalHealth(stats *OpticalStats) {
	rx := stats.RxPowerDBm
	switch {
	case rx == 0:
		// 0 is the zero-value sentinel — extractor didn't find an RxPower
		// field. Don't classify; mark unknown.
		stats.Health = "unknown"
	case rx <= opticalRxNoSignalDBm:
		stats.Health = "no_signal"
	case rx <= opticalRxCriticalDBm:
		stats.Health = "critical"
	case rx <= opticalRxWarningDBm:
		stats.Health = "warning"
	case rx >= opticalRxOverloadDBm:
		// Above overload threshold = receiver too hot, unusual but
		// happens on misconfigured short-haul links.
		stats.Health = "warning"
	default:
		stats.Health = "good"
	}
}
