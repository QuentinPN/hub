<script lang="ts">
	import { resolve } from '$app/paths';
	import { getMediaPage } from '#lib/api/client.ts';
	import type { MediaPage } from '#lib/api/types.ts';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import SectionPage from '#lib/components/SectionPage.svelte';
	import ServiceIcon from '#lib/components/ServiceIcon.svelte';
	import SessionList from '#lib/components/SessionList.svelte';
	import Shelf from '#lib/components/Shelf.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import { statusStyle } from '#lib/status.ts';
	import { mediaPoster, upcomingPoster } from '#lib/widgets.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;

	function lead(p: MediaPage): string {
		const parts = [
			p.sessions.length > 0
				? plural(p.sessions.length, 'lecture en cours', 'lectures en cours')
				: '',
			p.upcoming.length > 0 ? `${p.upcoming.length} à venir` : '',
			p.requests?.pendingApprovals
				? plural(p.requests.pendingApprovals, 'demande à approuver', 'demandes à approuver')
				: ''
		].filter(Boolean);
		return parts.join(' · ') || 'Rien en lecture';
	}
</script>

<SectionPage
	title="Médias"
	tint="#2a2340"
	loaded={data.page}
	reload={() => getMediaPage(fetch)}
	base={(p) => p}
	{lead}
>
	{#snippet children(page: MediaPage)}
		<div class="layout">
			<div class="column">
				<section class="card" aria-labelledby="play">
					<h2 id="play">En lecture <span class="muted">· Emby</span></h2>
					<SessionList sessions={page.sessions} />
				</section>
				{#if page.upcoming.length > 0}
					<Shelf title="À venir · Sonarr et Radarr" id="soon" focusable>
						{#each page.upcoming as item, i (item.title + item.detail + i)}
							<PosterCard item={upcomingPoster(item, new Date())} />
						{/each}
					</Shelf>
				{/if}
				{#if page.latest.length > 0}
					<div class="wide">
						<Shelf title="Ajoutés récemment · Emby" id="latest" focusable>
							{#each page.latest as item, i (item.title + item.detail + i)}
								<PosterCard item={mediaPoster(item, page.latestSlug)} />
							{/each}
						</Shelf>
					</div>
				{/if}
			</div>
			<div class="column">
				{#if page.requests}
					<section class="card" aria-labelledby="requests">
						<h2 id="requests">Demandes <span class="muted">· Request</span></h2>
						<strong class="big">{page.requests.pendingApprovals ?? '—'} à approuver</strong>
						<dl>
							<dt class="muted">Mes demandes en cours</dt>
							<dd class="tabular">{page.requests.mineActive}</dd>
							<dt class="muted">Signalements ouverts</dt>
							<dd class="tabular">{page.requests.openIssues}</dd>
						</dl>
						{#if page.requestsUrl}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- adresse externe -->
							<a class="btn" href={page.requestsUrl} target="_blank" rel="noopener noreferrer"
								>Ouvrir Request</a
							>
						{/if}
					</section>
				{/if}
				{#if page.library}
					<section class="card" aria-labelledby="library">
						<h2 id="library">Bibliothèque <span class="muted">· Emby</span></h2>
						<dl>
							<dt class="muted">Films</dt>
							<dd class="tabular">{page.library.movies}</dd>
							<dt class="muted">Séries</dt>
							<dd class="tabular">{page.library.series}</dd>
							<dt class="muted">Épisodes</dt>
							<dd class="tabular">{page.library.episodes}</dd>
						</dl>
					</section>
				{/if}
				{#if page.services.length > 0}
					<section class="services wide" aria-labelledby="services">
						<h2 id="services">Services</h2>
						{#each page.services as s (s.slug)}
							<a class="service" href={resolve('/services/[slug]', { slug: s.slug })}>
								<ServiceIcon icon={s.icon} monogram={s.monogram} color={s.color} size={44} />
								<strong>{s.name}</strong>
								<span class="muted state"
									><StatusDot status={s.status} size={8} />{statusStyle(s.status).label}</span
								>
							</a>
						{/each}
					</section>
				{/if}
			</div>
		</div>
	{/snippet}
</SectionPage>

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
		gap: 24px;
	}
	.column {
		display: flex;
		flex-direction: column;
		gap: 24px;
		min-width: 0;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	h2 .muted {
		font-weight: 400;
	}
	.big {
		font-size: 34px;
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	dl {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 10px 16px;
		margin: 0;
	}
	dd {
		margin: 0;
		text-align: right;
	}
	.btn {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		font-weight: 600;
	}
	.services {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.service {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 60px;
		box-sizing: border-box;
		padding: 8px 14px 8px 8px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-1);
	}
	.service strong {
		flex: 1;
		min-width: 0;
		font-weight: 600;
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		font-size: 14px;
	}
	@media (max-width: 960px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	@media (max-width: 720px) {
		.wide {
			display: none;
		}
	}
</style>
