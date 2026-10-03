/** Shapes mirrored from the Go models (models.go, handlers_devices.go). */

export interface DeviceSummary {
	device_id: string;
	ip?: string;
	last_inform?: string;
	manufacturer?: string;
	model?: string;
	serial?: string;
	mac?: string;
	/** Vendor parameter structure, e.g. "X_HW", "X_ZTE-COM", or "TR-098". */
	param_set?: string;
	pppoe_username?: string;
	/** Optical receive power in dBm; absent when the CPE reports no optics. */
	rx_power_dbm?: number;
}

export interface DevicesListResponse {
	page: number;
	page_size: number;
	count: number;
	has_more: boolean;
	devices: DeviceSummary[];
}

export interface DeviceSearchResponse {
	device: DeviceSummary;
}

// --- Per-device inspection (mirrors handlers_inspection.go / models.go / optical.go) ---

export interface DeviceStatus {
	device_id: string;
	ip?: string;
	last_inform?: string;
	last_inform_age_seconds?: number;
	online: boolean;
	uptime_seconds?: number;
	manufacturer?: string;
	model?: string;
	software_version?: string;
	hardware_version?: string;
}

export interface WANConnection {
	wan_device: number;
	connection_device: number;
	instance: number;
	type: string;
	/** Operator-assigned connection name (TR-069 Name). Absent if the CPE hides it. */
	name?: string;
	connection_status?: string;
	external_ip?: string;
	uptime_seconds?: number;
	username?: string;
	last_connection_error?: string;
	/** Current Enable state; absent when the CPE does not expose it. */
	enabled?: boolean;
	/** NAT enabled (TR-098 NATEnabled). Absent if not exposed. */
	nat_enabled?: boolean;
	/** VLAN tagging enabled (vendor X_ param). Absent if not exposed. */
	vlan_enabled?: boolean;
	/** 802.1Q VLAN id (vendor X_ param). Absent if not exposed. */
	vlan_id?: number;
	/** Vendor service-list string as reported by the CPE (e.g. "INTERNET_TR069" or "TR069,INTERNET"). Absent if not exposed. */
	service_list?: string;
	/** True when the CPE accepts only one service tag (Huawei). Absent if no service list is exposed. */
	service_list_single?: boolean;
}

export interface WANConnectionsResponse {
	device_id: string;
	ip?: string;
	wan_connections: WANConnection[];
}

export interface WANConnectionMutationResponse {
	message: string;
	device_id: string;
	ip?: string;
	type: string;
	wan_device: number;
	connection_device: number;
	instance: number;
}

export interface OpticalStats {
	device_id: string;
	tx_power_dbm: number;
	rx_power_dbm: number;
	bias_current_ma?: number;
	temperature_c?: number;
	voltage_v?: number;
	health: string;
	source: string;
	fetched_at?: string;
}

export interface DeviceCapability {
	device_id?: string;
	model?: string;
	band_type?: string;
	is_dual_band?: boolean;
	description?: string;
}

export interface WLANConfig {
	wlan: string;
	ssid: string;
	password?: string;
	band: string;
	hidden: boolean;
	auth_mode?: string;
	encryption?: string;
	/** Configured channel: "Auto" or a channel number (absent if the CPE hides it). */
	channel?: string;
	/** Operating channel bandwidth, e.g. "20MHz". */
	bandwidth?: string;
	enabled: boolean;
}

export interface WiFiStatsRadio {
	wlan: number;
	ssid?: string;
	band?: string;
	channel?: number;
	tx_power_percent?: number;
}

export interface WiFiStatsResponse {
	device_id: string;
	ip?: string;
	radios: WiFiStatsRadio[];
}

export interface WiFiClient {
	mac: string;
	wlan: number;
	ssid?: string;
	band?: string;
	signal_strength_dbm?: number;
	authenticated?: boolean;
}

export interface WiFiClientsResponse {
	device_id: string;
	ip?: string;
	clients: WiFiClient[];
}

export interface DHCPClient {
	mac: string;
	hostname: string;
	ip: string;
}

export interface MessageResponse {
	message: string;
}

export interface VersionResponse {
	version: string;
	commit: string;
	build_time: string;
	api_version: string;
	uptime: string;
}
