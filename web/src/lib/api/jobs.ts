/** Worker job monitoring. Live updates arrive over the WebSocket at /jobs/ws. */
import { api, API_BASE } from '$lib/api/client';

export type JobStatus = 'queued' | 'running' | 'success' | 'failed';

export interface Job {
	id: string;
	type: string;
	device_id: string;
	status: JobStatus;
	error?: string;
	parameter_count: number;
	created_at: string;
	started_at?: string;
	finished_at?: string;
	duration_ms: number;
}

export interface JobListResponse {
	jobs: Job[];
	active: number;
	count: number;
}

/** In-memory job history, newest first. Disappears on relay restart. */
export function listJobs(limit = 100): Promise<JobListResponse> {
	return api('/api/v1/genieacs/jobs', { query: { limit: String(limit) } });
}

/**
 * WebSocket URL for the live job stream.
 *
 * Browsers cannot set headers on a WebSocket, so the API key travels in the
 * `gr_api_key` cookie the same origin already holds. In dev, Vite's proxy
 * upgrades the `ws:` connection to the Go backend just like `http:`.
 */
export function jobsSocketUrl(): string {
	const base = new URL(`${API_BASE || ''}/api/v1/genieacs/jobs/ws`, location.origin);
	base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
	return base.toString();
}

/** Human label for a job's task type. */
export function jobLabel(type: string): string {
	switch (type) {
		case 'setParameterValues':
			return 'Applying settings';
		case 'applyChanges':
			return 'Committing changes';
		case 'refreshWLAN':
			return 'Refreshing WLAN';
		case 'wake':
			return 'Summoning device';
		case 'wanAdd':
			return 'Adding WAN connection';
		case 'wanUpdate':
			return 'Editing WAN connection';
		case 'wanDelete':
			return 'Deleting WAN connection';
		default:
			return type;
	}
}
