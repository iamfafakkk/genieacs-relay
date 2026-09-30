/** Worker job monitoring (backend GET /api/v1/genieacs/jobs). */
import { api } from '$lib/api/client';

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

/** Human label for a job's task type. */
export function jobLabel(type: string): string {
	switch (type) {
		case 'setParameterValues':
			return 'Applying settings';
		case 'applyChanges':
			return 'Committing changes';
		case 'refreshWLAN':
			return 'Refreshing WLAN';
		default:
			return type;
	}
}
