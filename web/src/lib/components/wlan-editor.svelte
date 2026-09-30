<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { updateWLAN, updateWLANRadio } from '$lib/api/device';
	import type { WLANConfig } from '$lib/api/types';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { Spinner } from '$lib/components/ui/spinner';
	import SaveIcon from '@lucide/svelte/icons/save';

	let {
		ip,
		wlan,
		currentChannel
	}: {
		ip: string;
		wlan: WLANConfig;
		currentChannel?: number;
	} = $props();

	const AUTH_MODES = ['Open', 'WPA', 'WPA2', 'WPA/WPA2'];
	const ENCRYPTIONS = ['AES', 'TKIP', 'TKIP+AES'];
	/** Sentinel: leave the radio setting untouched on save. */
	const UNCHANGED = 'unchanged';

	const is5GHz = $derived(wlan.band.toLowerCase().includes('5'));
	const channels = $derived(
		is5GHz
			? ['36', '40', '44', '48', '52', '56', '60', '64', '149', '153', '157', '161']
			: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13']
	);
	const bandwidths = $derived(is5GHz ? ['20MHz', '40MHz', '80MHz'] : ['20MHz', '40MHz']);

	// Drafts seeded from props; kept in sync by the $effect below.
	// svelte-ignore state_referenced_locally
	let ssid = $state(wlan.ssid);
	// svelte-ignore state_referenced_locally
	let authMode = $state(wlan.auth_mode || 'WPA2');
	// svelte-ignore state_referenced_locally
	let encryption = $state(wlan.encryption || 'AES');
	// svelte-ignore state_referenced_locally
	let hidden = $state(wlan.hidden);
	let hiddenBusy = $state(false);
	let password = $state('');
	let channel = $state(UNCHANGED);
	let bandwidth = $state(UNCHANGED);
	let busy = $state(false);

	// Re-sync drafts when the parent replaces the config (e.g. page refresh).
	// Toggling Hide SSID changes `hidden`, not `wlan`, so this never fights the
	// operator's click; a real refresh passes a new `wlan` and re-syncs it.
	$effect(() => {
		ssid = wlan.ssid;
		authMode = wlan.auth_mode || 'WPA2';
		encryption = wlan.encryption || 'AES';
		hidden = wlan.hidden;
	});

	function errMessage(e: unknown): string {
		return e instanceof Error ? e.message : 'Request failed.';
	}

	// Apply the SSID advertisement change right away (same model as the
	// enable/disable switch in the slots table) instead of waiting for Save.
	async function applyHidden(next: boolean) {
		if (!ip) return;
		hidden = next;
		hiddenBusy = true;
		try {
			await updateWLAN(ip, wlan.wlan, { hidden: next });
			toast.success(
				`WLAN ${wlan.wlan} ${next ? 'hidden' : 'visible'} — pushed now if the CPE responds, else on its next inform (~30s).`
			);
		} catch (e) {
			hidden = !next;
			toast.error(`WLAN ${wlan.wlan} ${next ? 'hide' : 'unhide'} failed: ${errMessage(e)}`);
		} finally {
			hiddenBusy = false;
		}
	}

	async function save() {
		if (!ip) return;
		busy = true;
		try {
			const body: Record<string, unknown> = { ssid: ssid.trim(), auth_mode: authMode, encryption };
			if (password) body.password = password;
			await updateWLAN(ip, wlan.wlan, body);

			const radio: { channel?: string; bandwidth?: string } = {};
			if (channel !== UNCHANGED) radio.channel = channel;
			if (bandwidth !== UNCHANGED) radio.bandwidth = bandwidth;
			if (Object.keys(radio).length > 0) await updateWLANRadio(ip, wlan.wlan, radio);

			toast.success(
				`WLAN ${wlan.wlan} settings submitted — pushed now if the CPE responds, else on its next inform (~30s).`
			);
			password = '';
			channel = UNCHANGED;
			bandwidth = UNCHANGED;
		} catch (e) {
			toast.error(`WLAN ${wlan.wlan} update failed: ${errMessage(e)}`);
		} finally {
			busy = false;
		}
	}
