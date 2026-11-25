<script lang="ts">
	import ScriptManager from '$lib/components/script-manager.svelte';
	import {
		listVirtualParameters,
		saveVirtualParameter,
		deleteVirtualParameter
	} from '$lib/api/scripts';
</script>

<svelte:head><title>Virtual Parameters · GenieACS Relay</title></svelte:head>

<ScriptManager
	title="Virtual Parameters"
	description="Computed values that GenieACS derives from device data and exposes like any other parameter."
	noun="virtual parameter"
	namePlaceholder="e.g. getUptime"
	scriptPlaceholder={'// Return a computed value\nconst d = args[0];\nlet uptime = d.InternetGatewayDevice.DeviceInfo.UpTime.value[0];\nreturn {writable: false, value: [uptime, "xsd:unsignedInt"]};'}
	list={listVirtualParameters}
	save={saveVirtualParameter}
	remove={deleteVirtualParameter}
/>
