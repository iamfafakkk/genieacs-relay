<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Empty from '$lib/components/ui/empty';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import { jobsState, refreshJobs } from '$lib/stores/jobs.svelte';
	import { jobLabel, type Job, type JobStatus } from '$lib/api/jobs';
	import { onMount } from 'svelte';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let refreshing = $state(false);

	// Pull once on mount so navigating straight to /jobs shows data before the
	// layout poller's first tick.
	onMount(refreshJobs);

	async function manualRefresh() {
		refreshing = true;
		await refreshJobs();
		refreshing = false;
	}

	const STATUS_BADGE: Record<JobStatus, string> = {
		queued: 'border-muted-foreground/40 text-muted-foreground',
		running: 'border-primary/50 text-primary',
		success: 'border-success/50 text-success',
		failed: 'border-destructive/50 text-destructive'
	};

	const active = $derived(jobsState.jobs.filter((j) => j.status === 'queued' || j.status === 'running'));

	function formatTime(iso?: string): string {
		if (!iso) return '—';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return iso;
		return date.toLocaleString();
	}

	function formatDuration(job: Job): string {
		if (job.status === 'queued' || job.status === 'running') return '—';
		if (job.duration_ms < 1000) return `${job.duration_ms} ms`;
		return `${(job.duration_ms / 1000).toFixed(1)} s`;
	}
</script>

<svelte:head><title>Jobs · GenieACS Relay</title></svelte:head>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<h2 class="text-lg font-semibold">Jobs</h2>
			<p class="text-muted-foreground text-sm">
				Worker jobs assigned to devices ({jobsState.active} active · {jobsState.count} tracked)
			</p>
		</div>
		<div class="flex items-center gap-2">
			{#if jobsState.connected}
				<Badge variant="outline" class="border-success/50 text-success">live</Badge>
			{:else}
				<Badge variant="outline" class="border-muted-foreground/40 text-muted-foreground">polling</Badge>
			{/if}
			<Button variant="outline" size="sm" onclick={manualRefresh} disabled={refreshing}>
				{#if refreshing}
					<Spinner data-icon="inline-start" />
				{:else}
					<RefreshCwIcon data-icon="inline-start" />
				{/if}
				Refresh
			</Button>
		</div>
	</div>

	{#if jobsState.error}
		<Alert variant="destructive">
			<TriangleAlertIcon />
			<AlertTitle>Failed to load jobs</AlertTitle>
			<AlertDescription>{jobsState.error}</AlertDescription>
		</Alert>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Monitoring</Card.Title>
			<Card.Description>
				Live progress of queued and running jobs. Notifications appear via toast; jobs still
				running when you refresh re-appear here.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if !jobsState.loaded}
				<div class="flex flex-col gap-2">
					{#each Array(5) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if jobsState.jobs.length === 0}
				<Empty.Root>
					<Empty.Header>
						<Empty.Media variant="icon">
							<InboxIcon />
						</Empty.Media>
						<Empty.Title>No jobs yet</Empty.Title>
						<Empty.Description>
							Worker jobs are created when you push configuration to a device (WLAN edits,
							PPPoE credentials, refreshes). They appear here while running and after they finish.
						</Empty.Description>
					</Empty.Header>
				</Empty.Root>
			{:else}
				<div class="overflow-x-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Status</Table.Head>
								<Table.Head>Job</Table.Head>
								<Table.Head>Device</Table.Head>
								<Table.Head>Params</Table.Head>
								<Table.Head>Created</Table.Head>
								<Table.Head>Duration</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each jobsState.jobs as job (job.id)}
								<Table.Row>
									<Table.Cell>
										<Badge variant="outline" class={STATUS_BADGE[job.status]}>
											{#if job.status === 'queued' || job.status === 'running'}
												<LoaderCircleIcon class="animate-spin" />
											{/if}
											{job.status}
										</Badge>
									</Table.Cell>
									<Table.Cell>
										<span class="font-medium">{jobLabel(job.type)}</span>
										<span class="text-muted-foreground ml-1 font-mono text-xs">{job.type}</span>
									</Table.Cell>
									<Table.Cell class="max-w-64 truncate font-mono text-xs">
										<a
											class="hover:text-primary underline-offset-4 hover:underline"
											href={`/devices/${encodeURIComponent(job.device_id)}`}
										>
											{job.device_id}
										</a>
									</Table.Cell>
									<Table.Cell class="font-mono text-xs">{job.parameter_count}</Table.Cell>
									<Table.Cell class="text-muted-foreground">{formatTime(job.created_at)}</Table.Cell>
									<Table.Cell class="font-mono text-xs">
										{formatDuration(job)}
										{#if job.status === 'failed' && job.error}
											<span class="text-destructive block max-w-64 truncate" title={job.error}>
												{job.error}
											</span>
										{/if}
									</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
		<Card.Footer>
			<p class="text-muted-foreground text-sm">
				{active.length} active · {jobsState.jobs.length} shown. Job history is in-memory and resets
				when the relay restarts.
			</p>
		</Card.Footer>
	</Card.Root>
</div>
