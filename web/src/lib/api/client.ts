/**
 * Single access point for the genieacs-relay REST API.
 * Browser → Go backend (CORS + X-API-Key auth).
 */

const ENV_BASE = import.meta.env.VITE_API_BASE_URL as string | undefined;

/** Base URL of the Go backend. Empty in dev → Vite proxy (see vite.config.ts). */
export const API_BASE = (ENV_BASE ?? '').replace(/\/$/, '');

/** Cookie holding the admin API key. Read by the (admin) layout guard. */
export const KEY_COOKIE = 'gr_api_key';

export interface ApiEnvelope<T> {
	code: number;
	status: string;
	data?: T;
	error_code?: string;
	request_id?: string;
}

export class ApiError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly errorCode?: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

/** Read the stored API key (browser only). */
export function getApiKey(): string | null {
	if (typeof document === 'undefined') return null;
	const match = document.cookie.match(new RegExp(`(?:^|; )${KEY_COOKIE}=([^;]*)`));
	return match ? decodeURIComponent(match[1]) : null;
}

/** Persist / clear the API key. `secure` on in production. */
export function setApiKey(key: string | null): void {
	if (typeof document === 'undefined') return;
	const secure = location.protocol === 'https:' ? '; secure' : '';
	document.cookie =
		key === null
			? `${KEY_COOKIE}=; path=/; max-age=0${secure}`
			: `${KEY_COOKIE}=${encodeURIComponent(key)}; path=/; max-age=${60 * 60 * 24 * 7}; samesite=lax${secure}`;
}

/** Error message from an API envelope, falling back to a generic string. */
function errorMessage(body: ApiEnvelope<unknown>): string {
	const data = body.data;
	if (typeof data === 'string') return data;
	if (data && typeof data === 'object' && 'message' in data) return String(data.message);
	return body.status || 'Request failed';
}

/**
 * Fetch the backend and unwrap the `{code,status,data}` envelope.
 * @param query optional query params; undefined values are dropped.
 */
export async function api<T>(
	path: string,
	init: RequestInit & { query?: Record<string, string | undefined> } = {}
): Promise<T> {
	const { query, ...rest } = init;
	const url = new URL(`${API_BASE}${path}`, location.origin);
	for (const [key, value] of Object.entries(query ?? {})) {
		if (value !== undefined && value !== '') url.searchParams.set(key, value);
	}

	const key = getApiKey();
	const res = await fetch(url, {
		...rest,
		headers: {
			...(rest.body ? { 'Content-Type': 'application/json' } : {}),
			...(key ? { 'X-API-Key': key } : {}),
			...rest.headers
		}
	});

	let body: ApiEnvelope<T>;
	try {
		body = (await res.json()) as ApiEnvelope<T>;
	} catch {
		throw new ApiError(`HTTP ${res.status}`, res.status);
	}

	if (!res.ok) {
		// Stale/invalid key → drop it so the next visit lands on /login.
		if (res.status === 401) setApiKey(null);
		throw new ApiError(errorMessage(body), res.status, body.error_code);
	}
	return body.data as T;
}

/** Backend `/health` — `{status:"healthy"}`. Public. */
export function health(): Promise<{ status: string }> {
	return api<{ status: string }>('/health');
}

/** Backend `/version` — build metadata. Public, NOT wrapped in the envelope. */
export async function versionInfo(): Promise<import('$lib/api/types').VersionResponse> {
	const res = await fetch(`${API_BASE}/version`);
	if (!res.ok) throw new ApiError(`HTTP ${res.status}`, res.status);
	return (await res.json()) as import('$lib/api/types').VersionResponse;
}
