<script lang="ts">
	import { cancelEnrollment, getHosts } from '#lib/api/client.ts';
	import type { AgentHost, HostsResponse } from '#lib/api/types.ts';
	import {
		agentStateView,
		ago,
		formatDay,
		hasAgent,
		minutesLeft,
		renewalDay
	} from '#lib/agents.ts';
	import EnrollDialog from './EnrollDialog.svelte';
	import HostContainers from './HostContainers.svelte';
	import RevokeDialog from './RevokeDialog.svelte';
	import Skeleton from './Skeleton.svelte';

	let data = $state<HostsResponse | null>(null);
	let error = $state('');
	let message = $state('');
	let now = $state(Date.now());
	let version = $state(0);
	let expanded = $state<Record<string, boolean>>({});
	let busy = $state<string | null>(null);
	/** Fenêtre d'enrôlement ouverte : `null` pour un nouvel hôte, sinon l'hôte à enrôler. */
	let enrolling = $state<{ host: AgentHost | null } | null>(null);
	let revoking = $state<AgentHost | null>(null);

	const reason = (err: unknown) => (err instanceof Error ? err.message : String(err));

	async function load() {
		try {
			data = await getHosts(fetch);
			error = '';
			version++;
		} catch (err) {
			error = `Impossible de charger les hôtes : ${reason(err)}`;
		}
	}

	// Relu toutes les 10 s, toutes les 3 s pendant un enrôlement pour voir l'agent arriver.
	$effect(() => {
		void load();
		const every = enrolling ? 3_000 : 10_000;
		const poll = setInterval(load, every);
		const tick = setInterval(() => (now = Date.now()), 1_000);
		return () => {
			clearInterval(poll);
			clearInterval(tick);
		};
	});

	const connected = $derived(
		data?.hosts.filter((h) => h.state === 'connected' || h.state === 'docker_down').length ?? 0
	);
	const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;
	/** Identifiant de l'hôte qui vient de recevoir un code, connu seulement après sa création. */
	let lastEnrolled = $state('');
	const live = $derived(
		enrolling && data ? (data.hosts.find((h) => h.id === lastEnrolled) ?? null) : null
	);

	function details(h: AgentHost): string[] {
		return [
			h.os,
			h.arch,
			h.dockerVersion && `Docker ${h.dockerVersion}`,
			h.agentVersion && `agent ${h.agentVersion}`
		].filter(Boolean);
	}

	function chip(h: AgentHost): string {
		const label = agentStateView(h.state).label;
		return h.state === 'connected' && h.lastSeenAt
			? `${label} · vu ${ago(h.lastSeenAt, now)}`
			: label;
	}

	function note(h: AgentHost): string {
		const n = plural(h.containers, 'conteneur', 'conteneurs');
		switch (h.state) {
			case 'connected':
				return h.certExpiresAt
					? `Certificat valide jusqu’au ${formatDay(h.certExpiresAt)} · renouvelé seul vers le ${renewalDay(h.certExpiresAt)}`
					: '';
			case 'docker_down':
				return `Agent vu ${h.lastSeenAt ? ago(h.lastSeenAt, now) : ''} · le proxy Docker ne répond pas. Ses ${n} sont affichés « inconnu ».`;
			case 'offline':
				return h.lastSeenAt
					? `Dernier contact ${ago(h.lastSeenAt, now)}. Ses services sont affichés « inconnu », pas « arrêté ».`
					: 'Enrôlé, mais l’agent n’a encore rien envoyé.';
			case 'pending':
				return h.enrollmentExpiresAt
					? `Code émis · expire dans ${minutesLeft(h.enrollmentExpiresAt, now)} min`
					: '';
			case 'revoked':
				return `Agent révoqué. ${h.containers ? `Ses ${n} restent dans l’inventaire.` : ''}`;
			case 'expired':
				return 'Certificat expiré sans renouvellement : enrôle l’agent à nouveau.';
			case 'none':
				// La pastille « Aucun agent » suffit.
				return '';
		}
	}

	function openEnroll(host: AgentHost | null) {
		message = '';
		lastEnrolled = host?.id ?? '';
		enrolling = { host };
	}

	async function cancel(h: AgentHost) {
		busy = h.id;
		message = '';
		try {
			await cancelEnrollment(fetch, h.id);
			message = `Code de ${h.name} annulé.`;
			await load();
		} catch (err) {
			message = `Impossible d’annuler le code : ${reason(err)}`;
		} finally {
			busy = null;
		}
	}
