<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { getHome } from '#lib/api/client.ts';
	import type { Home } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import { onWidgetNews } from '#lib/live.ts';
	import { mediaPoster, upcomingPoster, widgetStateView } from '#lib/widgets.ts';
	import AppCard from '#lib/components/AppCard.svelte';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import HostCard from '#lib/components/HostCard.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import Shelf from '#lib/components/Shelf.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import { shell } from '#lib/shell.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const DEFAULT_ROWS = ['playing', 'favorites', 'latest', 'upcoming', 'downloads', 'hosts'];
	const count = (n: number) => n.toLocaleString('fr-FR');

	// Relu sans repasser par le squelette : la lecture en cours change sous les yeux, dès que
	// le serveur le signale. La relecture régulière rattrape un signal perdu.
	let fresh = $state<Home | null>(null);
	function reload() {
		if (document.visibilityState !== 'visible') return;
		getHome(fetch)
			.then((home) => {
				fresh = home;
				shell.summary = home.summary;
			})
			.catch(() => {});
	}
	$effect(() => {
		const poll = setInterval(reload, 30_000);
		const stop = onWidgetNews(reload);
		return () => {
			clearInterval(poll);
			stop();
		};
	});

	// Le compteur « en ligne » de la barre du haut n'apparaît que sur l'accueil.
	$effect(() => {
		let current = true;
		data.home
			.then((home) => {
				if (current) shell.summary = home.summary;
			})
			.catch(() => {});
		return () => {
			current = false;
			shell.summary = null;
		};
	});
</script>

<svelte:head><title>Accueil · Hub</title></svelte:head>

