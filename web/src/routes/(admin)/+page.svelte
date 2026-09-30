<script lang="ts">
	import { listDevices } from '$lib/api/devices';
	import { health, versionInfo } from '$lib/api/client';
	import type { DeviceSummary, VersionResponse } from '$lib/api/types';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Empty from '$lib/components/ui/empty';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import ServerIcon from '@lucide/svelte/icons/server';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import PackageIcon from '@lucide/svelte/icons/package';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let refreshing = $state(false);
	let devices = $state<DeviceSummary[]>([]);
	let page = $state(1);
	let count = $state(0);
	let hasMore = $state(false);
	let search = $state('');
	let version = $state<VersionResponse | null>(null);
	let healthy = $state<boolean | null>(null);
	let error = $state('');

	const pageSize = 20;

	// Online = informed within the last 5 minutes.
	const ONLINE_WINDOW_MS = 5 * 60 * 1000;

	function isOnline(device: DeviceSummary): boolean {
		if (!device.last_inform) return false;
		const t = new Date(device.last_inform).getTime();
		return !Number.isNaN(t) && Date.now() - t < ONLINE_WINDOW_MS;
	}

	function formatTime(iso?: string): string {
		if (!iso) return '—';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return iso;
		return date.toLocaleString();
	}

	function formatRxPower(dbm?: number): string {
		return typeof dbm === 'number' && dbm !== 0 ? `${dbm.toFixed(2)} dBm` : '—';
	}

	async function loadDevices() {
		refreshing = true;
		try {
			const res = await listDevices(page, pageSize, search);
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

	function goToPage(next: number) {
		page = next;
		loadDevices();
	}

	// Server-side search across the whole collection. Any term matching
	// model, MAC, serial, or PPPoE username resets to page 1.
	function applySearch(term: string) {
		search = term;
		page = 1;
		loadDevices();
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

	const online = $derived(devices.filter(isOnline).length);

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
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title>Devices</Card.Title>
					<Card.Description>GenieACS device collection, {pageSize} per page.</Card.Description>
				</div>
				<form
					class="relative w-full sm:w-72"
					onsubmit={(e) => {
						e.preventDefault();
						applySearch(search.trim());
					}}
				>
					<SearchIcon
						class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
					/>
					<Input
						class="pl-9"
						placeholder="Search model, MAC, serial, PPPoE user…"
						bind:value={search}
						oninput={(e) => applySearch(e.currentTarget.value.trim())}
					/>
				</form>
			</div>
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
						<Empty.Title>{search ? 'No matching devices' : 'No devices yet'}</Empty.Title>
						<Empty.Description>
							{#if search}
								No device matches “{search}”. Try a different model, MAC, serial, or PPPoE user.
							{:else}
								GenieACS has not reported any device. Make sure your CPE is connected to the ACS.
							{/if}
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
								<Table.Head>PPPoE User</Table.Head>
								<Table.Head>Rx Power</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head>Status</Table.Head>
								<Table.Head>Last Inform</Table.Head>
							</Table.Row>						</Table.Header>
						<Table.Body>
							{#each devices as device (device.device_id)}
								<Table.Row>
									<Table.Cell class="max-w-64 truncate font-mono text-xs">
										<a
											class="hover:text-primary underline-offset-4 hover:underline"
											href={`/devices/${encodeURIComponent(device.device_id)}`}
										>
											{device.device_id}
										</a>
									</Table.Cell>
									<Table.Cell>{device.model ?? '—'}</Table.Cell>
									<Table.Cell class="font-mono text-xs">{device.serial ?? '—'}</Table.Cell>
									<Table.Cell class="font-mono text-xs">{device.pppoe_username ?? '—'}</Table.Cell>
									<Table.Cell class="font-mono text-xs">{formatRxPower(device.rx_power_dbm)}</Table.Cell>
									<Table.Cell>
										{#if device.ip}
											<Badge
												variant="outline"
												class="border-success/50 text-success"
												href={`http://${device.ip}`}
												target="_blank"
												rel="noopener noreferrer">{device.ip}</Badge
											>
										{:else}
											<span class="text-muted-foreground">—</span>
										{/if}
									</Table.Cell>
									<Table.Cell>
										{#if isOnline(device)}
											<Badge variant="outline" class="border-success/50 text-success">Online</Badge>
										{:else}
											<Badge variant="outline" class="border-destructive/50 text-destructive">
												Offline
											</Badge>
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
		<Card.Footer class="flex items-center justify-between gap-2">
			<p class="text-muted-foreground text-sm">
				Page {page} · {count}
				{hasMore ? '+' : ''} devices{search ? ` matching “${search}”` : ''}
			</p>
			<div class="flex items-center gap-2">
				<Button
					variant="outline"
					size="sm"
					disabled={page <= 1 || refreshing}
					onclick={() => goToPage(page - 1)}
				>
					<ChevronLeftIcon data-icon="inline-start" />
					Previous
				</Button>
				<Button
					variant="outline"
					size="sm"
					disabled={!hasMore || refreshing}
					onclick={() => goToPage(page + 1)}
				>
					Next
					<ChevronRightIcon data-icon="inline-end" />
				</Button>
			</div>
		</Card.Footer>
	</Card.Root>
</div>
