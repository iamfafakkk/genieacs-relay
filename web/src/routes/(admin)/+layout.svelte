<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import AppSidebar from '$lib/components/app-sidebar.svelte';
	import AppHeader from '$lib/components/app-header.svelte';
	import { onMount } from 'svelte';
	import { startJobNotifier } from '$lib/stores/jobs.svelte';

	let { children } = $props();

	// Stream worker jobs over WebSocket and raise sonner notifications globally.
	onMount(startJobNotifier);
</script>

<Sidebar.Provider>
	<AppSidebar />
	<Sidebar.Inset>
		<AppHeader />
		<div class="flex flex-1 flex-col gap-4 p-4">
			{@render children()}
		</div>
	</Sidebar.Inset>
</Sidebar.Provider>
