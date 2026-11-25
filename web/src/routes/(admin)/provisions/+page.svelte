<script lang="ts">
	import ScriptManager from '$lib/components/script-manager.svelte';
	import { listProvisions, saveProvision, deleteProvision } from '$lib/api/scripts';
</script>

<svelte:head><title>Provisions · GenieACS Relay</title></svelte:head>

<ScriptManager
	title="Provisions"
	description="JavaScript scripts GenieACS runs on devices in response to presets and events."
	noun="provision"
	namePlaceholder="e.g. refresh-wifi"
	scriptPlaceholder={'// Runs on every inform\nconst now = Date.now();\ndeclare("_LastSync", {value: now, writable: false}, {timestamp: now});'}
	list={listProvisions}
	save={saveProvision}
	remove={deleteProvision}
/>
