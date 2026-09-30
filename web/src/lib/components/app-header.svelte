<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { Separator } from '$lib/components/ui/separator';
	import { page } from '$app/state';

	const titles: Record<string, string> = {
		'/': 'Dashboard'
	};

	const title = $derived(titles[page.url.pathname] ?? 'Admin');
</script>

<header class="flex h-16 shrink-0 items-center gap-2 border-b px-4">
	<Sidebar.Trigger class="-ml-1" />
	<Separator orientation="vertical" class="mr-2 data-[orientation=vertical]:h-4" />
	<h1 class="text-sm font-medium">{title}</h1>
</header>
