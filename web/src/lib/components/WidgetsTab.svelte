<script lang="ts">
	import { getService } from '#lib/api/client.ts';
	import type { ServiceDetail } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import { onWidgetNews } from '#lib/live.ts';
	import {
		connectionLabel,
		mediaPoster,
		refreshLabel,
		speedLabel,
		upcomingPoster,
		widgetStateView
	} from '#lib/widgets.ts';
	import ConnectDialog from './ConnectDialog.svelte';
	import PosterCard from './PosterCard.svelte';
	import SessionList from './SessionList.svelte';
	import Shelf from './Shelf.svelte';

	let {
		service,
		onchange
	}: { service: ServiceDetail; onchange: (service: ServiceDetail) => void } = $props();

	const widget = $derived(service.widget);
	const conn = $derived(widget?.connection);
	const view = $derived(conn ? widgetStateView(conn.state) : null);
	const age = $derived.by(() =>
		conn?.dataAt ? `${conn.state === 'ok' ? 'à jour' : 'données'} ${ago(conn.dataAt, now)}` : ''
	);

	let connecting = $state(false);
	let now = $state(Date.now());

	// Relue dès que le serveur signale du nouveau ; la relecture régulière rattrape un signal perdu.
	$effect(() => {
		if (!service.widget?.connection) return;
		const slug = service.slug;
		const reload = () => {
			if (document.visibilityState === 'visible') {
				getService(fetch, slug)
					.then(onchange)
					.catch(() => {});
			}
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
						<dt class="muted">En attente</dt>
						<dd class="tabular">{conn.data.requests.pending}</dd>
						<dt class="muted">Approuvées</dt>
						<dd class="tabular">{conn.data.requests.approved}</dd>
						<dt class="muted">En cours</dt>
						<dd class="tabular">{conn.data.requests.processing}</dd>
						<dt class="muted">Disponibles</dt>
						<dd class="tabular">{conn.data.requests.available}</dd>
					</dl>
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
					<dd>lecture par Hub</dd>
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
	.ask {
		flex-flow: row wrap;
		align-items: center;
		justify-content: space-between;
		border: 1px dashed rgba(232, 201, 166, 0.4);
		background: rgba(232, 201, 166, 0.05);
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
		border-color: rgba(255, 214, 10, 0.3);
		background: rgba(255, 214, 10, 0.06);
	}
	.bad {
		border-color: rgba(255, 69, 58, 0.32);
		background: rgba(255, 69, 58, 0.07);
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
