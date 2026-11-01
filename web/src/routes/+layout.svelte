<script lang="ts">
	import '@fontsource-variable/inter';
	import '#lib/styles/global.css';
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

{@render children()}
