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
	WANConnectionMutationResponse,
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

// --- WAN connection CRUD ---

export interface CreateWANConnectionBody {
	type: 'pppoe' | 'dhcp' | 'static';
	wan_device?: number;
	connection_device?: number;
	/** Operator label for the new connection. Required: GenieACS uses it to seed the new instance. */
	name: string;
	enabled?: boolean;
	username?: string;
	password?: string;
	external_ip_address?: string;
	subnet_mask?: string;
	default_gateway?: string;
	dns_servers?: string;
}

export interface UpdateWANConnectionBody {
	enabled?: boolean;
	username?: string;
	password?: string;
	addressing_type?: string;
	external_ip_address?: string;
	subnet_mask?: string;
	default_gateway?: string;
	dns_servers?: string;
	/** Vendor extension: NAT (standard NATEnabled). */
	nat_enabled?: boolean;
	/** Vendor extension: VLAN tagging enable. */
	vlan_enabled?: boolean;
	/** Vendor extension: 802.1Q VLAN id (1-4094). */
	vlan_id?: number;
	/** Vendor extension: service tokens (e.g. ["INTERNET","TR069"]). */
	service_list?: string[];
}

function wanInstancePath(
	ip: string,
	type: string,
	wanDevice: number,
	connectionDevice: number,
	instance: number
): string {
	return `/api/v1/genieacs/wan/${encodeURIComponent(type)}/${wanDevice}/${connectionDevice}/${instance}/${encodeURIComponent(ip)}`;
}
/** Add a WAN connection. `instance` in the response is 0 when the AddObject task is queued. */
export function createWANConnection(
	ip: string,
	body: CreateWANConnectionBody
): Promise<WANConnectionMutationResponse> {
	return api(`/api/v1/genieacs/wan/${encodeURIComponent(ip)}`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

/** Edit fields on an existing WAN connection instance. */
export function updateWANConnection(
	ip: string,
	type: string,
	wanDevice: number,
	connectionDevice: number,
	instance: number,
	body: UpdateWANConnectionBody
): Promise<WANConnectionMutationResponse> {
	return api(wanInstancePath(ip, type, wanDevice, connectionDevice, instance), {
		method: 'PUT',
		body: JSON.stringify(body)
	});
}

/** Delete a WAN connection instance. */
export function deleteWANConnection(
	ip: string,
	type: string,
	wanDevice: number,
	connectionDevice: number,
	instance: number
): Promise<WANConnectionMutationResponse> {
	return api(wanInstancePath(ip, type, wanDevice, connectionDevice, instance), {
		method: 'DELETE'
	});
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

/** WLAN slots on the CPE. Default only enabled slots; `all` includes disabled ones. */
export function wlanConfigs(ip: string, all = false): Promise<WLANConfig[]> {
	return api(`/api/v1/genieacs/ssid/${encodeURIComponent(ip)}`, {
		query: { all: all ? 'true' : undefined }
	});
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

/** Radio settings for a WLAN slot: channel/mode/bandwidth/transmit power (partial). */
export function updateWLANRadio(
	ip: string,
	wlan: string,
	body: { channel?: string; mode?: string; bandwidth?: string; transmit_power?: number }
): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/wlan/optimize/${encodeURIComponent(wlan)}/${encodeURIComponent(ip)}`, {
		method: 'PUT',
		body: JSON.stringify(body)
	});
}

/** Toggle a WLAN slot on/off without touching its SSID or password. */
export function setWLANEnabled(
	ip: string,
	wlan: string,
	enabled: boolean
): Promise<MessageResponse> {
	const action = enabled ? 'enable' : 'delete';
	return api(`/api/v1/genieacs/wlan/${action}/${encodeURIComponent(wlan)}/${encodeURIComponent(ip)}`, {
		method: enabled ? 'PUT' : 'DELETE'
	});
}
