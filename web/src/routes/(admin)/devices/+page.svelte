<script lang="ts">
	import { allDevices, listDevices } from '$lib/api/devices';
	import type { DeviceSummary } from '$lib/api/types';
	import { page as routePage } from '$app/state';
	import { goto } from '$app/navigation';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Empty from '$lib/components/ui/empty';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import XIcon from '@lucide/svelte/icons/x';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let refreshing = $state(false);
	let devices = $state<DeviceSummary[]>([]);
	let page = $state(1);
	let count = $state(0);
	let hasMore = $state(false);
	let search = $state('');
	let error = $state('');

	const pageSize = 20;

	// Optional `?rx=` filter driven by the dashboard's Optical Signal Health
	// card. Handled client-side (the backend has no numeric RX filter), so in
	// this mode the whole fleet is fetched and paged in the browser.
	type RxBucket = 'healthy' | 'weak' | 'critical';
	const RX_LABEL: Record<RxBucket, string> = {
		healthy: 'Healthy (> -27 dBm)',
		weak: 'Weak (-27 to -30 dBm)',
		critical: 'Critical (≤ -30 dBm)'
	};
	const rxFilter = $derived(routePage.url.searchParams.get('rx') as RxBucket | null);

	function inRxBucket(dbm: number | undefined, bucket: RxBucket): boolean {
		// 0/absent = the CPE reports no optics.
		if (typeof dbm !== 'number' || dbm === 0) return false;
		if (bucket === 'healthy') return dbm > -27;
		if (bucket === 'weak') return dbm <= -27 && dbm > -30;
		return dbm <= -30;
	}

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
			if (rxFilter) {
				// Client-side RX bucket: fetch the fleet once, then filter + page in memory.
				const all = await allDevices();
				const matched = all.filter((d) => inRxBucket(d.rx_power_dbm, rxFilter));
				const start = (page - 1) * pageSize;
				devices = matched.slice(start, start + pageSize);
				count = matched.length;
				hasMore = start + pageSize < matched.length;
			} else {
				const res = await listDevices(page, pageSize, search);
				devices = res.devices;
				count = res.count;
				hasMore = res.has_more;
			}
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load devices.';
		} finally {
			refreshing = false;
		}
	}

	function goToPage(next: number) {
		page = next;
	}

	// Server-side search across the whole collection. Any term matching
	// model, MAC, serial, or PPPoE username resets to page 1.
	function applySearch(term: string) {
		search = term;
		page = 1;
	}

	// Clear the ?rx= filter and return to the normal server-side listing.
	function clearRxFilter() {
		search = '';
		page = 1;
		goto('/devices', { invalidateAll: true });
	}

	// Single loader: loadDevices reads page/search/rxFilter synchronously, so
	// this effect re-runs (and refetches) whenever any of them changes.
	$effect(() => {
		loadDevices();
	});
</script>

<svelte:head><title>Devices · GenieACS Relay</title></svelte:head>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<h2 class="text-lg font-semibold">Devices</h2>
			<p class="text-muted-foreground text-sm">GenieACS device collection, {pageSize} per page.</p>
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
			<AlertTitle>Failed to load devices</AlertTitle>
			<AlertDescription>{error}</AlertDescription>
		</Alert>
	{/if}

	<Card.Root>
		<Card.Header>
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title>All devices</Card.Title>
					<Card.Description>
						{#if rxFilter}
							Optical RX filter · {RX_LABEL[rxFilter]}
						{:else}
							Search by model, MAC, serial, or PPPoE user.
						{/if}
					</Card.Description>
				</div>
				{#if rxFilter}
					<Button variant="outline" size="sm" onclick={clearRxFilter}>
						<XIcon data-icon="inline-start" />
						Clear RX filter
					</Button>
				{:else}
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
				{/if}
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
						<Empty.Title>
							{#if rxFilter}
								No devices in this bucket
							{:else if search}
								No matching devices
							{:else}
								No devices yet
							{/if}
						</Empty.Title>
						<Empty.Description>
							{#if rxFilter}
								No device reports optical RX power in “{RX_LABEL[rxFilter]}”.
							{:else if search}
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
							</Table.Row>
						</Table.Header>
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
				{hasMore ? '+' : ''} devices{search ? ` matching “${search}”` : ''}{rxFilter
					? ` · ${RX_LABEL[rxFilter]}`
					: ''}
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
