import { api } from '$lib/api/client';
import type { DevicesListResponse, DeviceSummary, MessageResponse } from '$lib/api/types';

/** Paginated device listing, optionally filtered by free-text search. Also used to validate the API key at login. */
export function listDevices(page = 1, pageSize = 50, search?: string): Promise<DevicesListResponse> {
	return api('/api/v1/genieacs/devices', {
		query: { page: String(page), page_size: String(pageSize), search }
	});
}

/**
 * Page through every device (page_size capped at 200 by the backend) so the
 * dashboard can aggregate the whole fleet. Bounded by maxPages to keep a
 * pathological fleet from hammering the API.
 */
export async function allDevices(maxPages = 25): Promise<DeviceSummary[]> {
	const all: DeviceSummary[] = [];
	for (let page = 1; page <= maxPages; page++) {
		const res = await listDevices(page, 200);
		all.push(...res.devices);
		if (!res.has_more || res.devices.length === 0) break;
	}
	return all;
}

/** Remove a device from GenieACS by its device ID. The CPE itself is not reset. */
export function deleteDevice(deviceId: string): Promise<MessageResponse> {
	return api(`/api/v1/genieacs/devices/${encodeURIComponent(deviceId)}`, { method: 'DELETE' });
}
