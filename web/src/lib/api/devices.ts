import { api } from '$lib/api/client';
import type { DevicesListResponse } from '$lib/api/types';

/** Paginated device listing, optionally filtered by free-text search. Also used to validate the API key at login. */
export function listDevices(page = 1, pageSize = 50, search?: string): Promise<DevicesListResponse> {
	return api('/api/v1/genieacs/devices', {
		query: { page: String(page), page_size: String(pageSize), search }
	});
}
