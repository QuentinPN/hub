<script lang="ts">
	import { afterNavigate, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError } from '#lib/api/client.ts';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import InfoCard from '#lib/components/InfoCard.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import ServiceIcon from '#lib/components/ServiceIcon.svelte';
	import Shelf from '#lib/components/Shelf.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import { statusStyle } from '#lib/status.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Les sous-onglets autres qu'Aperçu arrivent avec la découverte réelle (phase 2).
	const subtabs = ['Aperçu', 'Widgets', 'Conteneurs', 'Historique'];

	// Le lien retour ramène d'où l'on vient : le catalogue ou, par défaut, l'accueil.
	let fromCatalogue = $state(false);
	afterNavigate(({ from }) => {
		fromCatalogue = from?.url.pathname === resolve('/services');
	});

	let name = $state('Service');
	$effect(() => {
		data.service.then((svc) => (name = svc.name)).catch(() => (name = 'Service'));
	});
</script>

<svelte:head><title>{name} · Hub</title></svelte:head>

{#await data.service}
	<section class="hero">
		<div class="hero-body" aria-busy="true">
			<Skeleton width="96px" height="96px" radius="24px" />
			<Skeleton width="min(420px, 80%)" height="56px" radius="14px" />
		</div>
	</section>
{:then svc}
	<section class="hero" style:--tint={svc.tint}>
		<div class="hero-body">
			<div class="hero-text">
				{#if fromCatalogue}
					<a class="muted back" href={resolve('/services')}>‹ Tous les services</a>
				{:else}
					<a class="muted back" href={resolve('/')}>‹ Accueil</a>
				{/if}
				<div class="title-row">
					<ServiceIcon icon={svc.icon} monogram={svc.monogram} color={svc.color} size={96} />
					<div>
						<span class="category">{svc.category} · {svc.hostId}</span>
						<h1>{svc.name}</h1>
					</div>
				</div>
				<p class="state">
					<span class="inline"
						><StatusDot status={svc.status} size={8} />{statusStyle(svc.status)
							.label}{#if svc.latencyMs !== null}&nbsp;· {svc.latencyMs} ms{/if}</span
					>
					<span class="muted">{svc.version}</span>
				</p>
			</div>
			<ul class="stats">
				{#each svc.stats as s (s.label)}
					<li><b class="tabular">{s.value}</b><span class="muted">{s.label}</span></li>
				{/each}
			</ul>
		</div>
	</section>

	<div class="content">
		<nav class="subtabs" aria-label="Sections du service">
			{#each subtabs as t, i (t)}
				<span
					class="subtab"
					class:on={i === 0}
					aria-current={i === 0 ? 'page' : undefined}
					aria-disabled={i === 0 ? undefined : 'true'}>{t}</span
				>
			{/each}
		</nav>
		{#if svc.upcoming.length > 0}
			<Shelf title="Cette semaine" id="week" focusable>
				{#each svc.upcoming as item (item.title + item.detail)}
					<PosterCard {item} />
				{/each}
			</Shelf>
		{/if}
		<div class="cards">
			<InfoCard title="Identification" id="ident" rows={svc.identification} />
			<InfoCard title="Conteneurs et accès" id="access" rows={svc.access} />
			<InfoCard title="Connexion du widget" id="conn" rows={svc.connection} />
		</div>
	</div>
{:catch err}
	<section class="hero">
		<div class="hero-body">
			{#if err instanceof ApiError && err.status === 404}
				<ErrorPanel
					title="Service introuvable"
					detail="Ce service n'existe pas ou n'a pas encore été découvert."
				/>
			{:else}
				<ErrorPanel
					title="Impossible de charger le service"
					detail={err instanceof Error ? err.message : String(err)}
					onretry={() => invalidateAll()}
				/>
			{/if}
		</div>
	</section>
{/await}

<style>
	.hero {
		--tint: #24466f;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		min-height: 460px;
		padding: calc(var(--shell-h) + 48px) var(--space-page) 44px;
		background:
			radial-gradient(110% 90% at 80% 20%, var(--tint) 0%, rgba(7, 8, 10, 0) 62%),
			linear-gradient(180deg, #0f1620 0%, var(--bg) 100%);
	}
	.hero-body {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: 32px;
	}
	.hero-body[aria-busy='true'] {
		flex-direction: column;
		align-items: flex-start;
	}
	.hero-text {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.back {
		font-size: 15px;
	}
	.title-row {
		display: flex;
		align-items: center;
		gap: 22px;
	}
	.category {
		color: #b9d3f0;
		font-size: 15px;
	}
	h1 {
		margin: 4px 0 0;
		font-size: var(--fs-hero);
		font-weight: 700;
		line-height: 1;
		letter-spacing: -0.03em;
	}
	.state {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		margin: 0;
		font-size: 17px;
	}
	.inline {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}
	.stats {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.stats li {
		min-width: 150px;
		padding: 14px 18px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: rgba(0, 0, 0, 0.35);
		backdrop-filter: var(--blur);
		font-size: 14px;
	}
	.stats b {
		display: block;
		margin-bottom: 2px;
		font-size: 26px;
	}
	.content {
		display: flex;
		flex-direction: column;
		gap: 22px;
		padding: 8px var(--space-page) 64px;
	}
	.subtabs {
		display: flex;
		flex-wrap: wrap;
		align-self: flex-start;
		gap: 4px;
		padding: 5px;
		border-radius: var(--r-pill);
		background: var(--surface-1);
	}
	.subtab {
		padding: 8px 18px;
		border-radius: var(--r-pill);
		color: var(--text-2);
		font-size: 15px;
		font-weight: 500;
	}
	.subtab[aria-disabled='true'] {
		opacity: 0.55;
	}
	.subtab.on {
		background: rgba(255, 255, 255, 0.92);
		color: var(--text-inverse);
	}
	.cards {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
	}
	@media (max-width: 720px) {
		.title-row {
			gap: 14px;
		}
		.title-row :global(.mono-tile),
		.title-row :global(.svc-icon) {
			width: 64px !important;
			height: 64px !important;
		}
		.stats {
			display: grid;
			grid-template-columns: repeat(3, minmax(0, 1fr));
			gap: 8px;
			width: 100%;
		}
		.stats li {
			min-width: 0;
			padding: 10px 12px;
			font-size: 12px;
		}
		.stats b {
			font-size: 20px;
		}
		.subtab {
			padding: 8px 14px;
		}
	}
</style>
