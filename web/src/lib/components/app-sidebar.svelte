<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import GaugeIcon from '@lucide/svelte/icons/gauge';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { setApiKey } from '$lib/api/client';
	import { jobsState } from '$lib/stores/jobs.svelte';

	const items = [
		{ title: 'Dashboard', url: '/', icon: LayoutDashboardIcon },
		{ title: 'Devices', url: '/devices', icon: GaugeIcon },
		{ title: 'Jobs', url: '/jobs', icon: ListChecksIcon }
	];

	// Device detail pages keep the Devices entry highlighted.
	function isActive(url: string, pathname: string): boolean {
		if (url === '/') return pathname === '/';
		if (url === '/devices') return pathname === '/devices' || pathname.startsWith('/devices/');
		return pathname.startsWith(url);
	}

	function logout() {
		setApiKey(null);
		goto('/login', { invalidateAll: true });
	}
</script>

<Sidebar.Root collapsible="icon">
	<Sidebar.Header>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton size="lg" isActive={page.url.pathname === '/'}>
					{#snippet child({ props })}
						<a href="/" {...props}>
							<div
								class="bg-primary text-primary-foreground flex aspect-square size-8 items-center justify-center rounded-lg"
							>
								<NetworkIcon />
							</div>
							<div class="grid flex-1 text-left text-sm leading-tight">
								<span class="truncate font-semibold">GenieACS Relay</span>
								<span class="text-muted-foreground truncate text-xs">Admin Panel</span>
							</div>
						</a>
					{/snippet}
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Header>

	<Sidebar.Content>
		<Sidebar.Group>
			<Sidebar.GroupLabel>Overview</Sidebar.GroupLabel>
			<Sidebar.GroupContent>
				<Sidebar.Menu>
					{#each items as item (item.url)}
						<Sidebar.MenuItem>
							<Sidebar.MenuButton
								isActive={isActive(item.url, page.url.pathname)}
								tooltipContent={item.title}
							>
								{#snippet child({ props })}
									<a href={item.url} {...props}>
										<item.icon />
										<span>{item.title}</span>
									</a>
								{/snippet}
							</Sidebar.MenuButton>
							{#if item.url === '/jobs' && jobsState.active > 0}
								<Sidebar.MenuBadge>{jobsState.active}</Sidebar.MenuBadge>
							{/if}
						</Sidebar.MenuItem>
					{/each}
				</Sidebar.Menu>
			</Sidebar.GroupContent>
		</Sidebar.Group>
	</Sidebar.Content>

	<Sidebar.Footer>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton size="lg" onclick={logout} tooltipContent="Log out">
					<div
						class="bg-muted text-muted-foreground flex aspect-square size-8 items-center justify-center rounded-lg"
					>
						<LogOutIcon />
					</div>
					<div class="grid flex-1 text-left text-sm leading-tight">
						<span class="truncate font-medium">Log out</span>
						<span class="text-muted-foreground truncate text-xs">End session</span>
					</div>
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Footer>

	<Sidebar.Rail />
</Sidebar.Root>
