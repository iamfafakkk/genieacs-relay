<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import GaugeIcon from '@lucide/svelte/icons/gauge';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { setApiKey } from '$lib/api/client';

	const items = [{ title: 'Dashboard', url: '/', icon: GaugeIcon }];

	function logout() {
		setApiKey(null);
		goto('/login', { invalidateAll: true });
	}
</script>

<Sidebar.Root collapsible="offcanvas">
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
								isActive={item.url === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(item.url)}
								tooltipContent={item.title}
							>
								{#snippet child({ props })}
									<a href={item.url} {...props}>
										<item.icon />
										<span>{item.title}</span>
									</a>
								{/snippet}
							</Sidebar.MenuButton>
						</Sidebar.MenuItem>
					{/each}
				</Sidebar.Menu>
			</Sidebar.GroupContent>
		</Sidebar.Group>
	</Sidebar.Content>

	<Sidebar.Footer>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton onclick={logout} tooltipContent="Log out">
					<LogOutIcon />
					<span>Log out</span>
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Footer>

	<Sidebar.Rail />
</Sidebar.Root>
