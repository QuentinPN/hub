<script lang="ts">
	import { ApiError, deleteWidgetConnection, getService } from '#lib/api/client.ts';
	import type { ServiceDetail } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import { onWidgetNews } from '#lib/live.ts';
	import { durationLabel, episodeLabel, fourKShare, issueTypeLabel } from '#lib/request.ts';
	import {
		accountStateView,
		connectionLabel,
		mediaPoster,
		refreshLabel,
		speedLabel,
		upcomingPoster,
		widgetStateView
	} from '#lib/widgets.ts';
	import ConnectDialog from './ConnectDialog.svelte';
	import PosterCard from './PosterCard.svelte';
	import RequestPending from './RequestPending.svelte';
	import SessionList from './SessionList.svelte';
	import Shelf from './Shelf.svelte';
	import Sparkline from './Sparkline.svelte';
	import StatusPill from './StatusPill.svelte';

	let {
		service,
		onchange
	}: { service: ServiceDetail; onchange: (service: ServiceDetail) => void } = $props();

	const widget = $derived(service.widget);
	const conn = $derived(widget?.connection);
	const view = $derived(
		conn
			? widget?.manifest?.auth === 'password'
				? accountStateView(conn.state)
				: widgetStateView(conn.state)
			: null
	);
	const age = $derived.by(() =>
		conn?.dataAt ? `${conn.state === 'ok' ? 'à jour' : 'données'} ${ago(conn.dataAt, now)}` : ''
	);

	let connecting = $state(false);
	let now = $state(Date.now());
	let confirmRemove = $state(false);
	let removing = $state(false);
	let removeError = $state('');

	// Relire la connexion, sans attendre un signal : l'écriture qui vient de partir n'en promet pas.
	const refresh = () =>
		getService(fetch, service.slug)
			.then(onchange)
			.catch(() => {});

	// Une connexion dont Hub n'a plus le connecteur (Seerr) ne se lit plus : on ne peut que la retirer.
	async function removeConnection() {
		if (!confirmRemove) {
			confirmRemove = true;
			return;
		}
		removing = true;
		removeError = '';
		try {
			onchange(await deleteWidgetConnection(fetch, service.slug));
		} catch (err) {
			removeError = err instanceof ApiError ? err.message : String(err);
			removing = false;
		}
	}

	// Relue dès que le serveur signale du nouveau ; la relecture régulière rattrape un signal perdu.
	$effect(() => {
		if (!service.widget?.connection) return;
		const slug = service.slug;
		const reload = () => {
			if (document.visibilityState === 'visible') void refresh();
		};
		const poll = setInterval(reload, 30_000);
		const stop = onWidgetNews((news) => {
			if (news === slug) reload();
		});
		const tick = setInterval(() => (now = Date.now()), 5_000);
		return () => {
			clearInterval(poll);
			clearInterval(tick);
			stop();
		};
	});
</script>

