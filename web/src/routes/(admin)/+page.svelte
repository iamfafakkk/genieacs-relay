<script lang="ts">
	import { listDevices } from '$lib/api/devices';
	import { health, versionInfo } from '$lib/api/client';
	import type { DeviceSummary, VersionResponse } from '$lib/api/types';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Empty from '$lib/components/ui/empty';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import ServerIcon from '@lucide/svelte/icons/server';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import PackageIcon from '@lucide/svelte/icons/package';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let refreshing = $state(false);
	let devices = $state<DeviceSummary[]>([]);
	let count = $state(0);
	let hasMore = $state(false);
	let version = $state<VersionResponse | null>(null);
	let healthy = $state<boolean | null>(null);
	let error = $state('');

	const pageSize = 20;

	function formatTime(iso?: string): string {
		if (!iso) return '—';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return iso;
		return date.toLocaleString();
	}

	async function loadDevices() {
		refreshing = true;
		try {
			const res = await listDevices(1, pageSize);
			devices = res.devices;
			count = res.count;
			hasMore = res.has_more;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load devices.';
		} finally {
			refreshing = false;
		}
	}

	$effect(() => {
		loadDevices();
		health()
			.then(() => (healthy = true))
			.catch(() => (healthy = false));
		versionInfo()
			.then((v) => (version = v))
			.catch(() => (version = null));
	});

	const online = $derived(devices.filter((d) => d.ip).length);

	const stats = $derived([
		{ label: 'Total Devices', value: count + (hasMore ? '+' : ''), icon: ServerIcon },
		{ label: 'Devices Online', value: String(online), icon: WifiIcon },
		{ label: 'API Status', value: healthy === null ? '…' : healthy ? 'Healthy' : 'Down', icon: ActivityIcon },
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
		<Button variant="outline" size="sm" onclick={loadDevices} disabled={refreshing}>
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
			<AlertTitle>Failed to load data</AlertTitle>
			<AlertDescription>{error}</AlertDescription>
		</Alert>
	{/if}

	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
		{#each stats as stat (stat.label)}
			<Card.Root>
				<Card.Header class="flex-row items-center justify-between gap-2 space-y-0">
					<Card.Description>{stat.label}</Card.Description>
					<stat.icon class="text-muted-foreground size-4" />
				</Card.Header>
				<Card.Content>
					<p class="text-2xl font-semibold tracking-tight">{stat.value}</p>
				</Card.Content>
			</Card.Root>
		{/each}
	</div>

	<Card.Root>
		<Card.Header>
			<Card.Title>Devices</Card.Title>
			<Card.Description>First page of the GenieACS device collection.</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if refreshing && devices.length === 0}
				<div class="flex flex-col gap-2">
					{#each Array(5) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if devices.length === 0}
				<Empty.Root>
					<Empty.Header>
						<Empty.Media variant="icon">
							<InboxIcon />
						</Empty.Media>
						<Empty.Title>No devices yet</Empty.Title>
						<Empty.Description>
							GenieACS has not reported any device. Make sure your CPE is connected to the ACS.
						</Empty.Description>
					</Empty.Header>
				</Empty.Root>
			{:else}
				<div class="overflow-x-auto">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Device ID</Table.Head>
								<Table.Head>Model</Table.Head>
								<Table.Head>Serial</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head>Last Inform</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each devices as device (device.device_id)}
								<Table.Row>
									<Table.Cell class="max-w-64 truncate font-mono text-xs">{device.device_id}</Table.Cell>
									<Table.Cell>{device.model ?? '—'}</Table.Cell>
									<Table.Cell class="font-mono text-xs">{device.serial ?? '—'}</Table.Cell>
									<Table.Cell>
										{#if device.ip}
											<Badge variant="secondary">{device.ip}</Badge>
										{:else}
											<Badge variant="outline">offline</Badge>
										{/if}
									</Table.Cell>
									<Table.Cell class="text-muted-foreground">{formatTime(device.last_inform)}</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
