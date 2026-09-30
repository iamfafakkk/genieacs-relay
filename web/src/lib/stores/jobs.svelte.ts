/**
 * Global worker-job notifier.
 *
 * Polls GET /jobs and raises sonner toasts as jobs progress. Running jobs
 * reappear after a page refresh (their IDs are not persisted until they reach
 * a terminal state), while completed jobs are remembered in localStorage so a
 * refresh does not replay the whole history.
 */
import { toast } from 'svelte-sonner';
import { listJobs, jobLabel, type Job } from '$lib/api/jobs';

const POLL_MS = 4000;
const SEEN_KEY = 'gr_jobs_announced';
const MAX_SEEN = 200;

/** Shared reactive snapshot for the sidebar badge and the Jobs page. */
export const jobsState = $state({
	active: 0,
	count: 0,
	jobs: [] as Job[],
	loaded: false,
	error: ''
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

/** Fetch the current job list once and update the shared snapshot. */
export async function refreshJobs(): Promise<void> {
	try {
		const res = await listJobs(100);
		jobsState.jobs = res.jobs;
		jobsState.active = res.active;
		jobsState.count = res.count;
		jobsState.loaded = true;
		jobsState.error = '';
	} catch (e) {
		jobsState.error = e instanceof Error ? e.message : 'Failed to load jobs';
		jobsState.loaded = true;
	}
}

function announce(job: Job) {
	const title = `${jobLabel(job.type)} — ${job.device_id}`;
	if (job.status === 'success') {
		toast.success(title, { id: job.id, description: 'Completed' });
	} else {
		toast.error(title, { id: job.id, description: job.error || 'Failed' });
	}
}

/**
 * Start the polling loop. Idempotent: calling it while already running returns
 * a stop function without spawning a second interval.
 */
let running = false;
export function startJobNotifier(): () => void {
	if (running) return () => {};
	running = true;

	const seen = loadSeen();
	let seeded = false;
	let stopped = false;

	async function poll() {
		if (stopped) return;
		await refreshJobs();
		for (const job of jobsState.jobs) {
			if (isTerminal(job.status)) {
				if (!seen.has(job.id)) {
					if (seeded) announce(job);
					seen.add(job.id);
				}
			} else if (!seen.has(job.id)) {
				// Queued/running: keep a loading toast alive across refresh by
				// keying it on the stable job id.
				toast.loading(`${jobLabel(job.type)} — ${job.device_id}`, {
					id: job.id,
					description: 'Running…'
				});
			}
		}
		seeded = true;
		saveSeen(seen);
	}

	poll();
	const timer = setInterval(poll, POLL_MS);
	return () => {
		stopped = true;
		running = false;
		clearInterval(timer);
	};
}
