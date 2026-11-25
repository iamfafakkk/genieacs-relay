<script lang="ts">
	import { onMount } from 'svelte';
	import type { EditorView } from '@codemirror/view';

	interface Props {
		id?: string;
		value?: string;
		placeholder?: string;
		/** Accessible name for the editable area. */
		label?: string;
		class?: string;
	}

	let {
		id,
		value = $bindable(''),
		placeholder = '',
		label = 'Code editor',
		class: className = ''
	}: Props = $props();

	let host = $state<HTMLDivElement | null>(null);
	let view: EditorView | null = null;
	let ready = $state(false);

	// CodeMirror is loaded lazily so it never runs during prerender.
	onMount(() => {
		let destroyed = false;
		void (async () => {
			const [{ EditorView, basicSetup }, { javascript }, { placeholder: cmPlaceholder }] =
				await Promise.all([
					import('codemirror'),
					import('@codemirror/lang-javascript'),
					import('@codemirror/view')
				]);
			if (destroyed || !host) return;
			view = new EditorView({
				parent: host,
				doc: value,
				extensions: [
					basicSetup,
					javascript(),
					cmPlaceholder(placeholder),
					EditorView.contentAttributes.of({
						...(id ? { id } : {}),
						'aria-label': label,
						spellcheck: 'false',
						autocapitalize: 'off',
						autocomplete: 'off'
					}),
					EditorView.updateListener.of((update) => {
						if (update.docChanged) value = update.state.doc.toString();
					}),
					EditorView.theme({
						'&': {
							width: '100%',
							fontSize: '12px',
							fontFamily: 'var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)',
							color: 'var(--foreground)',
							backgroundColor: 'transparent',
							border: '1px solid var(--border)',
							borderRadius: 'calc(var(--radius) - 2px)'
						},
						'&.cm-focused': {
							outline: 'none',
							borderColor: 'var(--ring)',
							boxShadow: '0 0 0 3px color-mix(in oklab, var(--ring) 50%, transparent)'
						},
						'.cm-scroller': {
							minHeight: '20rem',
							maxHeight: '32rem',
							overflow: 'auto'
						},
						'.cm-content': { padding: '8px 0' },
						'.cm-gutters': {
							backgroundColor: 'transparent',
							borderRight: '1px solid var(--border)',
							color: 'var(--muted-foreground)'
						},
						'.cm-activeLine': {
							backgroundColor: 'color-mix(in oklab, var(--muted) 40%, transparent)'
						},
						'.cm-activeLineGutter': {
							backgroundColor: 'color-mix(in oklab, var(--muted) 40%, transparent)'
						},
						'.cm-placeholder': { color: 'var(--muted-foreground)' }
					})
				]
			});
			ready = true;
		})();

		return () => {
			destroyed = true;
			view?.destroy();
			view = null;
		};
	});

	// Keep the document in sync when the parent swaps in another script.
	$effect(() => {
		if (!ready || !view) return;
		const next = value;
		if (next !== view.state.doc.toString()) {
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: next } });
		}
	});
</script>

<div bind:this={host} class={className}></div>
