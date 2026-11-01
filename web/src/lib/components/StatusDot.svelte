<script lang="ts">
	import type { ServiceStatus } from '#lib/api/types.ts';
	import { statusStyle } from '#lib/status.ts';

	let { status, size = 9 }: { status: ServiceStatus; size?: number } = $props();
	const style = $derived(statusStyle(status));
</script>

<span
	class="dot"
	style:width="{size}px"
	style:height="{size}px"
	style:background={style.color}
	role="img"
	aria-label={style.label}
></span>

<style>
	.dot {
		display: inline-block;
		flex: none;
		border-radius: 99px;
	}
</style>
