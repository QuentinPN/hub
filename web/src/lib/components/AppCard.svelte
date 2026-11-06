<script lang="ts">
	import { resolve } from '$app/paths';
	import type { ServiceSummary } from '#lib/api/types.ts';
	import ServiceIcon from './ServiceIcon.svelte';
	import StatusDot from './StatusDot.svelte';

	let {
		service,
		sub,
		onfavorite
	}: {
		service: ServiceSummary;
		sub?: string;
		/** Avec lui, la carte montre l'étoile. */
		onfavorite?: (service: ServiceSummary) => void;
	} = $props();
	const subtitle = $derived(sub ?? `${service.hostId} · ${service.note}`);
</script>

<article
	class="app"
	class:cfg={service.status === 'config_required'}
	class:unk={service.status === 'unknown'}
>
	<div class="row">
		<ServiceIcon icon={service.icon} monogram={service.monogram} color={service.color} />
		<span class="marks">
			<StatusDot status={service.status} />
			{#if onfavorite}
				<button
					type="button"
					class="mark"
					class:on={service.favorite}
					aria-pressed={service.favorite}
					aria-label="Favori : {service.name}"
					onclick={() => onfavorite(service)}
				>
					<svg
						width="17"
						height="17"
						viewBox="0 0 24 24"
						fill={service.favorite ? 'currentColor' : 'none'}
						stroke="currentColor"
						stroke-width="2"
						stroke-linejoin="round"
						aria-hidden="true"
						><path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z"
						></path></svg
					>
				</button>
			{/if}
			{#if service.url}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- adresse externe -->
				<a
					class="mark open"
					href={service.url}
					target="_blank"
					rel="noopener noreferrer"
					aria-label="Ouvrir {service.name} dans un nouvel onglet"
				>
					<svg
						width="16"
						height="16"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2.2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"><path d="M7 17 17 7M9 7h8v8"></path></svg
					>
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
		</span>
	</div>
	<a class="main" href={resolve('/services/[slug]', { slug: service.slug })}>
		<span class="name">{service.name}</span>
		<span class="meta muted">{subtitle}</span>
	</a>
</article>

<style>
	.app {
		display: flex;
		flex: 0 0 232px;
		position: relative;
		flex-direction: column;
		justify-content: space-between;
		box-sizing: border-box;
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
	.app:has(.main:focus-visible) {
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
		position: relative;
		z-index: 1;
		display: inline-flex;
		align-items: center;
		gap: 2px;
		margin: -8px -8px 0 0;
	}
	.mark {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--text-2);
		cursor: pointer;
	}
	.mark:hover,
	.open {
		color: var(--text);
	}
	.on {
		color: var(--accent-play);
	}
	/* Toute la carte ouvre la fiche ; l'étoile et la flèche restent au-dessus. */
	.main {
		display: flex;
		flex-direction: column;
	}
	.main::after {
		content: '';
		position: absolute;
		inset: 0;
		border-radius: inherit;
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
		.meta,
		.open {
			display: none;
		}
	}
</style>
