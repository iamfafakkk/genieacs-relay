/**
 * Single-device operations. Every backend route identifies a device by its
 * WAN IP ({ip} path param), not by device_id, so callers pass the IP from the
 * DeviceSummary they navigated from.
 */
import { api } from '$lib/api/client';
import type {
	DeviceCapability,
	DeviceStatus,
	DHCPClient,
	MessageResponse,
	OpticalStats,
	WANConnectionsResponse,
	WiFiClientsResponse,
	WiFiStatsResponse,
	WLANConfig
} from '$lib/api/types';

export function deviceStatus(ip: string): Promise<DeviceStatus> {
	return api(`/api/v1/genieacs/status/${encodeURIComponent(ip)}`);
}

export function wanStatus(ip: string): Promise<WANConnectionsResponse> {
	return api(`/api/v1/genieacs/wan/${encodeURIComponent(ip)}`);
}

/** Optical stats. The backend 404s (OPTICAL_NOT_SUPPORTED) when the CPE exposes no optical tree. */
export function opticalStats(ip: string, refresh = false): Promise<OpticalStats> {
	return api(`/api/v1/genieacs/optical/${encodeURIComponent(ip)}`, {
		query: { refresh: refresh ? 'true' : undefined }
	});
}

export function deviceCapability(ip: string): Promise<DeviceCapability> {
	return api(`/api/v1/genieacs/capability/${encodeURIComponent(ip)}`);
}

/** WLAN slots currently broadcasting on the CPE. */
export function wlanConfigs(ip: string): Promise<WLANConfig[]> {
	return api(`/api/v1/genieacs/ssid/${encodeURIComponent(ip)}`);
}

export function wifiStats(ip: string): Promise<WiFiStatsResponse> {
	return api(`/api/v1/genieacs/wifi-stats/${encodeURIComponent(ip)}`);
}

export function wifiClients(ip: string): Promise<WiFiClientsResponse> {
	return api(`/api/v1/genieacs/wifi-clients/${encodeURIComponent(ip)}`);
}

export function dhcpClients(ip: string, refresh = false): Promise<DHCPClient[]> {
	return api(`/api/v1/genieacs/dhcp-client/${encodeURIComponent(ip)}`, {
		query: { refresh: refresh ? 'true' : undefined }
	});
}

// --- Actions (TR-069 tasks) ---

export function rebootDevice(ip: string): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/reboot/${encodeURIComponent(ip)}`, { method: 'POST' });
}

export function factoryResetDevice(ip: string): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/factory-reset/${encodeURIComponent(ip)}`, { method: 'POST' });
}

export function wakeDevice(ip: string): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/wake/${encodeURIComponent(ip)}`, { method: 'POST' });
}

export function setPPPoECredentials(
	ip: string,
	username: string,
	password: string,
	wanInstance = 1
): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/pppoe/${encodeURIComponent(ip)}`, {
		method: 'PUT',
		body: JSON.stringify({ username, password, wan_instance: wanInstance })
	});
}

export function wifiConnectivityRefresh(ip: string): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/ssid/${encodeURIComponent(ip)}/refresh`, { method: 'POST' });
}

export function updateWLAN(
	ip: string,
	wlan: string,
	body: Record<string, unknown>
): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/wlan/update/${encodeURIComponent(wlan)}/${encodeURIComponent(ip)}`, {
		method: 'PUT',
		body: JSON.stringify(body)
	});
}
