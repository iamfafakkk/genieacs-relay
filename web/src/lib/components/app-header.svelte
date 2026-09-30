<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { Separator } from '$lib/components/ui/separator';
	import { page } from '$app/state';

	const titles: Record<string, string> = {
		'/': 'Dashboard'
	};

	const title = $derived(titles[page.url.pathname] ?? 'Admin');
</script>

<header
	class="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12"
>
	<div class="flex items-center gap-2 px-4">
		<Sidebar.Trigger class="-ms-1" />
		<Separator orientation="vertical" class="me-2 data-vertical:h-4 data-vertical:self-auto" />
		<h1 class="text-sm font-medium">{title}</h1>
	</div>
</header>
