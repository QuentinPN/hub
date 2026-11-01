<script lang="ts">
	import type { KeyValue } from '#lib/api/types.ts';

	let { title, id, rows }: { title: string; id: string; rows: KeyValue[] } = $props();
</script>

<section class="info" aria-labelledby={id}>
	<h3 {id}>{title}</h3>
	<dl>
		{#each rows as row (row.label)}
			<dt class="muted">{row.label}</dt>
			<dd class:mono={row.mono}>{row.value}</dd>
		{/each}
	</dl>
</section>

<style>
	.info {
		display: flex;
		flex: 1 1 320px;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	h3 {
		margin: 0;
		font-size: 17px;
		font-weight: 600;
	}
	dl {
		display: grid;
		grid-template-columns: 120px minmax(0, 1fr);
		gap: 10px 14px;
		margin: 0;
		font-size: 14px;
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
</style>