</script>

<Card.Root>
	<Card.Header>
		<div class="flex flex-wrap items-center gap-2">
			<Card.Title>WLAN {wlan.wlan}</Card.Title>
			<Badge variant="outline" class="text-muted-foreground">{wlan.band}</Badge>
			{#if currentChannel}
				<Badge variant="outline" class="text-muted-foreground">channel {currentChannel}</Badge>
			{/if}
		</div>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={(e) => {
				e.preventDefault();
				save();
			}}
		>
			<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-ssid`}>SSID</Field.FieldLabel>
					<Input id={`wlan-${wlan.wlan}-ssid`} bind:value={ssid} disabled={busy} />
				</Field.Field>

				<Field.Field orientation="horizontal">
					<Field.Content>
						<Field.FieldLabel for={`wlan-${wlan.wlan}-hidden`}>
							Hide SSID
						</Field.FieldLabel>
						<Field.FieldDescription>
							{hidden ? 'Not broadcast' : 'Broadcast publicly'}
						</Field.FieldDescription>
					</Field.Content>
					<Switch
						id={`wlan-${wlan.wlan}-hidden`}
						bind:checked={hidden}
						onCheckedChange={applyHidden}
						disabled={busy || hiddenBusy || !ip}
					/>
				</Field.Field>

				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-pass`}>Password</Field.FieldLabel>
					<Input
						id={`wlan-${wlan.wlan}-pass`}
						type="password"
						bind:value={password}
						placeholder="Leave blank to keep current"
						disabled={busy}
					/>
				</Field.Field>

				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-auth`}>Security</Field.FieldLabel>
					<Select.Root type="single" bind:value={authMode}>
						<Select.Trigger id={`wlan-${wlan.wlan}-auth`} class="w-full">
							<Select.Value placeholder="Security mode" />
						</Select.Trigger>
						<Select.Content>
							{#each AUTH_MODES as mode (mode)}
								<Select.Item value={mode}>{mode}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</Field.Field>

				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-enc`}>Encryption</Field.FieldLabel>
					<Select.Root type="single" bind:value={encryption}>
						<Select.Trigger id={`wlan-${wlan.wlan}-enc`} class="w-full">
							<Select.Value placeholder="Encryption" />
						</Select.Trigger>
						<Select.Content>
							{#each ENCRYPTIONS as enc (enc)}
								<Select.Item value={enc}>{enc}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</Field.Field>

				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-chan`}>Channel</Field.FieldLabel>
					<Select.Root type="single" bind:value={channel}>
						<Select.Trigger id={`wlan-${wlan.wlan}-chan`} class="w-full">
							<Select.Value placeholder="Unchanged" />
						</Select.Trigger>
						<Select.Content>
							<Select.Item value={UNCHANGED}>Unchanged</Select.Item>
							<Select.Item value="Auto">Auto</Select.Item>
							{#each channels as ch (ch)}
								<Select.Item value={ch}>{ch}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</Field.Field>

				<Field.Field>
					<Field.FieldLabel for={`wlan-${wlan.wlan}-width`}>Channel width</Field.FieldLabel>
					<Select.Root type="single" bind:value={bandwidth}>
						<Select.Trigger id={`wlan-${wlan.wlan}-width`} class="w-full">
							<Select.Value placeholder="Unchanged" />
						</Select.Trigger>
						<Select.Content>
							<Select.Item value={UNCHANGED}>Unchanged</Select.Item>
							<Select.Item value="Auto">Auto</Select.Item>
							{#each bandwidths as bw (bw)}
								<Select.Item value={bw}>{bw}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</Field.Field>
			</div>

			<Button type="submit" class="self-start" disabled={busy || !ssid.trim() || !ip}>
				{#if busy}
					<Spinner data-icon="inline-start" />
				{:else}
					<SaveIcon data-icon="inline-start" />
				{/if}
				Save WLAN {wlan.wlan}
			</Button>
		</form>
	</Card.Content>
</Card.Root>
