<script lang="ts">
	import { toast } from 'svelte-sonner';
	import {
		createWANConnection,
		deleteWANConnection,
		updateWANConnection,
		type CreateWANConnectionBody,
		type UpdateWANConnectionBody
	} from '$lib/api/device';
	import type { WANConnection } from '$lib/api/types';
	import * as Field from '$lib/components/ui/field';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { Separator } from '$lib/components/ui/separator';
	import { Spinner } from '$lib/components/ui/spinner';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SaveIcon from '@lucide/svelte/icons/save';

	let {
		ip,
		conn = null,
		existing = [],
		ondone
	}: {
		ip: string;
		/** Existing connection to edit, or null to create a new one. */
		conn?: WANConnection | null;
		/** Current connections, used to derive the parent object for a new instance. */
		existing?: WANConnection[];
		ondone: () => void;
	} = $props();

	// svelte-ignore state_referenced_locally
	const isCreate = conn === null;

	// svelte-ignore state_referenced_locally
	let type = $state(conn?.type ?? 'pppoe');
	// svelte-ignore state_referenced_locally
	let name = $state(conn?.name ?? '');
	// svelte-ignore state_referenced_locally
	let enabled = $state(conn?.enabled ?? true);
	// svelte-ignore state_referenced_locally
	let username = $state(conn?.username ?? '');
	let password = $state('');
	// svelte-ignore state_referenced_locally
	let addressingType = $state(conn?.type === 'static' ? 'static' : conn?.type === 'ipcp' ? 'ipcp' : 'dhcp');
	let externalIP = $state('');
	let subnetMask = $state('');
	let defaultGateway = $state('');
	let dnsServers = $state('');
	// Vendor extensions. Only rendered/sent when the CPE exposes them
	// (undefined = not exposed), so we never write a param the CPE lacks.
	// svelte-ignore state_referenced_locally
	let natEnabled = $state(conn?.nat_enabled ?? false);
	// svelte-ignore state_referenced_locally
	let vlanEnabled = $state(conn?.vlan_enabled ?? false);
	// svelte-ignore state_referenced_locally
	let vlanId = $state(conn?.vlan_id?.toString() ?? '');
	// Service list is a vendor token string (e.g. "INTERNET_TR069" or
	// "TR069,INTERNET"); parse it to tokens and edit as toggles.
	// svelte-ignore state_referenced_locally
	let services = $state(parseServices(conn?.service_list));
	let busy = $state(false);
	let deleting = $state(false);

	const isPPPoE = $derived(type === 'pppoe');
	const hasNat = $derived(!isCreate && conn?.nat_enabled !== undefined);
	const hasVlan = $derived(
		!isCreate && (conn?.vlan_enabled !== undefined || conn?.vlan_id !== undefined)
	);
	const hasServices = $derived(!isCreate && conn?.service_list !== undefined);
	const servicesSingle = $derived(conn?.service_list_single === true);

	// Common WAN service tags. Not exhaustive — unknown tags already on the
	// connection are preserved and sent back untouched.
	const SERVICE_OPTIONS = ['INTERNET', 'TR069', 'VoIP', 'IPTV', 'OTHER'];
	const serviceOn = (name: string) =>
		services.some((s) => s.toLowerCase() === name.toLowerCase());
	function toggleService(name: string, on: boolean) {
		const idx = services.findIndex((s) => s.toLowerCase() === name.toLowerCase());
		if (on && idx < 0) services = [...services, name];
		if (!on && idx >= 0) services = services.filter((_, i) => i !== idx);
	}
	// Single-token CPVs (Huawei) get a select; unknown current values are
	// kept as an option so an edit can't silently drop them.
	const singleOptions = $derived.by(() => {
		const opts = [...SERVICE_OPTIONS];
		for (const s of services) {
			if (!opts.some((o) => o.toLowerCase() === s.toLowerCase())) opts.push(s);
		}
		return opts;
	});
	function setSingleService(name: string) {
		services = name ? [name] : [];
	}

	// Split a vendor service-list string into tokens. Handles both
	// separators (Huawei '_', others ',') and whitespace.
	function parseServices(raw?: string): string[] {
		if (raw == null || raw.trim() === '') return [];
		return raw
			.split(/[_,]/)
			.map((s) => s.trim())
			.filter(Boolean);
	}

	// GenieACS assigns the connection instance on AddObject; the parent
	// WANDevice/WANConnectionDevice is derived from what the CPE already
	// exposes (prefer a same-type connection's parent, else the first one).
	const parent = $derived.by(() => {
		const same = existing.filter((c) => (type === 'pppoe' ? c.type === 'pppoe' : c.type !== 'pppoe'));
		const ref = same[0] ?? existing[0];
		if (ref) return { wan: ref.wan_device, cdev: ref.connection_device };
		return { wan: 1, cdev: 1 };
	});

	function errMessage(e: unknown): string {
		return e instanceof Error ? e.message : 'Request failed.';
	}

	async function save() {
		if (!ip) return;
		busy = true;
		try {
			if (isCreate) {
				if (!name.trim()) {
					toast.error('A connection name is required.');
					return;
				}
				const body: CreateWANConnectionBody = {
					type: type as CreateWANConnectionBody['type'],
					wan_device: parent.wan,
					connection_device: parent.cdev,
					name: name.trim(),
					enabled
				};
				if (isPPPoE) {
					if (username) body.username = username.trim();
					if (password) body.password = password;
				} else {
					if (externalIP) body.external_ip_address = externalIP.trim();
					if (subnetMask) body.subnet_mask = subnetMask.trim();
					if (defaultGateway) body.default_gateway = defaultGateway.trim();
					if (dnsServers) body.dns_servers = dnsServers.trim();
				}
				const res = await createWANConnection(ip, body);
				toast.success(res.message);
			} else {
				const body: UpdateWANConnectionBody = { enabled };
				if (isPPPoE) {
					if (username !== (conn?.username ?? '')) body.username = username.trim();
					if (password) body.password = password;
				} else {
					if (addressingType) body.addressing_type = addressingType;
					if (externalIP) body.external_ip_address = externalIP.trim();
					if (subnetMask) body.subnet_mask = subnetMask.trim();
					if (defaultGateway) body.default_gateway = defaultGateway.trim();
					if (dnsServers) body.dns_servers = dnsServers.trim();
				}
				if (hasNat && natEnabled !== conn?.nat_enabled) body.nat_enabled = natEnabled;
				if (hasVlan) {
					// Only send changed values; a blank id is left untouched
					// (disable via the toggle maps to vendor VLAN 0). The
					// number input binds a number, so normalize before .trim().
					const idText = String(vlanId ?? '').trim();
					if (vlanEnabled !== conn?.vlan_enabled) body.vlan_enabled = vlanEnabled;
					if (idText !== '' && Number(idText) !== (conn?.vlan_id ?? 0)) {
						body.vlan_id = Number(idText);
					}
				}
				if (
					hasServices &&
					services.join(',') !== parseServices(conn?.service_list).join(',')
				) {
					body.service_list = services;
				}
				const res = await updateWANConnection(
					ip,
					conn!.type,
					conn!.wan_device,
					conn!.connection_device,
					conn!.instance,
					body
				);
				toast.success(res.message);
			}
			ondone();
		} catch (e) {
			toast.error(`WAN ${isCreate ? 'create' : 'update'} failed: ${errMessage(e)}`);
		} finally {
			busy = false;
		}
	}

	async function remove() {
		if (!ip || isCreate) return;
		deleting = true;
		try {
			const res = await deleteWANConnection(
				ip,
				conn!.type,
				conn!.wan_device,
				conn!.connection_device,
				conn!.instance
			);
			toast.success(res.message);
			ondone();
		} catch (e) {
			toast.error(`WAN delete failed: ${errMessage(e)}`);
		} finally {
			deleting = false;
		}
	}
