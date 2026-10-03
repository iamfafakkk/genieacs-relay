/**
 * Global worker-job notifier.
 *
 * Subscribes to the backend WebSocket (/jobs/ws) and raises sonner toasts as
 * jobs progress. The backend pushes only on change, so there is no idle
 * polling. If the socket cannot connect, it falls back to polling so the panel
 * still works. Terminal jobs are remembered in localStorage so a page refresh
 * does not replay the whole history; running jobs reappear after refresh.
 */
import { toast } from 'svelte-sonner';
import { listJobs, jobsSocketUrl, jobLabel, type Job } from '$lib/api/jobs';
import { getApiKey } from '$lib/api/client';

const SEEN_KEY = 'gr_jobs_announced';
const MAX_SEEN = 200;
const FALLBACK_POLL_MS = 5000;
const RECONNECT_MIN_MS = 1000;
const RECONNECT_MAX_MS = 15000;

/** Shared reactive snapshot for the sidebar badge and the Jobs page. */
export const jobsState = $state({
	active: 0,
	count: 0,
	jobs: [] as Job[],
	loaded: false,
	error: '',
	connected: false
});

const isTerminal = (s: Job['status']) => s === 'success' || s === 'failed';

function loadSeen(): Set<string> {
	if (typeof localStorage === 'undefined') return new Set();
	try {
		return new Set<string>(JSON.parse(localStorage.getItem(SEEN_KEY) ?? '[]'));
	} catch {
		return new Set();
	}
}

function saveSeen(seen: Set<string>): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(SEEN_KEY, JSON.stringify([...seen].slice(-MAX_SEEN)));
}

/** Fetch the current job list once (fallback / manual refresh). */
export async function refreshJobs(): Promise<void> {
	try {
		apply(await listJobs(100));
	} catch (e) {
		jobsState.error = e instanceof Error ? e.message : 'Failed to load jobs';
		jobsState.loaded = true;
	}
}

function announce(job: Job) {
	const title = `${jobLabel(job.type)} — ${job.device_id}`;
	// Reset to a finite duration: the loading toast was created with
	// `duration: Infinity` so it survived the whole run, and updateToast would
	// otherwise carry that Infinity into the terminal toast (never auto-closing).
	const opts = { id: job.id, description: 'Completed', duration: 5000 };
	if (job.status === 'success') {
		toast.success(title, opts);
	} else {
		toast.error(title, { ...opts, description: job.error || 'Failed' });
	}
}

let seen = new Set<string>();
let seeded = false;
let pollTimer: ReturnType<typeof setInterval> | undefined;

/** Replace the snapshot and fire notifications for state transitions. */
function apply(res: { jobs: Job[]; active: number; count: number }) {
	jobsState.jobs = res.jobs;
	jobsState.active = res.active;
	jobsState.count = res.count;
	jobsState.loaded = true;
	jobsState.error = '';

	for (const job of res.jobs) {
		if (isTerminal(job.status)) {
			if (!seen.has(job.id)) {
				if (seeded) announce(job);
				seen.add(job.id);
			}
		} else if (!seen.has(job.id)) {
			// Queued/running: keep a loading toast alive across refresh by
			// keying it on the stable job id. duration: Infinity is essential —
			// without it sonner falls back to the 4s default and the toast
			// vanishes while the job is still running.
			toast.loading(`${jobLabel(job.type)} — ${job.device_id}`, {
				id: job.id,
				description: 'Running…',
				duration: Number.POSITIVE_INFINITY
			});
		}
	}
	seeded = true;
	saveSeen(seen);
}

function stopFallbackPoll() {
	if (pollTimer) {
		clearInterval(pollTimer);
		pollTimer = undefined;
	}
}

function startFallbackPoll() {
	if (pollTimer) return;
	pollTimer = setInterval(refreshJobs, FALLBACK_POLL_MS);
}

/**
 * Connect the live job stream. Returns a stop function. Calling it again
 * replaces any previous session (so a dev HMR reload cannot leave a dead
 * notifier behind).
 */
let activeStop: (() => void) | null = null;
export function startJobNotifier(): () => void {
	// Tear down a previous session first (idempotent safety for HMR).
	activeStop?.();

	seen = loadSeen();
	seeded = false;

	let ws: WebSocket | null = null;
	let stopped = false;
	let reconnectDelay = RECONNECT_MIN_MS;
	let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

	const connect = () => {
		if (stopped) return;
		// Pass the key in the query string too: harmless when the middleware is
		// off, and it lets a non-browser client connect without a cookie.
		const key = getApiKey();
		const url = jobsSocketUrl() + (key ? `?api_key=${encodeURIComponent(key)}` : '');
		try {
			ws = new WebSocket(url);
		} catch {
			startFallbackPoll();
			return;
		}

		ws.onopen = () => {
			reconnectDelay = RECONNECT_MIN_MS;
			jobsState.connected = true;
			jobsState.error = '';
			stopFallbackPoll();
		};
		ws.onmessage = (ev) => {
			try {
				apply(JSON.parse(ev.data));
			} catch {
				/* ignore malformed frame */
			}
		};
		ws.onclose = () => {
			jobsState.connected = false;
			if (stopped) return;
			startFallbackPoll();
			reconnectTimer = setTimeout(connect, reconnectDelay);
			reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
		};
		ws.onerror = () => ws?.close();
	};

	const stop = () => {
		stopped = true;
		if (reconnectTimer) clearTimeout(reconnectTimer);
		stopFallbackPoll();
		ws?.close();
		ws = null;
	};

	activeStop = stop;
	connect();
	// Seed immediately in case the socket is slow to open.
	refreshJobs();

	return () => {
		if (activeStop === stop) activeStop = null;
		stop();
	};
}
