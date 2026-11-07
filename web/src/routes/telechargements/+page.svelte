<script lang="ts">
	import { resolve } from '$app/paths';
	import { getDownloadsPage } from '#lib/api/client.ts';
	import type { DownloadsPage } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import SectionPage from '#lib/components/SectionPage.svelte';
	import {
		connectionLabel,
		etaLabel,
		queueStateLabel,
		speedLabel,
		torrentStateLabel,
		widgetStateView
	} from '#lib/widgets.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	function lead(p: DownloadsPage): string {
		if (!p.transfer) return `${p.queue.length} en file d’attente`;
		const parts = [`${p.transfer.downloading} en cours`];
		if (p.transfer.connection !== 'connected') parts.push(connectionLabel(p.transfer.connection));
		return parts.join(' · ');
	}
</script>

<SectionPage
	title="Téléchargements"
	tint="#1b3448"
	loaded={data.page}
	reload={() => getDownloadsPage(fetch)}
	base={(p) => p}
	{lead}
>
	{#snippet head(page: DownloadsPage)}
		{#if page.transfer}
			<div class="speeds">
				<span
					><strong class="tabular">{speedLabel(page.transfer.downSpeed)}</strong><span class="muted"
						>Réception · qBittorrent</span
					></span
				>
				<span
					><strong class="tabular">{speedLabel(page.transfer.upSpeed)}</strong><span class="muted"
						>Envoi</span
					></span
				>
			</div>
		{/if}
	{/snippet}
	{#snippet children(page: DownloadsPage)}
		<div class="layout">
			<div class="column">
				{#if page.transfer}
					<section class="card" aria-labelledby="active">
						<h2 id="active">
							En cours <span class="muted">· qBittorrent sur {page.transferHost}</span>
						</h2>
						{#each page.torrents as t, i (t.name + i)}
							<div class="torrent">
								<span class="who">
									<strong>{t.name}</strong>
									<span class="muted small">{torrentStateLabel(t.state)}</span>
								</span>
								<span
									class="progress"
									role="progressbar"
									aria-label="Progression de {t.name}"
									aria-valuenow={Math.round(t.progress * 100)}
									aria-valuemin={0}
									aria-valuemax={100}><span style:width="{t.progress * 100}%"></span></span
								>
								<span class="tabular speed">{speedLabel(t.downSpeed)}</span>
								<span class="muted tabular eta">{etaLabel(t.eta)}</span>
							</div>
						{:else}
							<p class="muted">Aucun téléchargement en cours.</p>
						{/each}
					</section>
				{/if}
				{#if page.queue.length > 0}
					<section class="card wide" aria-labelledby="queue">
						<h2 id="queue">File d’attente <span class="muted">· Sonarr et Radarr</span></h2>
						{#each page.queue as q, i (q.title + q.detail + i)}
							<div class="item">
								<span class="who">
									<strong>{q.title}</strong>
									<span class="muted small">{q.detail}</span>
								</span>
								<span class="muted state">
									<span class="dot" class:live={q.status === 'downloading'} class:warn={q.problem}
									></span>
									{q.problem ? 'À vérifier' : queueStateLabel(q.status)}
								</span>
							</div>
						{/each}
					</section>
				{/if}
			</div>
			<div class="column">
				{#each page.cards as card (card.label)}
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
							<span class="muted"
								>{widgetStateView(card.state).label}{card.dataAt
									? `, données ${ago(card.dataAt, Date.now())}`
									: ''}</span
							>
						{/if}
					</a>
				{/each}
			</div>
		</div>
	{/snippet}
</SectionPage>

<style>
	.speeds {
		display: flex;
		flex-wrap: wrap;
		gap: 16px 32px;
	}
	.speeds > span {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.speeds strong {
		font-size: clamp(1.75rem, 3vw, 2.4rem);
		font-weight: 700;
		letter-spacing: -0.02em;
	}
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
	p {
		margin: 0;
	}
	.torrent {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 180px 100px 70px;
		align-items: center;
		gap: 18px;
		padding-top: 12px;
		border-top: 1px solid var(--border-1);
	}
	.item {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 6px 18px;
		padding-top: 12px;
		border-top: 1px solid var(--border-1);
	}
	.who {
		display: flex;
		flex: 1 1 200px;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}
	.who strong {
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.small {
		font-size: 14px;
	}
	.progress {
		display: block;
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
	.speed,
	.eta {
		text-align: right;
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}
	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 99px;
		background: var(--status-unknown);
	}
	.dot.live {
		background: var(--status-online);
	}
	.dot.warn {
		background: var(--status-degraded);
	}
	.download {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 20px;
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
	@media (max-width: 960px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	@media (max-width: 720px) {
		.wide {
			display: none;
		}
		.torrent {
			grid-template-columns: minmax(0, 1fr) auto auto;
			gap: 8px 12px;
		}
		.torrent .who,
		.torrent .progress {
			grid-column: 1 / -1;
		}
		.torrent .speed {
			text-align: left;
		}
	}
</style>
