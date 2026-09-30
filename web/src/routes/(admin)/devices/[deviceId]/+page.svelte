<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { listDevices } from '$lib/api/devices';
	import {
		deviceCapability,
		deviceStatus,
		dhcpClients,
		factoryResetDevice,
		opticalStats,
		rebootDevice,
		setPPPoECredentials,
		wakeDevice,
		wanStatus,
		wifiClients,
		wifiConnectivityRefresh,
		wifiStats,
		wlanConfigs,
		updateWLAN
	} from '$lib/api/device';
	import type {
		DeviceCapability,
		DeviceStatus,
		DeviceSummary,
		DHCPClient,
		OpticalStats,
		WANConnection,
		WiFiClient,
		WiFiStatsRadio,
		WLANConfig
	} from '$lib/api/types';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Empty from '$lib/components/ui/empty';
	import * as Field from '$lib/components/ui/field';
	import * as Select from '$lib/components/ui/select';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Spinner } from '$lib/components/ui/spinner';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import RotateCwIcon from '@lucide/svelte/icons/rotate-cw';
	import PowerIcon from '@lucide/svelte/icons/power';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import ThermometerIcon from '@lucide/svelte/icons/thermometer';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import InboxIcon from '@lucide/svelte/icons/inbox';

	// SvelteKit already URL-decodes the route param.
	const deviceId = $derived(page.params.deviceId ?? '');

	// Devices inform every ~200s; allow a grace window before calling one offline.
	const ONLINE_WINDOW_MS = 15 * 60 * 1000;

	let summary = $state<DeviceSummary | null>(null);
	let summaryError = $state('');
	let summaryLoading = $state(true);

	let status = $state<DeviceStatus | null>(null);
	let statusErr = $state('');
	let wan = $state<WANConnection[] | null>(null);
	let wanErr = $state('');
	let capability = $state<DeviceCapability | null>(null);
	let wlans = $state<WLANConfig[] | null>(null);
	let wlansErr = $state('');
	let optical = $state<OpticalStats | null>(null);
	let opticalErr = $state('');
	let opticalUnsupported = $state(false);
	let opticalLoading = $state(false);
	let radioStats = $state<WiFiStatsRadio[] | null>(null);
	let clients = $state<WiFiClient[] | null>(null);
	let clientsErr = $state('');
	let leases = $state<DHCPClient[] | null>(null);
	let leasesErr = $state('');
	let refreshing = $state(false);

	let activeTab = $state('overview');
	let pppoeUser = $state('');
	let pppoePass = $state('');
	let pppoeBusy = $state(false);
	let editWlan = $state('1');
	let editSSID = $state('');
	let editPass = $state('');
	let editBusy = $state(false);

	const ip = $derived(summary?.ip ?? '');
	const online = $derived(summary ? isOnline(summary) : (status?.online ?? false));

	function isOnline(d: DeviceSummary): boolean {
		if (!d.last_inform) return false;
		const t = new Date(d.last_inform).getTime();
		return !Number.isNaN(t) && Date.now() - t < ONLINE_WINDOW_MS;
	}

	function errMessage(e: unknown): string {
		return e instanceof Error ? e.message : 'Request failed.';
	}

	function formatTime(iso?: string): string {
		if (!iso) return '—';
		const date = new Date(iso);
		return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
	}

	function formatUptime(seconds?: number): string {
		if (!seconds || seconds < 0) return '—';
		const d = Math.floor(seconds / 86400);
		const h = Math.floor((seconds % 86400) / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		return [d && `${d}d`, h && `${h}h`, `${m}m`].filter(Boolean).join(' ');
	}

	function formatDBm(v?: number): string {
		return typeof v === 'number' && v !== 0 ? `${v.toFixed(2)} dBm` : '—';
	}

	function healthClass(health: string): string {
		switch (health) {
			case 'good':
				return 'border-success/50 text-success';
			case 'warning':
				return 'border-primary/50 text-primary';
			case 'critical':
			case 'no_signal':
				return 'border-destructive/50 text-destructive';
			default:
				return 'text-muted-foreground';
		}
	}

	async function safe<T>(fn: () => Promise<T>, apply: (v: T) => void, setErr: (e: string) => void) {
		try {
			apply(await fn());
		} catch (e) {
			setErr(errMessage(e));
		}
	}

	async function loadSummary() {
		summaryLoading = true;
		try {
			const res = await listDevices(1, 1, deviceId);
			if (res.devices.length === 0) {
				summaryError = `No device found with ID “${deviceId}”.`;
			} else {
				summary = res.devices[0];
			}
		} catch (e) {
			summaryError = errMessage(e);
		} finally {
			summaryLoading = false;
		}
	}

	async function loadOverview(target: string) {
		status = null;
		statusErr = '';
		wan = null;
		wanErr = '';
		capability = null;
		wlans = null;
		wlansErr = '';
		await Promise.all([
			safe(() => deviceStatus(target), (v) => (status = v), (e) => (statusErr = e)),
			safe(() => wanStatus(target), (v) => (wan = v.wan_connections), (e) => (wanErr = e)),
			safe(() => deviceCapability(target), (v) => (capability = v), () => {}),
			safe(() => wlanConfigs(target), (v) => (wlans = v), (e) => (wlansErr = e))
		]);
	}

	async function loadOptical(target: string, refresh = false) {
		opticalLoading = true;
		opticalErr = '';
		opticalUnsupported = false;
		try {
			optical = await opticalStats(target, refresh);
		} catch (e) {
			optical = null;
			// The backend 404s with OPTICAL_NOT_SUPPORTED for CPEs without optics.
			if (e && typeof e === 'object' && 'errorCode' in e && e.errorCode === 'OPTICAL_NOT_SUPPORTED') {
				opticalUnsupported = true;
			} else {
				opticalErr = errMessage(e);
			}
		} finally {
			opticalLoading = false;
		}
	}

	async function loadWifi(target: string) {
		radioStats = null;
		clients = null;
		clientsErr = '';
		leases = null;
		leasesErr = '';
		await Promise.all([
			safe(() => wifiStats(target), (v) => (radioStats = v.radios), () => {}),
			safe(() => wifiClients(target), (v) => (clients = v.clients), (e) => (clientsErr = e)),
			safe(() => dhcpClients(target), (v) => (leases = v), (e) => (leasesErr = e))
		]);
	}

	async function loadDetails(target: string) {
		await Promise.all([loadOverview(target), loadOptical(target), loadWifi(target)]);
	}

	async function refreshAll() {
		if (!ip) return;
		refreshing = true;
		await loadSummary();
		if (ip) await loadDetails(ip);
		refreshing = false;
	}

	onMount(async () => {
		await loadSummary();
		if (ip) {
			pppoeUser = summary?.pppoe_username ?? '';
			await loadDetails(ip);
			editSSID = wlans?.[0]?.ssid ?? '';
		}
	});

	$effect(() => {
		const w = wlans?.find((x) => x.wlan === editWlan);
		if (w) editSSID = w.ssid;
	});

	async function refreshDHCP() {
		if (!ip) return;
		await runAction('DHCP refresh', async () => {
			await dhcpClients(ip, true);
			leases = await dhcpClients(ip);
		});
	}

	async function refreshWLANConfig() {
		if (!ip) return;
		await runAction('WLAN refresh', async () => {
			await wifiConnectivityRefresh(ip);
			wlans = await wlanConfigs(ip);
		});
	}

	async function runAction(label: string, fn: () => Promise<unknown>) {
		try {
			await fn();
			toast.success(`${label} submitted.`);
		} catch (e) {
			toast.error(`${label} failed: ${errMessage(e)}`);
		}
	}

	async function submitPPPoE() {
		if (!ip) return;
		pppoeBusy = true;
		try {
			await setPPPoECredentials(ip, pppoeUser.trim(), pppoePass);
			toast.success('PPPoE credentials submitted. The device reconnects within ~30s.');
			pppoePass = '';
		} catch (e) {
			toast.error(`PPPoE update failed: ${errMessage(e)}`);
		} finally {
			pppoeBusy = false;
		}
	}

	async function saveWLAN() {
		if (!ip) return;
		editBusy = true;
		try {
			const body: Record<string, unknown> = { ssid: editSSID };
			if (editPass) body.password = editPass;
			await updateWLAN(ip, editWlan, body);
			toast.success(`WLAN ${editWlan} update submitted.`);
			editPass = '';
		} catch (e) {
			toast.error(`WLAN update failed: ${errMessage(e)}`);
		} finally {
			editBusy = false;
		}
	}
</script>

<svelte:head><title>{deviceId} · GenieACS Relay</title></svelte:head>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div class="flex min-w-0 items-center gap-2">
			<Button variant="ghost" size="icon-sm" aria-label="Back to devices" onclick={() => goto('/')}>
				<ArrowLeftIcon />
			</Button>
			<div class="min-w-0">
				<h2 class="truncate font-mono text-sm font-semibold">{deviceId}</h2>
				<p class="text-muted-foreground text-sm">
					{summary ? `${summary.model ?? 'Unknown'} · ${summary.serial ?? ''}` : 'Device detail'}
				</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<Badge variant="outline" class={online ? 'border-success/50 text-success' : 'border-destructive/50 text-destructive'}>
				{online ? 'online' : 'offline'}
			</Badge>
			{#if summary?.ip}
				<Badge variant="outline" href={`http://${summary.ip}`} target="_blank" rel="noopener noreferrer">
					{summary.ip}
				</Badge>
			{/if}
			<Button variant="outline" size="sm" onclick={refreshAll} disabled={refreshing || summaryLoading}>
				{#if refreshing}
					<Spinner data-icon="inline-start" />
				{:else}
					<RefreshCwIcon data-icon="inline-start" />
				{/if}
				Refresh
			</Button>
		</div>
	</div>

	{#if summaryLoading}
		<div class="flex flex-col gap-2">
			{#each Array(4) as _, i (i)}
				<Skeleton class="h-10 w-full" />
			{/each}
		</div>
	{:else if summaryError}
		<Alert variant="destructive">
			<TriangleAlertIcon />
			<AlertTitle>Device unavailable</AlertTitle>
			<AlertDescription>{summaryError}</AlertDescription>
		</Alert>
	{:else if summary}
		<Tabs.Root bind:value={activeTab}>
			<Tabs.List>
				<Tabs.Trigger value="overview">Overview</Tabs.Trigger>
				<Tabs.Trigger value="optical">Optical</Tabs.Trigger>
				<Tabs.Trigger value="wifi">WiFi &amp; LAN</Tabs.Trigger>
				<Tabs.Trigger value="actions">Actions</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="overview" class="flex flex-col gap-4">
				<div class="grid gap-4 lg:grid-cols-2">
					<Card.Root>
						<Card.Header>
							<Card.Title>Identification</Card.Title>
						</Card.Header>
						<Card.Content class="grid grid-cols-2 gap-3 text-sm">
							<div>
								<p class="text-muted-foreground text-xs">Manufacturer</p>
								<p>{status?.manufacturer ?? summary.manufacturer ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Model</p>
								<p>{summary.model ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Serial</p>
								<p class="font-mono text-xs">{summary.serial ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">MAC</p>
								<p class="font-mono text-xs">{summary.mac ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Software</p>
								<p>{status?.software_version ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Hardware</p>
								<p>{status?.hardware_version ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Band</p>
								<p>{capability?.band_type ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Uptime</p>
								<p>{formatUptime(status?.uptime_seconds)}</p>
							</div>
						</Card.Content>
					</Card.Root>

					<Card.Root>
						<Card.Header>
							<Card.Title>Connectivity</Card.Title>
						</Card.Header>
						<Card.Content class="grid grid-cols-2 gap-3 text-sm">
							<div>
								<p class="text-muted-foreground text-xs">WAN IP</p>
								<p class="font-mono text-xs">{summary.ip ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">PPPoE user</p>
								<p class="font-mono text-xs">{summary.pppoe_username ?? '—'}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Rx power</p>
								<p>{formatDBm(summary.rx_power_dbm)}</p>
							</div>
							<div>
								<p class="text-muted-foreground text-xs">Last inform</p>
								<p>{formatTime(summary.last_inform)}</p>
							</div>
						</Card.Content>
					</Card.Root>
				</div>

				<Card.Root>
					<Card.Header>
						<Card.Title>WAN connections</Card.Title>
						<Card.Description>Every PPP and IP connection instance on the CPE.</Card.Description>
					</Card.Header>
					<Card.Content>
						{#if wanErr}
							<Alert variant="destructive">
								<TriangleAlertIcon />
								<AlertTitle>Failed to read WAN status</AlertTitle>
								<AlertDescription>{wanErr}</AlertDescription>
							</Alert>
						{:else if wan === null}
							<Skeleton class="h-16 w-full" />
						{:else if wan.length === 0}
							{@render EmptyState({ title: 'No WAN connections', description: 'The CPE reports no WAN connection instances.' })}
						{:else}
							<div class="overflow-x-auto">
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>Instance</Table.Head>
											<Table.Head>Type</Table.Head>
											<Table.Head>Status</Table.Head>
											<Table.Head>External IP</Table.Head>
											<Table.Head>Uptime</Table.Head>
											<Table.Head>Username</Table.Head>
											<Table.Head>Last error</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each wan as conn, i (i)}
											<Table.Row>
												<Table.Cell>{conn.instance}</Table.Cell>
												<Table.Cell>{conn.type}</Table.Cell>
												<Table.Cell>
													<Badge
														variant="outline"
														class={conn.connection_status === 'Connected'
															? 'border-success/50 text-success'
															: 'text-muted-foreground'}>{conn.connection_status ?? 'unknown'}</Badge
													>
												</Table.Cell>
												<Table.Cell class="font-mono text-xs">{conn.external_ip || '—'}</Table.Cell>
												<Table.Cell>{formatUptime(conn.uptime_seconds)}</Table.Cell>
												<Table.Cell class="font-mono text-xs">{conn.username || '—'}</Table.Cell>
												<Table.Cell class="text-muted-foreground text-xs">
													{conn.last_connection_error && conn.last_connection_error !== 'ERROR_NONE'
														? conn.last_connection_error
														: '—'}
												</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							</div>
						{/if}
					</Card.Content>
				</Card.Root>
			</Tabs.Content>

			<Tabs.Content value="optical" class="flex flex-col gap-4">
				<Card.Root>
					<Card.Header>
						<div class="flex items-center justify-between gap-2">
							<div>
								<Card.Title>Optical interface</Card.Title>
								<Card.Description>Tx/Rx power, bias current, temperature, and voltage.</Card.Description>
							</div>
							<Button
								variant="outline"
								size="sm"
								onclick={() => ip && loadOptical(ip, true)}
								disabled={opticalLoading}
							>
								{#if opticalLoading}
									<Spinner data-icon="inline-start" />
								{:else}
									<RotateCwIcon data-icon="inline-start" />
								{/if}
								Refresh from CPE
							</Button>
						</div>
					</Card.Header>
					<Card.Content>
						{#if opticalLoading && !optical}
							<Skeleton class="h-24 w-full" />
						{:else if opticalUnsupported}
							{@render EmptyState({
								title: 'Optical not supported',
								description: 'This CPE exposes no known optical parameter tree.'
							})}
						{:else if opticalErr}
							<Alert variant="destructive">
								<TriangleAlertIcon />
								<AlertTitle>Failed to read optical stats</AlertTitle>
								<AlertDescription>{opticalErr}</AlertDescription>
							</Alert>
						{:else if optical}
							<div class="mb-4 flex flex-wrap items-center gap-2">
								<Badge variant="outline" class={healthClass(optical.health)}>{optical.health}</Badge>
								<Badge variant="outline" class="text-muted-foreground">source: {optical.source}</Badge>
								<span class="text-muted-foreground text-xs">read {formatTime(optical.fetched_at)}</span>
							</div>
							<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
								{@render Metric({ label: 'Rx power', value: formatDBm(optical.rx_power_dbm) })}
								{@render Metric({ label: 'Tx power', value: formatDBm(optical.tx_power_dbm) })}
								{@render Metric({ label: 'Bias current', value: optical.bias_current_ma ? `${optical.bias_current_ma} mA` : '—' })}
								{@render Metric({ label: 'Temperature', value: optical.temperature_c ? `${optical.temperature_c} °C` : '—' })}
								{@render Metric({ label: 'Voltage', value: optical.voltage_v ? `${optical.voltage_v} V` : '—' })}
							</div>
						{/if}
					</Card.Content>
				</Card.Root>

				<Card.Root>
					<Card.Header>
						<Card.Title>Optical readings</Card.Title>
					</Card.Header>
					<Card.Content class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
						{@render Reading({ icon: ZapIcon, label: 'Tx power', value: formatDBm(optical?.tx_power_dbm) })}
						{@render Reading({ icon: ZapIcon, label: 'Rx power', value: formatDBm(optical?.rx_power_dbm) })}
						{@render Reading({ icon: ActivityIcon, label: 'Signal health', value: optical?.health ?? '—' })}
						{@render Reading({ icon: ThermometerIcon, label: 'Temperature', value: optical?.temperature_c ? `${optical.temperature_c} °C` : '—' })}
					</Card.Content>
				</Card.Root>
			</Tabs.Content>

			<Tabs.Content value="wifi" class="flex flex-col gap-4">
				<Card.Root>
					<Card.Header>
						<Card.Title>WLAN slots</Card.Title>
						<Card.Description>SSID configuration currently broadcasting on the CPE.</Card.Description>
					</Card.Header>
					<Card.Content>
						{#if wlansErr}
							<Alert variant="destructive">
								<TriangleAlertIcon />
								<AlertTitle>Failed to read WLAN config</AlertTitle>
								<AlertDescription>{wlansErr}</AlertDescription>
							</Alert>
						{:else if wlans === null}
							<Skeleton class="h-16 w-full" />
						{:else if wlans.length === 0}
							{@render EmptyState({ title: 'No WLAN slots', description: 'The CPE reports no active WLAN configuration.' })}
						{:else}
							<div class="overflow-x-auto">
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>WLAN</Table.Head>
											<Table.Head>SSID</Table.Head>
											<Table.Head>Band</Table.Head>
											<Table.Head>Auth</Table.Head>
											<Table.Head>Encryption</Table.Head>
											<Table.Head>Hidden</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each wlans as w (w.wlan)}
											<Table.Row>
												<Table.Cell>{w.wlan}</Table.Cell>
												<Table.Cell>{w.ssid}</Table.Cell>
												<Table.Cell>{w.band}</Table.Cell>
												<Table.Cell>{w.auth_mode ?? '—'}</Table.Cell>
												<Table.Cell>{w.encryption ?? '—'}</Table.Cell>
												<Table.Cell>{w.hidden ? 'yes' : 'no'}</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							</div>
						{/if}
					</Card.Content>
				</Card.Root>

				<div class="grid gap-4 lg:grid-cols-2">
					<Card.Root>
						<Card.Header>
							<Card.Title>Radio statistics</Card.Title>
						</Card.Header>
						<Card.Content>
							{#if radioStats === null}
								<Skeleton class="h-16 w-full" />
							{:else if radioStats.length === 0}
								{@render EmptyState({ title: 'No radios', description: 'No radio statistics reported.' })}
							{:else}
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>WLAN</Table.Head>
											<Table.Head>SSID</Table.Head>
											<Table.Head>Channel</Table.Head>
											<Table.Head>Tx power</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each radioStats as r (r.wlan)}
											<Table.Row>
												<Table.Cell>{r.wlan}</Table.Cell>
												<Table.Cell>{r.ssid ?? '—'}</Table.Cell>
												<Table.Cell>{r.channel ?? '—'}</Table.Cell>
												<Table.Cell>{r.tx_power_percent != null ? `${r.tx_power_percent}%` : '—'}</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							{/if}
						</Card.Content>
					</Card.Root>

					<Card.Root>
						<Card.Header>
							<Card.Title>Wireless clients</Card.Title>
						</Card.Header>
						<Card.Content>
							{#if clientsErr}
								<Alert variant="destructive">
									<TriangleAlertIcon />
									<AlertTitle>Failed to read wireless clients</AlertTitle>
									<AlertDescription>{clientsErr}</AlertDescription>
								</Alert>
							{:else if clients === null}
								<Skeleton class="h-16 w-full" />
							{:else if clients.length === 0}
								{@render EmptyState({ title: 'No wireless clients', description: 'No devices are associated to the wireless network.' })}
							{:else}
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>MAC</Table.Head>
											<Table.Head>SSID</Table.Head>
											<Table.Head>Band</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each clients as c (c.mac + c.wlan)}
											<Table.Row>
												<Table.Cell class="font-mono text-xs">{c.mac}</Table.Cell>
												<Table.Cell>{c.ssid ?? '—'}</Table.Cell>
												<Table.Cell>{c.band ?? '—'}</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							{/if}
						</Card.Content>
					</Card.Root>
				</div>

				<Card.Root>
					<Card.Header>
						<div class="flex items-center justify-between gap-2">
							<div>
								<Card.Title>DHCP leases</Card.Title>
								<Card.Description>Clients that obtained an address from the CPE.</Card.Description>
							</div>
							<Button
								variant="outline"
								size="sm"
								onclick={refreshDHCP}
							>
								<RotateCwIcon data-icon="inline-start" />
								Refresh from CPE
							</Button>
						</div>
					</Card.Header>
					<Card.Content>
						{#if leasesErr}
							<Alert variant="destructive">
								<TriangleAlertIcon />
								<AlertTitle>Failed to read DHCP leases</AlertTitle>
								<AlertDescription>{leasesErr}</AlertDescription>
							</Alert>
						{:else if leases === null}
							<Skeleton class="h-16 w-full" />
						{:else if leases.length === 0}
							{@render EmptyState({ title: 'No DHCP leases', description: 'The CPE reports no DHCP clients.' })}
						{:else}
							<div class="overflow-x-auto">
								<Table.Root>
									<Table.Header>
										<Table.Row>
											<Table.Head>MAC</Table.Head>
											<Table.Head>Hostname</Table.Head>
											<Table.Head>IP</Table.Head>
										</Table.Row>
									</Table.Header>
									<Table.Body>
										{#each leases as l (l.mac + l.ip)}
											<Table.Row>
												<Table.Cell class="font-mono text-xs">{l.mac}</Table.Cell>
												<Table.Cell>{l.hostname || '—'}</Table.Cell>
												<Table.Cell class="font-mono text-xs">{l.ip}</Table.Cell>
											</Table.Row>
										{/each}
									</Table.Body>
								</Table.Root>
							</div>
						{/if}
					</Card.Content>
				</Card.Root>
			</Tabs.Content>

			<Tabs.Content value="actions" class="flex flex-col gap-4">
				<div class="grid gap-4 lg:grid-cols-2">
					<Card.Root>
						<Card.Header>
							<Card.Title class="text-destructive">Danger zone</Card.Title>
							<Card.Description>Lifecycle tasks submitted to the CPE over TR-069.</Card.Description>
						</Card.Header>
						<Card.Content class="flex flex-col gap-3">
							<div class="flex items-center justify-between gap-2">
								<div>
									<p class="text-sm font-medium">Wake</p>
									<p class="text-muted-foreground text-xs">Fire a ConnectionRequest without queuing work.</p>
								</div>
								<Button variant="outline" size="sm" onclick={() => ip && runAction('Wake', () => wakeDevice(ip))}>
									<ZapIcon data-icon="inline-start" />
									Wake
								</Button>
							</div>
							<Separator />
							<div class="flex items-center justify-between gap-2">
								<div>
									<p class="text-sm font-medium">Reboot</p>
									<p class="text-muted-foreground text-xs">The CPE drops offline for 30–90s.</p>
								</div>
								{@render ConfirmAction({ label: 'Reboot', action: 'Reboot', onconfirm: () => ip && runAction('Reboot', () => rebootDevice(ip)) })}
							</div>
							<Separator />
							<div class="flex items-center justify-between gap-2">
								<div>
									<p class="text-sm font-medium">Factory reset</p>
									<p class="text-muted-foreground text-xs">Erases all CPE configuration — irreversible.</p>
								</div>
								{@render ConfirmAction({
									label: 'Factory reset',
									action: 'Factory reset',
									destructive: true,
									onconfirm: () => ip && runAction('Factory reset', () => factoryResetDevice(ip))
								})}
							</div>
						</Card.Content>
					</Card.Root>

					<Card.Root>
						<Card.Header>
							<Card.Title>PPPoE credentials</Card.Title>
							<Card.Description>Update the WAN PPPoE username and password.</Card.Description>
						</Card.Header>
						<Card.Content>
							<form
								class="flex flex-col gap-4"
								onsubmit={(e) => {
									e.preventDefault();
									submitPPPoE();
								}}
							>
								<Field.FieldGroup>
									<Field.Field>
										<Field.FieldLabel for="pppoe-user">Username</Field.FieldLabel>
										<Input id="pppoe-user" bind:value={pppoeUser} placeholder="customer@isp" disabled={pppoeBusy} />
									</Field.Field>
									<Field.Field>
										<Field.FieldLabel for="pppoe-pass">Password</Field.FieldLabel>
										<Input id="pppoe-pass" type="password" bind:value={pppoePass} placeholder="••••••••" disabled={pppoeBusy} />
									</Field.Field>
								</Field.FieldGroup>
								<Button type="submit" disabled={pppoeBusy || !pppoeUser.trim() || !pppoePass}>
									{#if pppoeBusy}
										<Spinner data-icon="inline-start" />
									{:else}
										<KeyRoundIcon data-icon="inline-start" />
									{/if}
									Update PPPoE
								</Button>
							</form>
						</Card.Content>
					</Card.Root>

					<Card.Root>
						<Card.Header>
							<Card.Title>WiFi settings</Card.Title>
							<Card.Description>Change the SSID or password of a WLAN slot.</Card.Description>
						</Card.Header>
						<Card.Content>
							<form
								class="flex flex-col gap-4"
								onsubmit={(e) => {
									e.preventDefault();
									saveWLAN();
								}}
							>
								<Field.FieldGroup>
									<Field.Field>
										<Field.FieldLabel for="wlan-slot">WLAN slot</Field.FieldLabel>
										<Select.Root type="single" bind:value={editWlan}>
											<Select.Trigger id="wlan-slot" class="w-full">
												<Select.Value placeholder="Select a WLAN" />
											</Select.Trigger>
											<Select.Content>
												{#each wlans ?? [] as w (w.wlan)}
													<Select.Item value={w.wlan}>{w.wlan} · {w.ssid} ({w.band})</Select.Item>
												{/each}
											</Select.Content>
										</Select.Root>
									</Field.Field>
									<Field.Field>
										<Field.FieldLabel for="wlan-ssid">SSID</Field.FieldLabel>
										<Input id="wlan-ssid" bind:value={editSSID} disabled={editBusy} />
									</Field.Field>
									<Field.Field>
										<Field.FieldLabel for="wlan-pass">Password</Field.FieldLabel>
										<Input
											id="wlan-pass"
											type="password"
											bind:value={editPass}
											placeholder="Leave blank to keep current"
											disabled={editBusy}
										/>
									</Field.Field>
								</Field.FieldGroup>
								<Button type="submit" disabled={editBusy || !editSSID.trim()}>
									{#if editBusy}
										<Spinner data-icon="inline-start" />
									{:else}
										<WifiIcon data-icon="inline-start" />
									{/if}
									Save WiFi settings
								</Button>
							</form>
						</Card.Content>
					</Card.Root>

					<Card.Root>
						<Card.Header>
							<Card.Title>Data refresh</Card.Title>
							<Card.Description>Ask GenieACS to re-read a subtree from the CPE.</Card.Description>
						</Card.Header>
						<Card.Content class="flex flex-col gap-3">
							<Button variant="outline" size="sm" onclick={refreshWLANConfig}>
								<RotateCwIcon data-icon="inline-start" />
								Refresh WLAN config
							</Button>
							<Button variant="outline" size="sm" onclick={refreshDHCP}>
								<RotateCwIcon data-icon="inline-start" />
								Refresh DHCP clients
							</Button>
							<Button variant="outline" size="sm" onclick={() => ip && loadOptical(ip, true)}>
								<RotateCwIcon data-icon="inline-start" />
								Refresh optical stats
							</Button>
						</Card.Content>
					</Card.Root>
				</div>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

{#snippet EmptyState(props: { title: string; description: string })}
	<Empty.Root>
		<Empty.Header>
			<Empty.Media variant="icon">
				<InboxIcon />
			</Empty.Media>
			<Empty.Title>{props.title}</Empty.Title>
			<Empty.Description>{props.description}</Empty.Description>
		</Empty.Header>
	</Empty.Root>
{/snippet}

{#snippet Metric(props: { label: string; value: string })}
	<div>
		<p class="text-muted-foreground text-xs">{props.label}</p>
		<p class="font-medium">{props.value}</p>
	</div>
{/snippet}

{#snippet Reading(props: { icon: typeof ZapIcon; label: string; value: string })}
	<div class="flex items-center gap-3">
		<div class="bg-muted text-muted-foreground flex size-9 items-center justify-center rounded-lg">
			<props.icon class="size-4" />
		</div>
		<div>
			<p class="text-muted-foreground text-xs">{props.label}</p>
			<p class="text-sm font-medium">{props.value}</p>
		</div>
	</div>
{/snippet}

{#snippet ConfirmAction(props: { label: string; action: string; destructive?: boolean; onconfirm: () => void })}
	<AlertDialog.Root>
		<AlertDialog.Trigger>
			{#snippet child({ props: triggerProps })}
				<Button variant={props.destructive ? 'destructive' : 'outline'} size="sm" {...triggerProps}>
					<PowerIcon data-icon="inline-start" />
					{props.label}
				</Button>
			{/snippet}
		</AlertDialog.Trigger>
		<AlertDialog.Content>
			<AlertDialog.Header>
				<AlertDialog.Title>{props.label} this device?</AlertDialog.Title>
				<AlertDialog.Description>
					{props.action} is submitted to the CPE over TR-069. Make sure this is the intended device.
				</AlertDialog.Description>
			</AlertDialog.Header>
			<AlertDialog.Footer>
				<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
				<AlertDialog.Action onclick={props.onconfirm}>Yes, {props.label.toLowerCase()}</AlertDialog.Action>
			</AlertDialog.Footer>
		</AlertDialog.Content>
	</AlertDialog.Root>
{/snippet}