</script>

<div class="top">
	<div>
		<h2 id="hosts-title">Hôtes et agents</h2>
		{#if data}
			<p class="muted intro">
				{plural(data.hosts.length, 'hôte', 'hôtes')} · {plural(
					connected,
					'agent connecté',
					'agents connectés'
				)}
				{#if data.agentsEnabled}
					· les agents joignent <span class="mono">{data.serverUrl}</span> en TLS mutuel
				{/if}
			</p>
		{/if}
	</div>
	{#if data?.agentsEnabled}
		<button type="button" class="btn primary" onclick={() => openEnroll(null)}>
			<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
				<path d="M12 5v14M5 12h14" fill="none" stroke="currentColor" stroke-width="2" />
			</svg>
			Ajouter un hôte
		</button>
	{/if}
</div>

{#if data && !data.agentsEnabled}
	<p class="off" role="note">
		Port des agents éteint : le serveur démarre sans <span class="mono">HUB_AGENT_CA_FILE</span>.
		Les hôtes restent listés, mais aucun ne peut être enrôlé.
	</p>
{/if}

{#if data === null && !error}
	<div aria-busy="true" class="loading">
		<Skeleton height="64px" radius="14px" />
		<Skeleton height="64px" radius="14px" />
	</div>
{:else if data}
	<ul class="hosts">
		{#each data.hosts as h (h.id)}
			{@const view = agentStateView(h.state)}
			{@const listId = `containers-${h.id}`}
			<li class="host" class:hatched={view.hatched}>
				<div class="row">
					<div class="who">
						<div class="name">
							<span>{h.name}</span>
							{#if h.name !== h.id}<span class="mono muted id">{h.id}</span>{/if}
						</div>
						{#if details(h).length}
							<div class="muted small">{details(h).join(' · ')}</div>
						{/if}
					</div>
					<div class="state small">
						<span class="chip" style:--tone={view.color}
							><span class="dot" aria-hidden="true"></span>{chip(h)}</span
						>
						<span class="muted">{note(h)}</span>
					</div>
					<div class="row-actions">
						{#if h.state === 'pending' && data.agentsEnabled}
							<!-- Les noms accessibles commencent par le texte visible et ajoutent l'hôte :
							     la ligne le montre déjà à l'œil. -->
							<button
								type="button"
								class="btn ghost"
								aria-label="Nouveau code pour {h.name}"
								onclick={() => openEnroll(h)}>Nouveau code</button
							>
							<button
								type="button"
								class="btn ghost"
								aria-label="Annuler le code de {h.name}"
								disabled={busy === h.id}
								onclick={() => cancel(h)}>Annuler le code</button
							>
						{:else if !hasAgent(h.state) && data.agentsEnabled}
							<button
								type="button"
								class="btn ghost"
								aria-label="Enrôler {h.name}"
								onclick={() => openEnroll(h)}>Enrôler</button
							>
						{/if}
						{#if hasAgent(h.state) || h.containers > 0}
							<button
								type="button"
								class="btn ghost"
								class:open={expanded[h.id]}
								aria-expanded={!!expanded[h.id]}
								aria-controls={listId}
								aria-label={expanded[h.id]
									? `Masquer les conteneurs de ${h.name}`
									: `Conteneurs de ${h.name}`}
								onclick={() => (expanded[h.id] = !expanded[h.id])}
								>{expanded[h.id] ? 'Masquer' : 'Conteneurs'}</button
							>
						{/if}
					</div>
				</div>
				{#if expanded[h.id]}
					<HostContainers
						hostId={h.id}
						id={listId}
						{version}
						stale={h.state !== 'connected'}
						onrevoke={hasAgent(h.state) && data.agentsEnabled ? () => (revoking = h) : undefined}
					/>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

<p class="status" class:error={!!error} role={error ? 'alert' : 'status'}>{error || message}</p>
<p class="muted small rights">
	Droits d’un agent : liste des conteneurs, labels Compose et événements, à travers un proxy qui
	refuse tout le reste. Il ne peut ni démarrer, ni arrêter, ni lire les variables d’environnement.
	Révoquer coupe l’agent à sa requête suivante ; pour le reconnecter, il faut un nouveau code.
</p>

{#if enrolling}
	<EnrollDialog
		host={enrolling.host}
		{live}
		{now}
		onchange={() => {
			void load();
		}}
		onclose={() => (enrolling = null)}
		onenrolled={(id: string) => (lastEnrolled = id)}
	/>
{/if}
{#if revoking}
	<RevokeDialog
		host={revoking}
		onclose={() => (revoking = null)}
		ondone={(text) => {
			message = text;
			void load();
		}}
	/>
{/if}

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
		padding-bottom: 8px;
	}
	h2 {
		margin: 0 0 6px;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.intro {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
	}
	.small {
		font-size: var(--fs-small);
		line-height: 1.5;
	}
	.off {
		margin: 8px 0;
		padding: 14px 18px;
		border: 1px solid var(--tint-accent-border);
		border-radius: 16px;
		background: var(--tint-accent-bg);
		font-size: 14px;
		line-height: 1.5;
	}
	.loading {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 12px 0;
	}
	.hosts {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.host {
		border-top: 1px solid var(--border-1);
	}
	.host.hatched {
		margin: 0 -22px;
		padding: 0 22px;
		background: var(--hatch);
	}
	.row {
		display: grid;
		grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) auto;
		gap: 20px;
		align-items: center;
		padding: 18px 0;
	}
	.name {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 10px;
		margin-bottom: 3px;
		font-size: 17px;
		font-weight: 600;
	}
	.id {
		font-weight: 400;
	}
	.state {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 6px;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 4px 12px;
		border-radius: var(--r-pill);
		/* Teinte de l'état à 12 % derrière le texte, comme la maquette. */
		background: color-mix(in srgb, var(--tone) 12%, transparent);
		/* En clair, la couleur d'état est assombrie vers le texte pour rester à 4,5:1. */
		color: color-mix(in srgb, var(--tone) var(--tone-ink), var(--text));
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--tone);
	}
	.row-actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 8px;
	}
	.status {
		min-height: 1.5em;
		margin: 8px 0 0;
		color: var(--text-2);
		font-size: 14px;
	}
	.status.error {
		color: var(--text-danger);
	}
	.rights {
		margin: 4px 0 0;
	}
	.btn {
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: 8px;
		min-height: 44px;
		padding: 0 18px;
		border: 0;
		border-radius: 12px;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		background: var(--text);
		color: var(--text-inverse);
	}
	.ghost {
		border: 1px solid var(--border-2);
		background: transparent;
		color: var(--text);
	}
	.ghost.open {
		background: var(--surface-2);
	}
	@media (max-width: 900px) {
		.row {
			grid-template-columns: 1fr;
			gap: 10px;
		}
		.row-actions {
			justify-content: stretch;
		}
		.row-actions .btn {
			flex: 1;
			justify-content: center;
		}
	}
	@media (max-width: 720px) {
		.top .btn {
			width: 100%;
			justify-content: center;
		}
		.host.hatched {
			margin: 0 -18px;
			padding: 0 18px;
		}
	}
</style>
