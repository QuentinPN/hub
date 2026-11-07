<script lang="ts">
	import { afterNavigate, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ApiError, setFavorite } from '#lib/api/client.ts';
	import type { ServiceDetail } from '#lib/api/types.ts';
	import EditServiceDialog from '#lib/components/EditServiceDialog.svelte';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import InfoCard from '#lib/components/InfoCard.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import ServiceIcon from '#lib/components/ServiceIcon.svelte';
	import Shelf from '#lib/components/Shelf.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import UrlCard from '#lib/components/UrlCard.svelte';
	import WidgetsTab from '#lib/components/WidgetsTab.svelte';
	import { statusStyle } from '#lib/status.ts';
	import { tabTitle } from '#lib/shell.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Conteneurs et Historique arrivent par jalons.
	const later = ['Conteneurs', 'Historique'];
	const onWidgets = $derived(page.url.hash === '#widgets');

	// Le lien retour ramène d'où l'on vient : le catalogue ou, par défaut, l'accueil.
	let fromCatalogue = $state(false);
	afterNavigate(({ from }) => {
		// Au chargement direct d'une fiche, `from.url` vaut null malgré son type.
		const url = from?.url as URL | null | undefined;
		fromCatalogue = url?.pathname === resolve('/services');
	});

	// La dernière réponse du serveur après une action : recharger la page ferait clignoter
	// le squelette.
	let fresh = $state<ServiceDetail | null>(null);
	let editing = $state(false);
	let actionError = $state('');

	async function toggleFavorite(service: ServiceDetail) {
		actionError = '';
		try {
			fresh = await setFavorite(fetch, service.slug, !service.favorite);
		} catch (err) {
			actionError = `Favori non enregistré : ${err instanceof Error ? err.message : String(err)}`;
		}
	}

	let name = $state('Service');
	$effect(() => {
		data.service.then((svc) => (name = svc.name)).catch(() => (name = 'Service'));
	});
</script>

<svelte:head><title>{tabTitle(name)}</title></svelte:head>

{#await data.service}
	<section class="hero">
		<div class="hero-body" aria-busy="true">
			<Skeleton width="96px" height="96px" radius="24px" />
			<Skeleton width="min(420px, 80%)" height="56px" radius="14px" />
		</div>
	</section>
{:then loaded}
	{@const svc = fresh?.slug === loaded.slug ? fresh : loaded}
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
						<span class="category"
							>{svc.category} · {svc.hostId}{svc.hidden ? ' · masqué' : ''}</span
						>
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
			{#if svc.discovered}
				<div class="hero-actions">
					{#if svc.url}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- adresse externe -->
						<a class="btn primary" href={svc.url} target="_blank" rel="noopener noreferrer">
							Ouvrir {svc.name}
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
					{/if}
					<button
						type="button"
						class="btn"
						class:fav={svc.favorite}
						aria-pressed={svc.favorite}
						onclick={() => toggleFavorite(svc)}
					>
						<svg
							width="18"
							height="18"
							viewBox="0 0 24 24"
							fill={svc.favorite ? 'currentColor' : 'none'}
							stroke="currentColor"
							stroke-width="2"
							stroke-linejoin="round"
							aria-hidden="true"
							><path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z"
							></path></svg
						>
						Favori
					</button>
					<button type="button" class="btn" onclick={() => (editing = true)}>Modifier</button>
				</div>
			{/if}
			<ul class="stats">
				{#each svc.stats as s (s.label)}
					<li><b class="tabular">{s.value}</b><span class="muted">{s.label}</span></li>
				{/each}
			</ul>
		</div>
	</section>

	<div class="content">
		<nav class="subtabs" aria-label="Sections du service">
			<a
				class="subtab"
				class:on={!onWidgets}
				href="#apercu"
				aria-current={onWidgets ? undefined : 'page'}>Aperçu</a
			>
			<a
				class="subtab"
				class:on={onWidgets}
				href="#widgets"
				aria-current={onWidgets ? 'page' : undefined}>Widgets</a
			>
			{#each later as t (t)}
				<span class="subtab" aria-disabled="true">{t}</span>
			{/each}
		</nav>
		{#if onWidgets}
			<WidgetsTab service={svc} onchange={(s) => (fresh = s)} />
		{:else}
			{#if svc.upcoming.length > 0}
				<Shelf title="Cette semaine" id="week" focusable>
					{#each svc.upcoming as item (item.title + item.detail)}
						<PosterCard {item} />
					{/each}
				</Shelf>
			{/if}
			<p class="action-error" role="alert">{actionError}</p>
			<div class="cards">
				{#if svc.discovered}
					<UrlCard service={svc} onchange={(s) => (fresh = s)} />
				{/if}
				<InfoCard title="Identification" id="ident" rows={svc.identification} />
				<InfoCard title="Conteneurs et accès" id="access" rows={svc.access} />
				<InfoCard title="Connexion du widget" id="conn" rows={svc.connection} />
			</div>
		{/if}
	</div>
	{#if editing}
		<EditServiceDialog
			service={svc}
			onclose={() => (editing = false)}
			onsaved={(s) => (fresh = s)}
		/>
	{/if}
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
	.hero-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
	}
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 9px;
		min-height: 48px;
		box-sizing: border-box;
		padding: 0 18px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-button);
		background: var(--surface-2);
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	.fav {
		border-color: var(--tint-accent-border-strong);
		background: var(--tint-accent-bg-strong);
		color: var(--accent-text);
	}
	.action-error {
		margin: 0;
		color: var(--text-danger);
		font-size: 14px;
	}
	.action-error:empty {
		display: none;
	}
	.hero {
		--tint: #24466f;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		min-height: 460px;
		padding: calc(var(--shell-h) + 48px) var(--space-page) 44px;
		background:
			radial-gradient(
				110% 90% at 80% 20%,
				color-mix(in srgb, var(--tint) var(--tint-mix), var(--bg)) 0%,
				transparent 62%
			),
			linear-gradient(180deg, var(--hero-fade) 0%, var(--bg) 100%);
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
		color: var(--hero-meta);
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
		background: var(--glass-scrim);
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
		background: var(--fill-selected);
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
