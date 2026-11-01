<script lang="ts">
	import { resolve } from '$app/paths';
	import type { ServiceSummary } from '#lib/api/types.ts';
	import Monogram from './Monogram.svelte';
	import StatusDot from './StatusDot.svelte';

	let { service }: { service: ServiceSummary } = $props();
</script>

<a class="app" href={resolve('/services/[slug]', { slug: service.slug })}>
	<div class="row">
		<Monogram text={service.monogram} color={service.color} />
		<StatusDot status={service.status} />
	</div>
	<div>
		<div class="name">{service.name}</div>
		<div class="meta muted">{service.hostId} · {service.note}</div>
	</div>
</a>

<style>
	.app {
		display: flex;
		flex: 0 0 232px;
		flex-direction: column;
		justify-content: space-between;
		height: 148px;
		padding: 18px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
		transition:
			transform var(--dur) var(--ease),
			box-shadow var(--dur) var(--ease),
			border-color var(--dur) var(--ease);
	}
	.app:hover,
	.app:focus-visible {
		transform: scale(var(--focus-scale));
		border-color: var(--border-focus);
		background: var(--surface-2);
		box-shadow: var(--shadow-lift);
		outline: none;
	}
	.row {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
	}
	.name {
		font-size: 17px;
		font-weight: 600;
	}
	.meta {
		margin-top: 2px;
		font-size: var(--fs-small);
	}
	@media (max-width: 720px) {
		.app {
			flex-basis: 148px;
			height: 104px;
			padding: 12px;
		}
		.meta {
			display: none;
		}
	}
</style>