</script>

<form
	class="flex flex-col gap-4"
	onsubmit={(e) => {
		e.preventDefault();
		save();
	}}
>
	<div class="grid gap-4 sm:grid-cols-2">
		{#if isCreate}
			<Field.Field>
				<Field.FieldLabel for="wan-type">Type</Field.FieldLabel>
				<Select.Root type="single" bind:value={type}>
					<Select.Trigger id="wan-type" class="w-full">
						<Select.Value placeholder="Type" />
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="pppoe">PPPoE</Select.Item>
						<Select.Item value="dhcp">DHCP</Select.Item>
						<Select.Item value="static">Static</Select.Item>
					</Select.Content>
				</Select.Root>
			</Field.Field>
		{/if}

		{#if isCreate}
			<Field.Field>
				<Field.FieldLabel for="wan-name">Name</Field.FieldLabel>
				<Input
					id="wan-name"
					bind:value={name}
					placeholder="e.g. 4_INTERNET_R_VID_1"
					disabled={busy}
				/>
			</Field.Field>
		{/if}

		<Field.Field>
			<Field.FieldLabel for="wan-enabled">Enabled</Field.FieldLabel>
			<div class="flex h-9 items-center">
				<Switch id="wan-enabled" bind:checked={enabled} aria-label="Enable connection" />
			</div>
		</Field.Field>

		{#if isCreate}
			<div class="sm:col-span-2">
				<p class="text-muted-foreground text-xs">
					The connection instance is assigned automatically by GenieACS on the CPE's WAN
					device/connection — no index needed.
				</p>
			</div>
		{:else}
			<div class="sm:col-span-2">
				<Field.FieldLabel>Instance</Field.FieldLabel>
				<p class="text-muted-foreground font-mono text-xs">
					{conn!.type} · WANDevice {conn!.wan_device} · WANConnectionDevice {conn!.connection_device}
					· instance {conn!.instance}
				</p>
			</div>
		{/if}

		{#if isPPPoE}
			<Field.Field>
				<Field.FieldLabel for="wan-user">Username</Field.FieldLabel>
				<Input
					id="wan-user"
					bind:value={username}
					placeholder={isCreate ? 'customer@isp' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
			<Field.Field>
				<Field.FieldLabel for="wan-pass">Password</Field.FieldLabel>
				<Input
					id="wan-pass"
					type="password"
					bind:value={password}
					placeholder={isCreate ? '••••••••' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
		{:else}
			{#if !isCreate}
				<Field.Field>
					<Field.FieldLabel for="wan-addressing">Addressing type</Field.FieldLabel>
					<Select.Root type="single" bind:value={addressingType}>
						<Select.Trigger id="wan-addressing" class="w-full">
							<Select.Value placeholder="Addressing" />
						</Select.Trigger>
						<Select.Content>
							<Select.Item value="dhcp">DHCP</Select.Item>
							<Select.Item value="static">Static</Select.Item>
							<Select.Item value="ipcp">IPCP</Select.Item>
						</Select.Content>
					</Select.Root>
				</Field.Field>
			{/if}
			<Field.Field>
				<Field.FieldLabel for="wan-extip">External IP address</Field.FieldLabel>
				<Input
					id="wan-extip"
					bind:value={externalIP}
					placeholder={isCreate ? '203.0.113.5' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
			<Field.Field>
				<Field.FieldLabel for="wan-mask">Subnet mask</Field.FieldLabel>
				<Input
					id="wan-mask"
					bind:value={subnetMask}
					placeholder={isCreate ? '255.255.255.0' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
			<Field.Field>
				<Field.FieldLabel for="wan-gw">Default gateway</Field.FieldLabel>
				<Input
					id="wan-gw"
					bind:value={defaultGateway}
					placeholder={isCreate ? '203.0.113.1' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
			<Field.Field>
				<Field.FieldLabel for="wan-dns">DNS servers</Field.FieldLabel>
				<Input
					id="wan-dns"
					bind:value={dnsServers}
					placeholder={isCreate ? '1.1.1.1,8.8.8.8' : 'Leave blank to keep current'}
					disabled={busy}
				/>
			</Field.Field>
		{/if}

		{#if hasNat || hasVlan || hasServices}
			<div class="sm:col-span-2">
				<Separator />
			</div>
			{#if hasNat}
				<Field.Field>
					<Field.FieldLabel for="wan-nat">NAT</Field.FieldLabel>
					<div class="flex h-9 items-center">
						<Switch id="wan-nat" bind:checked={natEnabled} aria-label="Enable NAT" disabled={busy} />
					</div>
				</Field.Field>
			{/if}
			{#if hasVlan}
				<Field.Field>
					<Field.FieldLabel for="wan-vlan">VLAN tagging</Field.FieldLabel>
					<div class="flex h-9 items-center">
						<Switch
							id="wan-vlan"
							bind:checked={vlanEnabled}
							aria-label="Enable VLAN tagging"
							disabled={busy}
						/>
					</div>
				</Field.Field>
				<Field.Field>
					<Field.FieldLabel for="wan-vlan-id">VLAN ID</Field.FieldLabel>
					<Input
						id="wan-vlan-id"
						type="number"
						min="1"
						max="4094"
						bind:value={vlanId}
						placeholder="e.g. 90"
						disabled={busy || !vlanEnabled}
					/>
				</Field.Field>
			{/if}
			{#if hasServices}
				<div class="sm:col-span-2">
					<Field.FieldLabel for="wan-svc">Service list</Field.FieldLabel>
					{#if servicesSingle}
						<p class="text-muted-foreground mt-1 mb-2 text-xs">
							This CPE accepts a single service tag.
						</p>
						<Select.Root
							type="single"
							value={services[0] ?? ''}
							onValueChange={(v) => setSingleService(v)}
						>
							<Select.Trigger id="wan-svc" class="w-full">
								<Select.Value placeholder="Select a service" />
							</Select.Trigger>
							<Select.Content>
								{#each singleOptions as svc (svc)}
									<Select.Item value={svc}>{svc}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					{:else}
						<div class="mt-2 flex flex-wrap gap-4">
							{#each SERVICE_OPTIONS as svc (svc)}
								<div class="flex items-center gap-2">
									<Switch
										id={`wan-svc-${svc}`}
										checked={serviceOn(svc)}
										onCheckedChange={(v) => toggleService(svc, v)}
										disabled={busy}
										aria-label={`Service ${svc}`}
									/>
									<label for={`wan-svc-${svc}`} class="text-sm">{svc}</label>
								</div>
							{/each}
						</div>
						{#if services.some((s) => !SERVICE_OPTIONS.some((o) => o.toLowerCase() === s.toLowerCase()))}
							<p class="text-muted-foreground mt-2 text-xs">
								Preserved: {services
									.filter(
										(s) => !SERVICE_OPTIONS.some((o) => o.toLowerCase() === s.toLowerCase())
									)
									.join(', ')}
							</p>
						{/if}
					{/if}
				</div>
			{/if}
		{/if}
	</div>

	<div class="flex flex-wrap items-center gap-2">
		<Button type="submit" disabled={busy || deleting || !ip}>
			{#if busy}
				<Spinner data-icon="inline-start" />
			{:else if isCreate}
				<PlusIcon data-icon="inline-start" />
			{:else}
				<SaveIcon data-icon="inline-start" />
			{/if}
			{isCreate ? 'Add WAN connection' : 'Save changes'}
		</Button>
		{#if !isCreate}
			<Button
				type="button"
				variant="destructive"
				disabled={busy || deleting || !ip}
				onclick={remove}
			>
				{#if deleting}
					<Spinner data-icon="inline-start" />
				{/if}
				Delete connection
			</Button>
		{/if}
	</div>
</form>
