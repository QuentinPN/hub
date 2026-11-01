<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AppCard from '#lib/components/AppCard.svelte';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import HostCard from '#lib/components/HostCard.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import Shelf from '#lib/components/Shelf.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import { shell } from '#lib/shell.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

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
{:then home}
	{@const main = home.sessions[0]}
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
						<a class="btn primary" href={resolve('/services/[slug]', { slug: 'emby' })}
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
		{:else}
			<div class="hero-body">
				<h1>Rien en lecture</h1>
			</div>
		{/if}
	</section>

	<div class="content">
		<Shelf title="Favoris" id="fav">
			{#each home.favorites as service (service.slug)}
				<AppCard {service} />
			{/each}
		</Shelf>
		<Shelf title="À venir · Sonarr et Radarr" id="soon" focusable>
			{#each home.upcoming as item (item.title + item.detail)}
				<PosterCard {item} />
			{/each}
		</Shelf>
		<section aria-labelledby="infra">
			<h2 id="infra">Infrastructure</h2>
			<div class="hosts">
				{#each home.hosts as host (host.id)}
					<HostCard {host} />
				{/each}
			</div>
		</section>
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
			radial-gradient(120% 90% at 78% 30%, #3a2a1c 0%, rgba(58, 42, 28, 0) 60%),
			linear-gradient(180deg, #14202b 0%, #0c1218 55%, var(--bg) 100%);
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
		color: var(--accent-play);
		font-size: 15px;
		font-weight: 500;
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
		background: rgba(255, 255, 255, 0.12);
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
		background: rgba(0, 0, 0, 0.35);
		backdrop-filter: var(--blur);
	}
	.small {
		font-size: var(--fs-small);
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
	.hosts {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
	}
</style>
