<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Empty from '$lib/components/ui/empty';
	import * as Field from '$lib/components/ui/field';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import CodeEditor from '$lib/components/code-editor.svelte';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Spinner } from '$lib/components/ui/spinner';
	import { toast } from 'svelte-sonner';
	import { onMount } from 'svelte';
	import FileCodeIcon from '@lucide/svelte/icons/file-code-2';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import type { ScriptEntry, ScriptOperationResponse } from '$lib/api/types';

	interface Props {
		title: string;
		description: string;
		/** Singular noun used in buttons and dialogs, e.g. "provision". */
		noun: string;
		namePlaceholder: string;
		scriptPlaceholder: string;
		list: () => Promise<ScriptEntry[]>;
		save: (name: string, script: string) => Promise<ScriptOperationResponse>;
		remove: (name: string) => Promise<ScriptOperationResponse>;
	}

	let { title, description, noun, namePlaceholder, scriptPlaceholder, list, save, remove }: Props =
		$props();

	const NAME_RE = /^[a-zA-Z0-9_-]{1,64}$/;

	let entries = $state<ScriptEntry[]>([]);
	let loading = $state(true);
	let error = $state('');

	let editorOpen = $state(false);
	let editingName = $state<string | null>(null);
	let formName = $state('');
	let formScript = $state('');
	let saving = $state(false);
	let deleting = $state<string | null>(null);

	const nameValid = $derived(NAME_RE.test(formName));
	const canSave = $derived(nameValid && formScript.trim().length > 0 && !saving);

	async function load() {
		loading = true;
		error = '';
		try {
			entries = await list();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function openNew() {
		editingName = null;
		formName = '';
		formScript = '';
		editorOpen = true;
	}

	function openEdit(entry: ScriptEntry) {
		editingName = entry.name;
		formName = entry.name;
		formScript = entry.script;
		editorOpen = true;
	}

	function closeEditor() {
		editorOpen = false;
		editingName = null;
		formName = '';
		formScript = '';
	}

	async function submit() {
		if (!canSave) return;
		saving = true;
		try {
			const res = await save(formName, formScript);
			toast.success(res.message);
			closeEditor();
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : String(e));
		} finally {
			saving = false;
		}
	}

	async function removeEntry(entry: ScriptEntry) {
		deleting = entry.name;
		try {
			const res = await remove(entry.name);
			toast.success(res.message);
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : String(e));
		} finally {
			deleting = null;
		}
	}

	function preview(script: string): string {
		const line = script.split('\n').find((l) => l.trim().length > 0) ?? '';
		return line.trim();
	}
</script>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<h2 class="text-lg font-semibold">{title}</h2>
			<p class="text-muted-foreground text-sm">{description}</p>
		</div>
		<div class="flex items-center gap-2">
			<Button variant="outline" size="sm" onclick={load} disabled={loading}>
				{#if loading}
					<Spinner data-icon="inline-start" />
				{:else}
					<RefreshCwIcon data-icon="inline-start" />
				{/if}
				Refresh
			</Button>
			<Button size="sm" onclick={openNew}>
				<PlusIcon data-icon="inline-start" />
				New {noun}
			</Button>
		</div>
	</div>

	{#if error}
		<Alert variant="destructive">
			<TriangleAlertIcon />
			<AlertTitle>Failed to load {noun}s</AlertTitle>
			<AlertDescription>{error}</AlertDescription>
		</Alert>
	{/if}

	{#if editorOpen}
		<Card.Root>
			<Card.Header>
				<Card.Title>
					{editingName ? `Edit ${noun}: ${editingName}` : `New ${noun}`}
				</Card.Title>
				<Card.Description>
					{editingName
						? 'Saving replaces the script stored in GenieACS. The name cannot be changed here.'
						: `Creates a new script in GenieACS. Choose a name now — it identifies the ${noun}.`}
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<div class="flex flex-col gap-4">
					<Field.Field>
						<Field.FieldLabel for="script-name">Name</Field.FieldLabel>
						<Input
							id="script-name"
							bind:value={formName}
							placeholder={namePlaceholder}
							disabled={editingName !== null}
							aria-invalid={formName.length > 0 && !nameValid}
						/>
						<Field.FieldDescription>
							Letters, numbers, underscore, or hyphen. 1–64 characters, no periods.
						</Field.FieldDescription>
					</Field.Field>
					<Field.Field>
						<Field.FieldLabel for="script-body">JavaScript</Field.FieldLabel>
						<CodeEditor
							id="script-body"
							bind:value={formScript}
							placeholder={scriptPlaceholder}
							label="JavaScript"
						/>
					</Field.Field>
				</div>
			</Card.Content>
			<Card.Footer class="flex justify-end gap-2">
				<Button variant="outline" onclick={closeEditor} disabled={saving}>Cancel</Button>
				<Button onclick={submit} disabled={!canSave}>
					{#if saving}
						<Spinner data-icon="inline-start" />
					{/if}
					Save
				</Button>
			</Card.Footer>
		</Card.Root>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Scripts</Card.Title>
			<Card.Description>Everything currently stored in GenieACS for this collection.</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if loading && entries.length === 0}
				<div class="flex flex-col gap-2">
					{#each Array(4) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if entries.length === 0}
				<Empty.Root>
					<Empty.Header>
						<Empty.Media variant="icon">
							<FileCodeIcon />
						</Empty.Media>
						<Empty.Title>No {noun}s yet</Empty.Title>
						<Empty.Description>
							Create one with “New {noun}” and it will show up here once saved to GenieACS.
						</Empty.Description>
					</Empty.Header>
				</Empty.Root>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Name</Table.Head>
							<Table.Head>Preview</Table.Head>
							<Table.Head>Size</Table.Head>
							<Table.Head class="text-right">Action</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each entries as entry (entry.name)}
							<Table.Row>
								<Table.Cell class="font-mono text-sm font-medium">{entry.name}</Table.Cell>
								<Table.Cell class="text-muted-foreground max-w-96 truncate font-mono text-xs">
									{preview(entry.script) || '—'}
								</Table.Cell>
								<Table.Cell class="text-muted-foreground font-mono text-xs">
									{entry.script.length} chars
								</Table.Cell>
								<Table.Cell class="text-right whitespace-nowrap">
									<Button variant="ghost" size="sm" onclick={() => openEdit(entry)}>
										<PencilIcon data-icon="inline-start" />
										Edit
									</Button>
									<AlertDialog.Root>
										<AlertDialog.Trigger>
											{#snippet child({ props })}
												<Button
													{...props}
													variant="ghost"
													size="sm"
													class="text-destructive hover:text-destructive"
													disabled={deleting === entry.name}
												>
													{#if deleting === entry.name}
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
												<AlertDialog.Title>Delete {noun}?</AlertDialog.Title>
												<AlertDialog.Description>
													{entry.name} will be removed from GenieACS. Devices that still use it
													stop receiving its changes on their next inform.
												</AlertDialog.Description>
											</AlertDialog.Header>
											<AlertDialog.Footer>
												<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
												<AlertDialog.Action
													variant="destructive"
													onclick={() => removeEntry(entry)}
												>
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
		<Card.Footer>
			<p class="text-muted-foreground text-sm">
				{entries.length} {noun}{entries.length === 1 ? '' : 's'} in GenieACS.
			</p>
		</Card.Footer>
	</Card.Root>
</div>
