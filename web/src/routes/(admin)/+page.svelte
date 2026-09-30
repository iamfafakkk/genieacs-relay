<script lang="ts">
	import { allDevices } from '$lib/api/devices';
	import { health, versionInfo } from '$lib/api/client';
	import { jobsState } from '$lib/stores/jobs.svelte';
	import type { DeviceSummary, VersionResponse } from '$lib/api/types';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import ServerIcon from '@lucide/svelte/icons/server';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import PackageIcon from '@lucide/svelte/icons/package';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
	import TimerIcon from '@lucide/svelte/icons/timer';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let devices = $state<DeviceSummary[]>([]);
	let loaded = $state(false);
	let refreshing = $state(false);
	let version = $state<VersionResponse | null>(null);
	let healthy = $state<boolean | null>(null);
	let error = $state('');

	// Online = informed within the last 5 minutes (matches the device list).
	const ONLINE_WINDOW_MS = 5 * 60 * 1000;

	function isOnline(device: DeviceSummary): boolean {
		if (!device.last_inform) return false;
		const t = new Date(device.last_inform).getTime();
		return !Number.isNaN(t) && Date.now() - t < ONLINE_WINDOW_MS;
	}

	async function load() {
		refreshing = true;
		try {
			const [list, v, h] = await Promise.allSettled([allDevices(), versionInfo(), health()]);
			if (list.status === 'fulfilled') devices = list.value;
			if (v.status === 'fulfilled') version = v.value;
			if (h.status === 'fulfilled') healthy = true;
			else healthy = false;
			error = list.status === 'rejected' ? (list.reason as Error).message : '';
		} finally {
			loaded = true;
			refreshing = false;
		}
	}

	$effect(() => {
		load();
	});

	const total = $derived(devices.length);
	const online = $derived(devices.filter(isOnline).length);
	const offline = $derived(total - online);
	const onlinePct = $derived(total ? Math.round((online / total) * 100) : 0);

	// Fleet breakdowns. rx_power_dbm is present only for CPEs reporting optics.
	const rxWindow = $derived(devices.filter((d) => typeof d.rx_power_dbm === 'number' && d.rx_power_dbm !== 0));
	const withRx = $derived(rxWindow.length);
	const rxHealthy = $derived(rxWindow.filter((d) => (d.rx_power_dbm as number) > -27).length);
	const rxWeak = $derived(rxWindow.filter((d) => { const v = d.rx_power_dbm as number; return v <= -27 && v > -30; }).length);
	const rxCritical = $derived(rxWindow.filter((d) => (d.rx_power_dbm as number) <= -30).length);

	// Top models by device count.
	const byModel = $derived(
		[...devices.reduce((m, d) => {
			const key = d.model || 'Unknown';
			m.set(key, (m.get(key) ?? 0) + 1);
			return m;
		}, new Map<string, number>())].sort((a, b) => b[1] - a[1]).slice(0, 6)
	);

	const stats = $derived([
		{ label: 'Total Devices', value: String(total), icon: ServerIcon },
		{ label: 'Online', value: total ? `${online} · ${onlinePct}%` : String(online), icon: WifiIcon },
		{ label: 'Offline', value: String(offline), icon: WifiOffIcon },
		{
			label: 'API Status',
			value: healthy === null ? '…' : healthy ? 'Healthy' : 'Down',
			icon: ActivityIcon
		},
		{
			label: 'Active Jobs',
			value: String(jobsState.active),
			icon: ListChecksIcon
		},
		{ label: 'Version', value: version ? version.version : '—', icon: PackageIcon }
	]);
</script>

<svelte:head><title>Dashboard · GenieACS Relay</title></svelte:head>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<h2 class="text-lg font-semibold">Dashboard</h2>
			<p class="text-muted-foreground text-sm">
				GenieACS relay overview · uptime {version?.uptime ?? '—'}
			</p>
		</div>
		<Button variant="outline" size="sm" onclick={load} disabled={refreshing}>
			{#if refreshing}
				<Spinner data-icon="inline-start" />
			{:else}
				<RefreshCwIcon data-icon="inline-start" />
			{/if}
			Refresh
		</Button>
	</div>

	{#if error}
		<Alert variant="destructive">
			<TriangleAlertIcon />
			<AlertTitle>Failed to load statistics</AlertTitle>
			<AlertDescription>{error}</AlertDescription>
		</Alert>
	{/if}

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
		{#each stats as stat (stat.label)}
			<Card.Root>
				<Card.Header class="flex-row items-center justify-between gap-2 space-y-0">
					<Card.Description>{stat.label}</Card.Description>
					<stat.icon class="text-muted-foreground size-4" />
				</Card.Header>
				<Card.Content>
					{#if refreshing && !loaded}
						<Skeleton class="h-8 w-24" />
					{:else}
						<p class="text-2xl font-semibold tracking-tight">{stat.value}</p>
					{/if}
				</Card.Content>
			</Card.Root>
		{/each}
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<Card.Root>
			<Card.Header class="flex-row items-center justify-between gap-2 space-y-0">
				<div>
					<Card.Title>Optical Signal Health</Card.Title>
					<Card.Description>{withRx} of {total} devices report optics.</Card.Description>
				</div>
				<TimerIcon class="text-muted-foreground size-4" />
			</Card.Header>
			<Card.Content class="flex flex-col gap-4">
				{#if refreshing && !loaded}
					<Skeleton class="h-24 w-full" />
				{:else if withRx === 0}
					<p class="text-muted-foreground text-sm">No optical data reported by any device.</p>
				{:else}
					{@render signalRow('Healthy (> -27 dBm)', rxHealthy, withRx, 'healthy', 'border-success/50 text-success')}
					{@render signalRow('Weak (-27 to -30 dBm)', rxWeak, withRx, 'weak', 'border-primary/50 text-primary')}
					{@render signalRow('Critical (≤ -30 dBm)', rxCritical, withRx, 'critical', 'border-destructive/50 text-destructive')}
				{/if}
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header class="flex-row items-center justify-between gap-2 space-y-0">
				<div>
					<Card.Title>Devices by Model</Card.Title>
					<Card.Description>Top models in the fleet.</Card.Description>
				</div>
				<ServerIcon class="text-muted-foreground size-4" />
			</Card.Header>
			<Card.Content>
				{#if refreshing && !loaded}
					<Skeleton class="h-24 w-full" />
				{:else if byModel.length === 0}
					<p class="text-muted-foreground text-sm">No devices yet.</p>
				{:else}
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Model</Table.Head>
								<Table.Head class="text-right">Devices</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each byModel as [model, n] (model)}
								<Table.Row>
									<Table.Cell class="font-medium">{model}</Table.Cell>
									<Table.Cell class="text-right font-mono text-xs">{n}</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				{/if}
			</Card.Content>
			<Card.Footer>
				<Button variant="outline" size="sm" href="/devices">
					Browse devices
				</Button>
			</Card.Footer>
		</Card.Root>
	</div>
</div>

{#snippet signalRow(label: string, value: number, denom: number, bucket: string, badgeClass: string)}
	<a
		class="hover:bg-muted/50 -mx-2 flex items-center justify-between gap-2 rounded-md px-2 py-2 transition-colors"
		href={`/devices?rx=${bucket}`}
	>
		<span class="text-sm">{label}</span>
		<Badge variant="outline" class={badgeClass}>
			{value}
			<span class="text-muted-foreground">/ {denom}</span>
		</Badge>
	</a>
{/snippet}