{#if !widget}
	<section class="card empty" aria-labelledby="w-none">
		<h2 id="w-none">Aucun widget</h2>
		<p class="muted">Hub ne connaît pas l’API de ce service : il reste un lien, avec son état.</p>
	</section>
{:else if conn?.state === 'unsupported'}
	<section class="card empty" aria-labelledby="w-unsupported">
		<h2 id="w-unsupported">{widgetStateView('unsupported').label}</h2>
		<p class="muted">{widgetStateView('unsupported').detail}</p>
		<div>
			<button type="button" class="btn" disabled={removing} onclick={removeConnection}>
				{confirmRemove ? 'Confirmer : retirer la connexion' : 'Retirer la connexion'}
			</button>
		</div>
		<p class="error" role="alert">{removeError}</p>
	</section>
{:else if widget.support === 'planned'}
	<section class="card empty" aria-labelledby="w-planned">
		<h2 id="w-planned">Widget à venir</h2>
		<p class="muted">
			Son connecteur n’est pas encore écrit. Le service reste un lien, avec son état.
		</p>
	</section>
{:else if widget.support === 'link'}
	<section class="card empty" aria-labelledby="w-link">
		<h2 id="w-link">Lien seulement</h2>
		<p class="muted">Pas d’API confirmée pour ce service : Hub n’invente pas de widget.</p>
	</section>
{:else if !conn}
	<section class="card ask" aria-labelledby="w-ask">
		<div>
			<h2 id="w-ask">Pas connecté</h2>
			{#if service.url}
				<p class="muted">
					Avec une clé d’API, Hub lit {service.name} depuis le serveur et affiche ses widgets ici.
				</p>
			{:else}
				<p class="muted">Donne d’abord l’adresse du service, dans l’onglet Aperçu.</p>
			{/if}
		</div>
		{#if service.url}
			<button type="button" class="btn primary" onclick={() => (connecting = true)}
				>Connecter {service.name}</button
			>
		{/if}
	</section>
{:else if view}
	{#if view.tone !== 'ok'}
		<section class="note {view.tone}" aria-label="État du widget">
			<span class="dot" style:background={view.color}></span>
			<p>
				<strong>{view.label}</strong>
				<span class="muted">
					{view.detail}
					{#if conn.dataAt && (conn.state === 'unreachable' || conn.state === 'cert')}
						Données {ago(conn.dataAt, now)}.
					{/if}
				</span>
			</p>
			{#if view.tone === 'bad'}
				<button type="button" class="btn" onclick={() => (connecting = true)}
					>Corriger la connexion</button
				>
			{/if}
		</section>
	{/if}

	<div class="layout">
		<div class="column">
			{#if conn.data.sessions}
				<section class="card" aria-labelledby="w-sessions">
					<div class="head">
						<h2 id="w-sessions">Sessions en cours</h2>
						<span class="muted age">{age}</span>
					</div>
					<SessionList sessions={conn.data.sessions} />
				</section>
			{/if}
			{#if conn.data.calendar}
				{#if conn.data.calendar.length > 0}
					<Shelf title="À venir" id="w-calendar" focusable>
						{#each conn.data.calendar as item, i (item.title + item.detail + i)}
							<PosterCard item={upcomingPoster(item, new Date(), service.slug)} />
						{/each}
					</Shelf>
				{:else}
					<section class="card" aria-labelledby="w-calendar-empty">
						<h2 id="w-calendar-empty">À venir</h2>
						<p class="muted">Rien de prévu dans les 14 prochains jours.</p>
					</section>
				{/if}
			{/if}
			{#if conn.data.latest}
				{#if conn.data.latest.length > 0}
					<Shelf title="Ajoutés récemment" id="w-latest" focusable>
						{#each conn.data.latest as item, i (item.title + item.detail + i)}
							<PosterCard item={mediaPoster(item, service.slug)} />
						{/each}
					</Shelf>
				{:else}
					<section class="card" aria-labelledby="w-latest-empty">
						<h2 id="w-latest-empty">Ajoutés récemment</h2>
						<p class="muted">Rien d’ajouté récemment.</p>
					</section>
				{/if}
			{/if}
			{#if conn.data.pending}
				<RequestPending slug={service.slug} pending={conn.data.pending} onwrite={refresh} />
			{/if}
			{#if conn.data.recent}
				<section class="card" aria-labelledby="w-recent">
					<h2 id="w-recent">Suivi des demandes</h2>
					{#each conn.data.recent.items as r (r.id)}
						<div class="entry" class:flagged={r.status === 'failed'}>
							<div class="entry-main">
								<strong>{r.title}</strong>
								<span class="muted">{r.requestedBy} · {ago(r.updatedAt, now)}</span>
							</div>
							<StatusPill status={r.status} />
							{#if r.reason}
								<p class="reason" class:failed={r.status === 'failed'}>{r.reason}</p>
							{/if}
						</div>
					{:else}
						<p class="muted">Aucune demande pour l’instant.</p>
					{/each}
				</section>
			{/if}
			{#if conn.data.issues}
				<section class="card" aria-labelledby="w-issues">
					<h2 id="w-issues">Signalements</h2>
					{#each conn.data.issues.items as issue (issue.id)}
						<div class="entry">
							<div class="entry-main">
								<strong>{issue.title}</strong>
								<span class="muted">
									{[
										issueTypeLabel(issue.type),
										episodeLabel(issue.season, issue.episode),
										issue.createdBy
									]
										.filter(Boolean)
										.join(' · ')}
								</span>
							</div>
						</div>
					{:else}
						<p class="muted">Aucun signalement ouvert.</p>
					{/each}
					{#if conn.data.issues.total > conn.data.issues.items.length}
						<p class="muted">
							Et {conn.data.issues.total - conn.data.issues.items.length} de plus dans Request.
						</p>
					{/if}
				</section>
			{/if}
		</div>
		<div class="column side">
			{#if conn.data.systems}
				<section class="card" aria-labelledby="w-systems">
					<h2 id="w-systems">Systèmes</h2>
					{#each conn.data.systems as sys (sys.name)}
						<div class="system">
							<strong>{sys.name}</strong>
							{#if sys.status === 'up'}
								<span class="muted tabular"
									>CPU {sys.cpu} % · Mémoire {sys.memory} % · Disque {sys.disk} %</span
								>
							{:else}
								<span class="muted">Hors ligne</span>
							{/if}
						</div>
					{:else}
						<p class="muted">Aucun système visible : partage-les avec ce compte dans Beszel.</p>
					{/each}
				</section>
			{/if}
			{#if conn.data.requests}
				<section class="card" aria-labelledby="w-requests">
					<h2 id="w-requests">Demandes</h2>
					<dl>
						<dt class="muted">À approuver</dt>
						<dd class="tabular">{conn.data.requests.pendingApprovals ?? '—'}</dd>
						<dt class="muted">Mes demandes en cours</dt>
						<dd class="tabular">{conn.data.requests.mineActive}</dd>
						<dt class="muted">Signalements ouverts</dt>
						<dd class="tabular">{conn.data.requests.openIssues}</dd>
					</dl>
				</section>
			{/if}
			{#if conn.data.stats}
				{@const stats = conn.data.stats}
				{@const share = fourKShare(stats.byQuality)}
				<section class="card" aria-labelledby="w-stats">
					<h2 id="w-stats">Statistiques</h2>
					<strong class="big tabular">{stats.total.toLocaleString('fr-FR')} demandes</strong>
					<Sparkline byMonth={stats.byMonth} />
					<span class="muted caption">{stats.byMonth.length} derniers mois</span>
					<dl>
						<dt class="muted">Délai médian jusqu’à disponible</dt>
						<dd class="tabular">{durationLabel(stats.medianAvailableSeconds)}</dd>
						<dt class="muted">Délai médian d’approbation</dt>
						<dd class="tabular">{durationLabel(stats.medianApprovalSeconds)}</dd>
						<dt class="muted">Films</dt>
						<dd class="tabular">{stats.byType.movie}</dd>
						<dt class="muted">Séries</dt>
						<dd class="tabular">{stats.byType.tv}</dd>
						<dt class="muted">En 4K</dt>
						<dd class="tabular">{share === null ? '—' : `${share} %`}</dd>
					</dl>
					{#if stats.topMembers.length > 0}
						<h3 id="w-members">Membres les plus actifs</h3>
						<ol class="members" aria-labelledby="w-members">
							{#each stats.topMembers.slice(0, 5) as member, i (i)}
								<li>
									<span>{member.name}</span>
									<span class="tabular muted">{member.count}</span>
								</li>
							{/each}
						</ol>
					{/if}
				</section>
			{/if}
			{#if conn.data.indexers}
				<section class="card" aria-labelledby="w-indexers">
					<h2 id="w-indexers">Indexeurs</h2>
					<dl>
						<dt class="muted">Activés</dt>
						<dd class="tabular">{conn.data.indexers.enabled}</dd>
						<dt class="muted">En panne</dt>
						<dd class="tabular">{conn.data.indexers.failing}</dd>
					</dl>
				</section>
			{/if}
			{#if conn.data.transfer}
				<section class="card" aria-labelledby="w-transfer">
					<h2 id="w-transfer">Transferts</h2>
					<dl>
						<dt class="muted">Réception</dt>
						<dd class="tabular">{speedLabel(conn.data.transfer.downSpeed)}</dd>
						<dt class="muted">Envoi</dt>
						<dd class="tabular">{speedLabel(conn.data.transfer.upSpeed)}</dd>
						<dt class="muted">En cours</dt>
						<dd class="tabular">{conn.data.transfer.downloading}</dd>
						<dt class="muted">Réseau</dt>
						<dd>{connectionLabel(conn.data.transfer.connection)}</dd>
					</dl>
				</section>
			{/if}
			{#if conn.data.queue}
				<section class="card" aria-labelledby="w-queue">
					<h2 id="w-queue">File d’attente</h2>
					<dl>
						<dt class="muted">En tout</dt>
						<dd class="tabular">{conn.data.queue.total}</dd>
						<dt class="muted">En cours</dt>
						<dd class="tabular">{conn.data.queue.downloading}</dd>
						<dt class="muted">À vérifier</dt>
						<dd class="tabular">{conn.data.queue.problems}</dd>
					</dl>
				</section>
			{/if}
			{#if conn.data.library}
				<section class="card" aria-labelledby="w-library">
					<h2 id="w-library">Bibliothèque</h2>
					<dl>
						<dt class="muted">Films</dt>
						<dd class="tabular">{conn.data.library.movies}</dd>
						<dt class="muted">Séries</dt>
						<dd class="tabular">{conn.data.library.series}</dd>
						<dt class="muted">Épisodes</dt>
						<dd class="tabular">{conn.data.library.episodes}</dd>
					</dl>
				</section>
			{/if}
			<section class="card" aria-labelledby="w-conn">
				<h2 id="w-conn">Connexion du widget</h2>
				<dl>
					<dt class="muted">État</dt>
					<dd>{view.label}</dd>
					{#if conn.keyHint}
						{#if widget.manifest?.auth === 'password'}
							<dt class="muted">Compte</dt>
							<dd>{conn.keyHint}</dd>
						{:else}
							<dt class="muted">Clé d’API</dt>
							<dd>•••• {conn.keyHint}</dd>
						{/if}
					{/if}
					<dt class="muted">Rafraîchi</dt>
					<dd>{refreshLabel(conn.refreshSeconds).toLowerCase()}</dd>
					<dt class="muted">Accès</dt>
					<dd>{widget.manifest?.writes ? 'lecture et actions par Hub' : 'lecture par Hub'}</dd>
				</dl>
				<button type="button" class="btn" onclick={() => (connecting = true)}
					>Gérer la connexion</button
				>
			</section>
		</div>
	</div>
{/if}

{#if connecting && widget?.manifest}
	<ConnectDialog
		{service}
		manifest={widget.manifest}
		onclose={() => (connecting = false)}
		onsaved={onchange}
	/>
{/if}

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
		gap: 22px;
	}
	.column {
		display: flex;
		flex-direction: column;
		gap: 22px;
		min-width: 0;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.error {
		color: var(--text-danger);
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	.ask {
		flex-flow: row wrap;
		align-items: center;
		justify-content: space-between;
		border: 1px dashed var(--tint-accent-border-strong);
		background: var(--tint-accent-bg);
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	p {
		margin: 4px 0 0;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 4px 12px;
	}
	.age {
		font-size: 14px;
	}
	.entry {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 6px 12px;
		padding-top: 10px;
		border-top: 1px solid var(--border-1);
	}
	.entry-main {
		display: flex;
		flex: 1 1 200px;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.entry-main strong {
		font-weight: 600;
	}
	.reason {
		flex: 1 1 100%;
		margin: 0;
		color: var(--text-2);
		font-size: 14px;
		overflow-wrap: anywhere;
	}
	/* Un échec se voit sans lire : liseré et motif en rouge, en plus de l'étiquette. */
	.flagged {
		padding: 10px 12px;
		border: 1px solid var(--tint-danger-border);
		border-radius: 14px;
		background: var(--tint-danger-bg);
	}
	.reason.failed {
		color: var(--text-danger);
	}
	.big {
		font-size: 28px;
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	.caption {
		margin-top: -8px;
		font-size: 13px;
	}
	h3 {
		margin: 6px 0 0;
		font-size: 16px;
		font-weight: 600;
	}
	.members {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.members li {
		display: flex;
		justify-content: space-between;
		gap: 12px;
	}
	.system {
		display: flex;
		flex-direction: column;
		gap: 3px;
		padding-top: 10px;
		border-top: 1px solid var(--border-1);
	}
	.system strong {
		font-weight: 600;
		overflow-wrap: anywhere;
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
	.note {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
		padding: 14px 16px;
		border: 1px solid var(--border-1);
		border-radius: 18px;
		background: var(--surface-1);
	}
	.note p {
		display: flex;
		flex: 1 1 280px;
		flex-direction: column;
		gap: 2px;
		margin: 0;
	}
	.note strong {
		font-weight: 600;
	}
	.warn {
		border-color: var(--tint-warn-border);
		background: var(--tint-warn-bg);
	}
	.bad {
		border-color: var(--tint-danger-border);
		background: var(--tint-danger-bg);
	}
	.dot {
		flex: none;
		width: 10px;
		height: 10px;
		border-radius: 99px;
	}
	.btn {
		min-height: 48px;
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
	@media (max-width: 960px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
