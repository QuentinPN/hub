<script lang="ts">
	import { resolve } from '$app/paths';
	import type { ServiceSummary } from '#lib/api/types.ts';
	import ServiceIcon from './ServiceIcon.svelte';
	import StatusDot from './StatusDot.svelte';

	let {
		service,
		sub,
		showFavorite = false
	}: { service: ServiceSummary; sub?: string; showFavorite?: boolean } = $props();
	const subtitle = $derived(sub ?? `${service.hostId} · ${service.note}`);
</script>

<a
	class="app"
	class:cfg={service.status === 'config_required'}
	class:unk={service.status === 'unknown'}
	href={resolve('/services/[slug]', { slug: service.slug })}
>
	<div class="row">
		<ServiceIcon icon={service.icon} monogram={service.monogram} color={service.color} />
		<span class="marks">
			{#if showFavorite && service.favorite}
				<svg width="15" height="15" viewBox="0 0 24 24" fill="var(--accent-play)" role="img"
					><title>Favori</title><path
						d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z"
					></path></svg
				>
			{/if}
			<StatusDot status={service.status} />
		</span>
	</div>
	<div>
		<div class="name">{service.name}</div>
		<div class="meta muted">{subtitle}</div>
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
	}
	/* Configuration requise : bord pointillé ; hôte injoignable : hachures grises. */
	.cfg {
		border-style: dashed;
		border-color: rgba(255, 255, 255, 0.25);
		background: transparent;
	}
	.unk {
		background: var(--hatch);
	}
	.marks {
		display: inline-flex;
		align-items: center;
		gap: 8px;
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