{#await data.home}
	<section class="hero">
		<div class="hero-body" aria-busy="true">
			<Skeleton width="180px" height="16px" />
			<Skeleton width="min(560px, 90%)" height="56px" radius="14px" />
			<Skeleton width="320px" height="16px" />
		</div>
	</section>
{:then loaded}
	{@const home = fresh ?? loaded}
	{@const main = home.sessions[0]}
	{@const rows = home.rows ?? DEFAULT_ROWS}
	{@const welcome = home.summary.total === 0 && home.hosts.length === 0}
	{@const showHero = rows.includes('playing') || welcome}
	{#if showHero}
		<section class="hero">
			{#if main}
				<div class="hero-body">
					<div class="hero-text">
						<span class="eyebrow">
							<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"
								><path d="M7 4v16l13-8z"></path></svg
							>
							En lecture sur Emby
						</span>
						<h1>{main.title}</h1>
						<p class="muted lead">
							{main.device} · {main.transcoding ? 'transcodage' : 'lecture directe'} · {main.durationLabel}
						</p>
						<div
							class="progress"
							role="progressbar"
							aria-label="Progression"
							aria-valuenow={Math.round(main.progress * 100)}
							aria-valuemin={0}
							aria-valuemax={100}
						>
							<span style:width="{main.progress * 100}%"></span>
						</div>
						<div class="actions">
							<a
								class="btn primary"
								href={resolve('/services/[slug]', { slug: home.sessionsSlug ?? 'emby' })}
								>Ouvrir la fiche Emby</a
							>
							<span class="muted sessions"
								>{home.sessions.length}
								{home.sessions.length > 1 ? 'sessions' : 'session'} en cours</span
							>
						</div>
					</div>
					<ul class="others">
						{#each home.sessions.slice(1) as s (s.title)}
							<li>
								<span class="muted small">{s.device}</span>
								<strong>{s.title}</strong>
							</li>
						{/each}
					</ul>
				</div>
			{:else if home.summary.total === 0 && home.hosts.length === 0}
				<div class="hero-body">
					<div class="hero-text">
						<h1>Bienvenue sur Hub</h1>
						<p class="muted lead">
							Aucun service pour l’instant. Ils apparaîtront ici une fois un hôte connecté.
						</p>
						<div class="actions">
							<a class="btn primary" href={resolve('/settings#hosts')}>Connecter un hôte</a>
						</div>
					</div>
				</div>
			{:else if home.sessionsSlug}
				<div class="hero-body">
					<div class="hero-text">
						<span class="eyebrow idle">
							<svg
								width="14"
								height="14"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2.2"
								stroke-linejoin="round"
								aria-hidden="true"
								><rect x="6" y="5" width="4" height="14" rx="1"></rect><rect
									x="14"
									y="5"
									width="4"
									height="14"
									rx="1"
								></rect></svg
							>
							Emby · aucune session
						</span>
						<h1>Rien en lecture</h1>
						{#if home.lastAdded}
							<p class="muted lead">
								Dernier ajout : {home.lastAdded.title}{home.lastAdded.addedAt
									? `, ${ago(home.lastAdded.addedAt, Date.now())}`
									: ''}
							</p>
						{/if}
						<div class="actions">
							<a class="btn primary" href={resolve('/services/[slug]', { slug: home.sessionsSlug })}
								>Ouvrir la fiche Emby</a
							>
							{#if home.library}
								<span class="muted sessions"
									>{count(home.library.movies)} films · {count(home.library.series)} séries</span
								>
							{/if}
						</div>
					</div>
				</div>
			{:else}
				<div class="hero-body">
					<h1>Rien en lecture</h1>
				</div>
			{/if}
		</section>
	{:else}
		<h1 class="visually-hidden">Accueil</h1>
	{/if}
	<div class="content" class:bare={!showHero}>
		{#each rows as row (row)}
			{#if row === 'favorites'}
				{#if home.favorites.length > 0}
					<Shelf title="Favoris" id="fav">
						{#each home.favorites as service (service.slug)}
							<AppCard {service} />
						{/each}
					</Shelf>
				{:else if home.services.length > 0}
					<Shelf title="Services" id="services">
						{#each home.services as service (service.slug)}
							<AppCard {service} />
						{/each}
					</Shelf>
				{/if}
			{:else if row === 'latest'}
				{#if home.latest?.length}
					<Shelf title="Ajoutés récemment · Emby" id="latest" focusable>
						{#each home.latest as item, i (item.title + item.detail + i)}
							<PosterCard item={mediaPoster(item, home.latestSlug)} />
						{/each}
					</Shelf>
				{/if}
			{:else if row === 'upcoming'}
				{#if home.upcoming.length > 0}
					<Shelf title="À venir · Sonarr et Radarr" id="soon" focusable>
						{#each home.upcoming as item, i (item.title + item.detail + i)}
							<PosterCard item={upcomingPoster(item, new Date())} />
						{/each}
					</Shelf>
				{/if}
			{:else if row === 'downloads'}
				{#if home.downloads?.length}
					<section aria-labelledby="downloads">
						<h2 id="downloads">Téléchargements</h2>
						<div class="downloads">
							{#each home.downloads as card (card.slug)}
								<a
									class="download"
									class:stale={card.state !== 'ok'}
									href="{resolve('/services/[slug]', { slug: card.slug })}#widgets"
								>
									<span class="muted small">{card.label}</span>
									<strong>{card.headline}</strong>
									{#if card.state === 'ok'}
										<span class="muted">{card.note}</span>
									{:else}
										<span class="late"
											><span class="dot"></span>{widgetStateView(card.state).label}{card.dataAt
												? `, données ${ago(card.dataAt, Date.now())}`
												: ''}</span
										>
									{/if}
								</a>
							{/each}
						</div>
					</section>
				{/if}
			{:else if row === 'hosts'}
				{#if home.hosts.length > 0}
					<section aria-labelledby="infra">
						<h2 id="infra">Infrastructure</h2>
						<div class="hosts">
							{#each home.hosts as host (host.id)}
								<HostCard {host} />
							{/each}
						</div>
					</section>
				{/if}
			{/if}
		{/each}
	</div>
{:catch err}
	<section class="hero">
		<div class="hero-body">
			<ErrorPanel
				title="Le serveur Hub ne répond pas"
				detail={err instanceof Error ? err.message : String(err)}
				onretry={() => invalidateAll()}
			/>
		</div>
	</section>
{/await}

<style>
	.hero {
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		min-height: 480px;
		padding: calc(var(--shell-h) + 48px) var(--space-page) 48px;
		background:
			radial-gradient(120% 90% at 78% 30%, var(--hero-warm) 0%, transparent 60%),
			linear-gradient(180deg, var(--hero-top) 0%, var(--hero-mid) 55%, var(--bg) 100%);
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
		gap: 14px;
	}
	.hero-text {
		display: flex;
		flex-direction: column;
		gap: 14px;
		max-width: 640px;
	}
	.eyebrow {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		color: var(--accent-text);
		font-size: 15px;
		font-weight: 500;
	}
	.eyebrow.idle {
		color: var(--text-2);
	}
	h1 {
		margin: 0;
		font-size: var(--fs-hero);
		font-weight: 700;
		line-height: 1.02;
		letter-spacing: -0.025em;
	}
	.lead {
		margin: 0;
		font-size: 17px;
	}
	.progress {
		width: min(420px, 100%);
		height: 5px;
		overflow: hidden;
		border-radius: 99px;
		background: var(--track);
	}
	.progress span {
		display: block;
		height: 100%;
		background: var(--accent-play);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin-top: 10px;
	}
	.btn {
		display: inline-flex;
		align-items: center;
		min-height: 48px;
		padding: 0 22px;
		border-radius: var(--r-button);
		font-size: 16px;
		font-weight: 600;
	}
	.primary {
		background: var(--text);
		color: var(--text-inverse);
	}
	.sessions {
		align-self: center;
		font-size: 15px;
	}
	.others {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.others li {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 180px;
		padding: 14px 16px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--glass-scrim);
		backdrop-filter: var(--blur);
	}
	.small {
		font-size: var(--fs-small);
	}
	.content.bare {
		padding-top: calc(var(--shell-h) + 36px);
	}
	.content {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 8px var(--space-page) 64px;
	}
	h2 {
		margin: 0 0 18px;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.downloads {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(var(--card-min), 1fr));
		gap: 18px;
	}
	.download {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 18px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.download strong {
		font-size: 30px;
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	.stale {
		border-color: var(--tint-warn-border);
		background: var(--tint-warn-bg);
	}
	.late {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 99px;
		background: var(--status-degraded);
	}
	.hosts {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
	}
</style>
