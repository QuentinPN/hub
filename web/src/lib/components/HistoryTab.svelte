<script module lang="ts">
	// Passe à true quand l'onglet est prêt : la fiche le rend alors cliquable.
	export const ready = true;
</script>

<script lang="ts">
	import { resolve } from '$app/paths';
	import { getServiceHistory } from '#lib/api/client.ts';
	import type { HistoryKind, ServiceDetail, ServiceHistory } from '#lib/api/types.ts';
	import {
		HISTORY_FILTERS,
		dayText,
		dayTone,
		eventView,
		groupByDay,
		shortDate,
		startText,
		summarize,
		duration,
		timeOf
	} from '#lib/history.ts';
	import ErrorPanel from './ErrorPanel.svelte';
	import Skeleton from './Skeleton.svelte';

	let { service }: { service: ServiceDetail } = $props();

	const PAGE = 25;
	let range = $state<30 | 90>(90);
	let kind = $state<HistoryKind | null>(null);
	let offset = $state(0);
	let history = $state<ServiceHistory | null>(null);
	let loading = $state(true);
	let error = $state('');
	let attempt = $state(0);
	let picked = $state<number | null>(null);

	$effect(() => {
		const slug = service.slug;
		const query = { kinds: kind ? [kind] : [], offset, limit: PAGE };
		void attempt;
		let stale = false;
		loading = true;
		error = '';
		getServiceHistory(fetch, slug, query)
			.then((h) => {
				if (stale) return;
				history = h;
			})
			.catch((err: unknown) => {
				if (!stale) error = err instanceof Error ? err.message : String(err);
			})
			.finally(() => {
				if (!stale) loading = false;
			});
		return () => {
			stale = true;
		};
	});

	const days = $derived(history ? history.days.slice(-range) : []);
	const summary = $derived(summarize(days));
	const selected = $derived(days.length ? Math.min(picked ?? days.length - 1, days.length - 1) : 0);
	const groups = $derived(history ? groupByDay(history.events, new Date()) : []);
	const allCount = $derived(
		history ? Object.values(history.counts).reduce((a, b) => a + (b ?? 0), 0) : 0
	);
	const measured = $derived(history?.days.some((d) => dayTone(d) !== 'none') ?? false);

	// La courbe de latence : un point par jour mesuré, à l'échelle du plus lent.
	const latency = $derived.by(() => {
		const values = days.map((d) => d.latencyMs);
		const known = values.filter((v): v is number => v !== null);
		if (!known.length) return null;
		const max = Math.max(...known, 1);
		const min = Math.min(...known);
		let path = '';
		values.forEach((v, i) => {
			if (v === null) return;
			const x = ((i + 0.5) * 1000) / values.length;
			const y = 60 - (v / max) * 54;
			path += `${path ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)} `;
		});
		return { path, min, max };
	});

	function pick(k: HistoryKind | null) {
		kind = k;
		offset = 0;
	}

	function onBarKey(e: KeyboardEvent) {
		const step = { ArrowLeft: -1, ArrowRight: 1, Home: -days.length, End: days.length }[e.key];
		if (step === undefined) return;
		e.preventDefault();
		picked = Math.max(0, Math.min(days.length - 1, selected + step));
	}

	function onBarPointer(e: PointerEvent) {
		const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const i = Math.floor(((e.clientX - box.left) / box.width) * days.length);
		picked = Math.max(0, Math.min(days.length - 1, i));
	}

	const filterEmpty = $derived(HISTORY_FILTERS.find((f) => f.id === kind)?.empty ?? '');
</script>

