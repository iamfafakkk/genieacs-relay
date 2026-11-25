import { api } from '$lib/api/client';
import type {
	ProvisionsResponse,
	ScriptEntry,
	ScriptOperationResponse,
	VirtualParametersResponse
} from '$lib/api/types';

/**
 * GenieACS provision scripts and virtual parameters.
 * Both are NBI collections whose body is raw JavaScript, not JSON.
 */

const SCRIPT_HEADERS = { 'Content-Type': 'text/plain; charset=utf-8' };

/** Every provision script in GenieACS. */
export async function listProvisions(): Promise<ScriptEntry[]> {
	const res = await api<ProvisionsResponse>('/api/v1/genieacs/provisions');
	return res.provisions;
}

/** Create or replace a provision script. */
export function saveProvision(name: string, script: string): Promise<ScriptOperationResponse> {
	return api(`/api/v1/genieacs/provisions/${encodeURIComponent(name)}`, {
		method: 'PUT',
		body: script,
		headers: SCRIPT_HEADERS
	});
}

/** Delete a provision script. Deleting a missing one still succeeds. */
export function deleteProvision(name: string): Promise<ScriptOperationResponse> {
	return api(`/api/v1/genieacs/provisions/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

/** Every virtual parameter in GenieACS. */
export async function listVirtualParameters(): Promise<ScriptEntry[]> {
	const res = await api<VirtualParametersResponse>('/api/v1/genieacs/virtual-parameters');
	return res.virtual_parameters;
}

/** Create or replace a virtual parameter. */
export function saveVirtualParameter(
	name: string,
	script: string
): Promise<ScriptOperationResponse> {
	return api(`/api/v1/genieacs/virtual-parameters/${encodeURIComponent(name)}`, {
		method: 'PUT',
		body: script,
		headers: SCRIPT_HEADERS
	});
}

/** Delete a virtual parameter. Deleting a missing one still succeeds. */
export function deleteVirtualParameter(name: string): Promise<ScriptOperationResponse> {
	return api(`/api/v1/genieacs/virtual-parameters/${encodeURIComponent(name)}`, {
		method: 'DELETE'
	});
}
