<script lang="ts">
	import { getApiKey, setApiKey } from '$lib/api/client';
	import { listDevices } from '$lib/api/devices';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import * as InputGroup from '$lib/components/ui/input-group';
	import { Button } from '$lib/components/ui/button';
	import { Spinner } from '$lib/components/ui/spinner';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import { goto } from '$app/navigation';

	let apiKey = $state('');
	let showKey = $state(false);
	let pending = $state(false);
	let error = $state('');

	// Pre-fill so a stale key can be corrected without retyping.
	$effect(() => {
		apiKey = getApiKey() ?? '';
	});

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		if (!apiKey.trim()) {
			error = 'API key wajib diisi.';
			return;
		}

		pending = true;
		setApiKey(apiKey.trim());
		try {
			// Validates the key: the backend answers 401 when it is wrong.
			await listDevices(1, 1);
			await goto('/', { invalidateAll: true });
		} catch (e) {
			setApiKey(null);
			error =
				e instanceof Error
					? e.message === 'Missing X-API-Key header' || e.message === 'Invalid API key'
						? 'API key tidak valid.'
						: e.message
					: 'Gagal memverifikasi API key.';
		} finally {
			pending = false;
		}
	}
</script>

<svelte:head><title>Login · GenieACS Relay</title></svelte:head>

<main class="bg-muted/40 flex min-h-svh items-center justify-center p-4">
	<Card.Root class="w-full max-w-sm">
		<Card.Header class="items-center text-center">
			<div
				class="bg-primary text-primary-foreground mx-auto mb-2 flex size-10 items-center justify-center rounded-lg"
			>
				<NetworkIcon />
			</div>
			<Card.Title>GenieACS Relay</Card.Title>
			<Card.Description>Masuk dengan API key untuk membuka admin panel.</Card.Description>
		</Card.Header>

		<Card.Content>
			<form onsubmit={submit} novalidate>
				<Field.FieldGroup>
					<Field.Field data-invalid={!!error}>
						<Field.FieldLabel for="api-key">API Key</Field.FieldLabel>
						<InputGroup.Root>
							<InputGroup.Input
								id="api-key"
								type={showKey ? 'text' : 'password'}
								bind:value={apiKey}
								autocomplete="current-password"
								placeholder="••••••••"
								aria-invalid={!!error}
								disabled={pending}
							/>
							<InputGroup.Addon align="inline-end">
								<InputGroup.Button
									size="icon-xs"
									aria-label={showKey ? 'Sembunyikan API key' : 'Tampilkan API key'}
									onclick={() => (showKey = !showKey)}
								>
									{#if showKey}
										<EyeOffIcon />
									{:else}
										<EyeIcon />
									{/if}
								</InputGroup.Button>
							</InputGroup.Addon>
						</InputGroup.Root>
						<Field.FieldDescription>
							Nilai <code class="text-foreground">AUTH_KEY</code> dari backend. Disimpan di cookie browser.
						</Field.FieldDescription>
						{#if error}
							<Field.FieldError>{error}</Field.FieldError>
						{/if}
					</Field.Field>
				</Field.FieldGroup>

				<Button type="submit" class="mt-6 w-full" disabled={pending}>
					{#if pending}
						<Spinner data-icon="inline-start" />
						Memeriksa…
					{:else}
						<LogInIcon data-icon="inline-start" />
						Masuk
					{/if}
				</Button>
			</form>
		</Card.Content>

		<Card.Footer class="flex-col items-stretch">
			<Alert>
				<KeyRoundIcon />
				<AlertTitle>Tanpa API key?</AlertTitle>
				<AlertDescription>
					Isi <code>AUTH_KEY</code> di <code>.env</code> dan set <code>MIDDLEWARE_AUTH=true</code>, lalu
					jalankan ulang <code>make dev</code>.
				</AlertDescription>
			</Alert>
		</Card.Footer>
	</Card.Root>
</main>
