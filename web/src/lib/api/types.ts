/** Shapes mirrored from the Go models (models.go, handlers_devices.go). */

export interface DeviceSummary {
	device_id: string;
	ip?: string;
	last_inform?: string;
	manufacturer?: string;
	model?: string;
	serial?: string;
	mac?: string;
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

export interface VersionResponse {
	version: string;
	commit: string;
	build_time: string;
	api_version: string;
	uptime: string;
}