{#if !service.discovered}
	<section class="card">
		<p class="muted">
			Ce service n’est pas découvert : Hub ne garde l’historique que des services trouvés sur un
			hôte.
		</p>
	</section>
{:else if !history && error}
	<ErrorPanel title="Impossible de charger l’historique" detail={error} onretry={() => attempt++} />
{:else if !history}
	<div class="stack" aria-busy="true">
		<section class="card">
			<Skeleton width="180px" height="26px" radius="8px" />
			<div class="stats">
				<Skeleton height="74px" radius="16px" />
				<Skeleton height="74px" radius="16px" />
				<Skeleton height="74px" radius="16px" />
				<Skeleton height="74px" radius="16px" />
			</div>
			<Skeleton height="44px" radius="6px" />
			<Skeleton height="64px" radius="6px" />
		</section>
		<section class="card">
			<Skeleton width="150px" height="26px" radius="8px" />
			<Skeleton height="62px" radius="12px" />
			<Skeleton height="62px" radius="12px" />
			<Skeleton height="62px" radius="12px" />
		</section>
	</div>
{:else}
	<div class="stack">
		<section class="card" aria-labelledby="h-avail">
			<div class="head">
				<h2 id="h-avail">Disponibilité</h2>
				<div class="seg" role="group" aria-label="Période">
					<button type="button" aria-pressed={range === 30} onclick={() => (range = 30)}
						>30 jours</button
					>
					<button type="button" aria-pressed={range === 90} onclick={() => (range = 90)}
						>90 jours</button
					>
				</div>
			</div>
			<dl class="stats">
				<div>
					<dt>Disponible</dt>
					<dd class="tabular">{summary.uptime ?? '—'}</dd>
				</div>
				<div>
					<dt>Pannes</dt>
					<dd class="tabular">
						{summary.outages}{#if summary.downMinutes > 0}<span class="sub"
								>&nbsp;· {duration(summary.downMinutes)}</span
							>{/if}
					</dd>
				</div>
				<div>
					<dt>Latence moyenne</dt>
					<dd class="tabular">{summary.latencyMs !== null ? `${summary.latencyMs} ms` : '—'}</dd>
				</div>
				<div>
					<dt>Mesuré depuis</dt>
					<dd class="tabular">{summary.firstDate ? shortDate(summary.firstDate) : '—'}</dd>
				</div>
			</dl>
			<div
				class="bar"
				class:dense={days.length > 45}
				role="slider"
				tabindex="0"
				aria-label="Disponibilité par jour"
				aria-valuemin={0}
				aria-valuemax={days.length - 1}
				aria-valuenow={selected}
				aria-valuetext={days[selected] ? dayText(days[selected]) : ''}
				onkeydown={onBarKey}
				onpointermove={onBarPointer}
				onpointerdown={onBarPointer}
			>
				{#each days as d, i (d.date)}
					<span class="cell {dayTone(d)}" class:on={i === selected}></span>
				{/each}
			</div>
			<div class="axis muted">
				<span>Il y a {range} jours</span><span>Aujourd’hui</span>
			</div>
			{#if days[selected]}
				<p class="day" aria-hidden="true">{dayText(days[selected])}</p>
			{/if}
			{#if latency}
				<div class="lat">
					<div class="lat-head">
						<h3>Latence de l’adresse</h3>
						<span class="muted">moyenne par jour · {latency.min} à {latency.max} ms</span>
					</div>
					<svg
						viewBox="0 0 1000 64"
						preserveAspectRatio="none"
						role="img"
						aria-label="Latence moyenne par jour, de {latency.min} à {latency.max} ms"
						><line x1="0" y1="63" x2="1000" y2="63" class="base"></line><path
							d={latency.path}
							class="line"
						></path></svg
					>
				</div>
			{/if}
			<ul class="legend muted">
				<li><span class="cell ok"></span>Sans panne</li>
				<li><span class="cell warn"></span>Panne courte (≥ 95 %)</li>
				<li><span class="cell down"></span>Panne longue</li>
				<li><span class="cell unknown"></span>Inconnu (hôte injoignable)</li>
				<li><span class="cell none"></span>Pas encore mesuré</li>
			</ul>
		</section>

		<section class="card" aria-labelledby="h-events" aria-busy={loading}>
			<div class="head">
				<h2 id="h-events">Événements</h2>
				<div class="chips" role="group" aria-label="Filtrer par type">
					<button type="button" aria-pressed={kind === null} onclick={() => pick(null)}
						>Tout <span class="n tabular">{allCount}</span></button
					>
					{#each HISTORY_FILTERS as f (f.id)}
						<button type="button" aria-pressed={kind === f.id} onclick={() => pick(f.id)}
							>{f.label} <span class="n tabular">{history.counts[f.id] ?? 0}</span></button
						>
					{/each}
				</div>
			</div>

			{#if error}
				<ErrorPanel
					title="Impossible de charger l’historique"
					detail={error}
					onretry={() => attempt++}
				/>
			{/if}

			{#if history.total === 0 && kind === null && !measured}
				<div class="empty">
					<strong>L’historique commence au {startText(history.since)}</strong>
					<span class="muted"
						>Hub note désormais les pannes et les retours, les redémarrages et les arrêts des
						conteneurs, les changements de version, vos modifications et les alertes envoyées. Rien
						de plus ancien n’a été gardé.</span
					>
				</div>
			{:else if history.total === 0}
				<div class="no-match">
					<span
						><strong>{kind ? filterEmpty : 'Aucun événement'}</strong>
						<span class="muted">sur les {history.retentionDays} derniers jours.</span></span
					>
					{#if kind}
						<button type="button" class="btn" onclick={() => pick(null)}>Tout afficher</button>
					{/if}
				</div>
			{:else}
				<div class="groups" class:dim={loading}>
					{#each groups as g (g.label)}
						<div>
							<h3 class="group">{g.label}</h3>
							<ol class="events">
								{#each g.events as e (e.id)}
									{@const v = eventView(e)}
									<li>
										<span class="icon {v.tone}" aria-hidden="true">
											<svg
												width="18"
												height="18"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												stroke-width="2.2"
												stroke-linecap="round"
												stroke-linejoin="round"
											>
												{#if v.tone === 'down'}
													<path
														d="M12 8v5M12 16.5v.5M10.3 3.9 2.6 17.2A2 2 0 0 0 4.3 20h15.4a2 2 0 0 0 1.7-2.8L13.7 3.9a2 2 0 0 0-3.4 0Z"
													></path>
												{:else if v.tone === 'ok'}
													<path d="m5 12 5 5L20 7"></path>
												{:else if v.tone === 'crash'}
													<path d="M3 7.5 12 3l9 4.5v9L12 21l-9-4.5v-9ZM10 10l4 4M14 10l-4 4"
													></path>
												{:else if v.tone === 'container'}
													<path d="M3 7.5 12 3l9 4.5v9L12 21l-9-4.5v-9ZM3 7.5l9 4.5 9-4.5M12 12v9"
													></path>
												{:else if v.tone === 'version'}
													<path d="M12 3v12M7 10l5 5 5-5M5 21h14"></path>
												{:else if v.tone === 'edit'}
													<path d="M4 20h4L19 9l-4-4L4 16v4ZM13.5 6.5l4 4"></path>
												{:else}
													<path d="M6 16V11a6 6 0 1 1 12 0v5l2 2H4l2-2ZM10 21h4"></path>
												{/if}
											</svg>
										</span>
										<div class="text">
											<span class="title">{v.title}</span>
											<span class="detail muted">{v.detail}</span>
										</div>
										{#if v.badge}
											<span class="badge tabular">{v.badge}</span>
										{/if}
										<time class="time muted tabular" datetime={e.at}>{timeOf(e.at)}</time>
									</li>
								{/each}
							</ol>
						</div>
					{/each}
				</div>
				<nav class="pages" aria-label="Pages de l’historique">
					<span class="muted tabular"
						>{offset + 1} à {Math.min(offset + history.limit, history.total)} sur {history.total}
						événement{history.total > 1 ? 's' : ''}</span
					>
					<div class="page-btns">
						<button
							type="button"
							class="btn"
							disabled={offset === 0 || loading}
							onclick={() => (offset = Math.max(0, offset - PAGE))}>‹ Plus récents</button
						>
						<button
							type="button"
							class="btn"
							disabled={offset + PAGE >= history.total || loading}
							onclick={() => (offset += PAGE)}>Plus anciens ›</button
						>
					</div>
				</nav>
			{/if}
			<p class="muted foot">
				Hub garde {history.retentionDays} jours d’historique. Durée réglable dans
				<a href={resolve('/settings#general')}>Réglages › Général</a>.
			</p>
		</section>
	</div>
{/if}

<style>
	.stack {
		display: flex;
		flex-direction: column;
		gap: 24px;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 20px;
		padding: 28px 32px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	h3 {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}
	.seg {
		display: flex;
		gap: 2px;
		padding: 4px;
		border-radius: var(--r-pill);
		background: var(--surface-1);
	}
	.seg button,
	.chips button {
		min-height: 40px;
		padding: 0 16px;
		border: 1px solid transparent;
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text-2);
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.chips button {
		border-color: var(--border-2);
		background: var(--surface-1);
		color: var(--text);
	}
	.seg button[aria-pressed='true'],
	.chips button[aria-pressed='true'] {
		border-color: transparent;
		background: var(--fill-selected);
		color: var(--text-inverse);
	}
	.n {
		opacity: 0.75;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 12px;
		margin: 0;
	}
	.stats > div {
		padding: 14px 18px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-inset);
	}
	dt {
		color: var(--text-2);
		font-size: 14px;
	}
	dd {
		margin: 2px 0 0;
		font-size: 26px;
		font-weight: 600;
	}
	.sub {
		color: var(--text-2);
		font-size: 17px;
		font-weight: 500;
	}
	.bar {
		display: flex;
		gap: 3px;
		height: 44px;
		border-radius: 6px;
		cursor: crosshair;
		touch-action: pan-y;
	}
	.bar.dense {
		gap: 2px;
	}
	.bar:focus-visible {
		outline: 2px solid var(--border-focus);
		outline-offset: 4px;
	}
	.cell {
		display: block;
		flex: 1;
		min-width: 0;
		border-radius: 4px;
	}
	.bar.dense .cell {
		border-radius: 2px;
	}
	.cell.ok {
		background: var(--status-online);
	}
	.cell.warn {
		background: var(--status-degraded);
	}
	.cell.down {
		background: var(--status-down);
	}
	.cell.unknown {
		background: var(--hatch), var(--track-strong);
	}
	.cell.none {
		box-sizing: border-box;
		border: 1px dashed var(--border-dashed);
	}
	.cell.on {
		outline: 2px solid var(--text);
		outline-offset: 2px;
	}
	.axis {
		display: flex;
		justify-content: space-between;
		margin-top: -12px;
		font-size: 13px;
	}
	.day {
		margin: -6px 0 0;
		font-size: 14px;
	}
	.day::first-letter {
		text-transform: uppercase;
	}
	.lat {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.lat-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 4px 12px;
		font-size: 13px;
	}
	.lat-head h3 {
		font-size: 15px;
	}
	svg {
		display: block;
	}
	.lat svg {
		width: 100%;
		height: 64px;
	}
	.base {
		stroke: var(--track);
	}
	.line {
		fill: none;
		stroke: var(--accent-play);
		stroke-width: 2;
		vector-effect: non-scaling-stroke;
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 22px;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 13px;
	}
	.legend li {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.legend .cell {
		flex: none;
		width: 12px;
		height: 12px;
		border-radius: 3px;
	}
	.groups {
		display: flex;
		flex-direction: column;
		gap: 20px;
		transition: opacity var(--dur, 200ms);
	}
	.groups.dim {
		opacity: 0.55;
	}
	.group {
		margin-bottom: 6px;
		color: var(--text-2);
		font-size: 14px;
	}
	.group::first-letter {
		text-transform: uppercase;
	}
	.events {
		margin: 0;
		padding: 0;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-inset);
		list-style: none;
	}
	.events li {
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 14px 18px;
	}
	.events li + li {
		border-top: 1px solid var(--border-1);
	}
	.icon {
		display: flex;
		flex: none;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border-radius: 50%;
	}
	.icon.down {
		background: var(--tint-danger-bg-strong);
		color: var(--text-danger);
	}
	.icon.ok {
		background: var(--tint-ok-bg-strong);
		color: var(--text-ok);
	}
	.icon.crash {
		background: var(--tint-warn-bg-strong);
		color: var(--text-warn);
	}
	.icon.container {
		background: var(--surface-2);
		color: var(--hero-meta);
	}
	.icon.version,
	.icon.alert {
		background: var(--tint-accent-bg-strong);
		color: var(--accent-text);
	}
	.icon.edit {
		background: var(--surface-2);
		color: var(--text);
	}
	.text {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.title {
		font-weight: 600;
	}
	.detail {
		overflow-wrap: anywhere;
		font-size: 14px;
	}
	.badge {
		flex: none;
		padding: 4px 10px;
		border: 1px solid var(--tint-danger-border);
		border-radius: var(--r-pill);
		background: var(--tint-danger-bg-strong);
		color: var(--text-danger);
		font-size: 13px;
	}
	.time {
		flex: none;
		min-width: 44px;
		font-size: 14px;
		text-align: right;
	}
	.pages {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		font-size: 14px;
	}
	.page-btns {
		display: flex;
		gap: 10px;
	}
	.btn {
		min-height: 44px;
		padding: 0 18px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-button);
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		border-color: var(--border-1);
		background: transparent;
		color: var(--text-2);
		cursor: default;
		opacity: 0.6;
	}
	.empty {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 8px;
		padding: 28px;
		border: 1px dashed var(--border-dashed);
		border-radius: 16px;
		background: var(--surface-inset);
	}
	.empty strong {
		font-size: 17px;
	}
	.no-match {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 22px 24px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-inset);
	}
	.foot {
		margin: 0;
		font-size: 13px;
	}
	.foot a {
		color: var(--accent-text);
		text-decoration: underline;
	}
	@media (max-width: 720px) {
		.card {
			gap: 16px;
			padding: 20px 16px;
		}
		.stats {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 8px;
		}
		.stats > div {
			padding: 10px 12px;
		}
		dd {
			font-size: 20px;
		}
		.sub {
			font-size: 14px;
		}
		.bar {
			gap: 2px;
			height: 40px;
		}
		.bar.dense {
			gap: 1px;
		}
		.bar.dense .cell {
			border-radius: 1px;
		}
		.chips {
			flex-wrap: nowrap;
			margin: 0 -16px;
			padding: 0 16px 2px;
			overflow-x: auto;
		}
		.chips button {
			flex: none;
		}
		.events li {
			align-items: flex-start;
			flex-wrap: wrap;
			gap: 6px 12px;
			padding: 12px 14px;
		}
		.icon {
			width: 32px;
			height: 32px;
		}
		.text {
			flex-basis: calc(100% - 44px - 60px);
		}
		.badge {
			order: 5;
			margin-left: 44px;
		}
		.pages,
		.page-btns {
			flex-direction: column;
			align-items: stretch;
			width: 100%;
		}
		.page-btns {
			display: grid;
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
