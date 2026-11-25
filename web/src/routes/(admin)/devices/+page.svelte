<script lang="ts">
	import { allDevices, deleteDevice, listDevices } from '$lib/api/devices';
	import type { DeviceSummary } from '$lib/api/types';
	import { toast } from 'svelte-sonner';
	import { page as routePage } from '$app/state';
	import { goto } from '$app/navigation';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
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
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
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

	// Optional `?offline=N` filter driven by the dashboard's Offline Duration
	// card: devices whose last inform is older than N days. Also client-side.
	const offlineDays = $derived.by(() => {
		const raw = routePage.url.searchParams.get('offline');
		if (!raw) return null;
		const n = Number(raw);
		return Number.isInteger(n) && n > 0 ? n : null;
	});

	function inRxBucket(dbm: number | undefined, bucket: RxBucket): boolean {
		// 0/absent = the CPE reports no optics.
		if (typeof dbm !== 'number' || dbm === 0) return false;
		if (bucket === 'healthy') return dbm > -27;
		if (bucket === 'weak') return dbm <= -27 && dbm > -30;
		return dbm <= -30;
	}

	// Online = informed within the last 5 minutes.
	const ONLINE_WINDOW_MS = 5 * 60 * 1000;
	const DAY_MS = 24 * 60 * 60 * 1000;

	function informAgeMs(device: DeviceSummary): number | null {
		if (!device.last_inform) return null;
		const t = new Date(device.last_inform).getTime();
		return Number.isNaN(t) ? null : Date.now() - t;
	}

	function isOnline(device: DeviceSummary): boolean {
		const age = informAgeMs(device);
		return age !== null && age < ONLINE_WINDOW_MS;
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
			if (rxFilter || offlineDays !== null) {
				// Client-side fleet filter: fetch once, then filter + page in memory.
				let matched = await allDevices();
				if (rxFilter) matched = matched.filter((d) => inRxBucket(d.rx_power_dbm, rxFilter));
				if (offlineDays !== null) {
					const threshold = offlineDays * DAY_MS;
					matched = matched.filter((d) => {
						const age = informAgeMs(d);
						return age !== null && age > threshold;
					});
				}
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

	let deleting = $state<string | null>(null);

	// Remove the device from GenieACS (no CPE reset). The list is reloaded so
	// the row disappears without a manual refresh.
	async function removeDevice(device: DeviceSummary) {
		deleting = device.device_id;
		try {
			const res = await deleteDevice(device.device_id);
			toast.success(res.message);
			await loadDevices();
		} catch (e) {
			toast.error(`Delete failed: ${e instanceof Error ? e.message : 'Unknown error'}`);
		} finally {
			deleting = null;
		}
	}

	// Server-side search across the whole collection. Any term matching
	// model, MAC, serial, or PPPoE username resets to page 1.
	function applySearch(term: string) {
		search = term;
		page = 1;
	}

	// Clear the ?rx= / ?offline= filter and return to the normal server-side listing.
	function clearFilter() {
		search = '';
		page = 1;
		goto('/devices', { invalidateAll: true });
	}

	// Single loader: loadDevices reads page/search/rxFilter/offlineDays
	// synchronously, so this effect re-runs (and refetches) whenever any of
	// them changes.
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
						{#if offlineDays !== null}
							Offline filter · more than {offlineDays} days
						{:else if rxFilter}
							Optical RX filter · {RX_LABEL[rxFilter]}
						{:else}
							Search by model, MAC, serial, or PPPoE user.
						{/if}
					</Card.Description>
				</div>
				{#if rxFilter || offlineDays !== null}
					<Button variant="outline" size="sm" onclick={clearFilter}>
						<XIcon data-icon="inline-start" />
						Clear filter
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
							{#if offlineDays !== null}
								No devices offline for more than {offlineDays} days
							{:else if rxFilter}
								No devices in this bucket
							{:else if search}
								No matching devices
							{:else}
								No devices yet
							{/if}
						</Empty.Title>
						<Empty.Description>
							{#if offlineDays !== null}
								No device has been silent for more than {offlineDays} days.
							{:else if rxFilter}
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
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Device ID</Table.Head>
							<Table.Head>Tags</Table.Head>
							<Table.Head>Model</Table.Head>
							<Table.Head>Serial</Table.Head>
							<Table.Head>PPPoE User</Table.Head>
							<Table.Head>Rx Power</Table.Head>
							<Table.Head>IP</Table.Head>
							<Table.Head>Status</Table.Head>
							<Table.Head>Last Inform</Table.Head>
							<Table.Head class="text-right">Action</Table.Head>
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
								<Table.Cell>
									{#if device.tags?.length}
										<div class="flex flex-wrap gap-1">
											{#each device.tags as tag (tag)}
												<Badge variant="outline" class="text-xs">{tag}</Badge>
											{/each}
										</div>
									{:else}
										<span class="text-muted-foreground">—</span>
									{/if}
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
								<Table.Cell class="text-right">
									<AlertDialog.Root>
										<AlertDialog.Trigger>
											{#snippet child({ props })}
												<Button
													{...props}
													variant="ghost"
													size="sm"
													class="text-destructive hover:text-destructive"
													disabled={deleting === device.device_id}
												>
													{#if deleting === device.device_id}
														<Spinner data-icon="inline-start" />
													{:else}
														<Trash2Icon data-icon="inline-start" />
													{/if}
													Delete
												</Button>
											{/snippet}
										</AlertDialog.Trigger>
										<AlertDialog.Content>
											<AlertDialog.Header>
												<AlertDialog.Title>Delete device?</AlertDialog.Title>
												<AlertDialog.Description>
													{device.device_id} will be removed from GenieACS. The device itself is
													not reset; it reappears here if it informs the ACS again.
												</AlertDialog.Description>
											</AlertDialog.Header>
											<AlertDialog.Footer>
												<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
												<AlertDialog.Action variant="destructive" onclick={() => removeDevice(device)}>
													Delete
												</AlertDialog.Action>
											</AlertDialog.Footer>
										</AlertDialog.Content>
									</AlertDialog.Root>
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			{/if}
		</Card.Content>
		<Card.Footer class="flex items-center justify-between gap-2">
			<p class="text-muted-foreground text-sm">
				Page {page} · {count}
				{hasMore ? '+' : ''} devices{search ? ` matching “${search}”` : ''}{offlineDays !== null
					? ` · offline > ${offlineDays} days`
					: rxFilter
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
