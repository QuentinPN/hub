<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { RemovalPolicy } from '#lib/api/types.ts';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import TopNav from '#lib/components/TopNav.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Seules « Hôtes et agents » et « Découverte » ont du contenu ; le reste arrive par jalons.
	const sections = [
		{ label: 'Général', id: null },
		{ label: 'Hôtes et agents', id: 'hosts' },
		{ label: 'Découverte', id: 'discovery' },
		{ label: 'Widgets et connexions', id: null },
		{ label: 'Notifications', id: null },
		{ label: 'Apparence', id: null },
		{ label: 'Sécurité et accès', id: null },
		{ label: 'Sauvegarde', id: null }
	] as const;

	const rescanChoices = [5, 15, 60];
	const confirmChoices = [70, 80, 90];
	const removalLabel = (policy: RemovalPolicy, days: number) =>
		({
			absent_then_archive: `Marquer « absent », archiver après ${days} jours`,
			absent_only: 'Marquer « absent » sans archiver',
			archive_now: 'Archiver tout de suite'
		})[policy];
	const removalPolicies: RemovalPolicy[] = ['absent_then_archive', 'absent_only', 'archive_now'];
	const minutesLabel = (m: number) => (m === 60 ? 'Toutes les heures' : `Toutes les ${m} min`);
</script>

<svelte:head><title>Réglages · Hub</title></svelte:head>

<div class="page">
	<TopNav />
	<h1>Réglages</h1>

	<div class="layout">
		<nav class="sections" aria-label="Sections des réglages">
			{#each sections as s (s.label)}
				{#if s.id}
					<a class="sec" href="#{s.id}">{s.label}</a>
				{:else}
					<span class="sec" aria-disabled="true" title="Prochain jalon">{s.label}</span>
				{/if}
			{/each}
		</nav>

		<main class="panels">
			{#await data.settings}
				<div class="card" aria-busy="true">
					<Skeleton width="200px" height="22px" />
					<Skeleton height="64px" radius="14px" />
					<Skeleton height="64px" radius="14px" />
				</div>
			{:then settings}
				{#if !settings.editable}
					<p class="readonly" role="note">
						Lecture seule pour l'instant : l'enregistrement des réglages arrivera avec la base
						PostgreSQL. Les valeurs ci-dessous sont celles du serveur.
					</p>
				{/if}

				<section class="card" id="hosts" aria-labelledby="hosts-title">
					<h2 id="hosts-title">Hôtes et agents</h2>
					<p class="muted intro">
						Chaque hôte fait tourner un agent léger, en lecture seule, qui parle au serveur Hub en
						TLS mutuel.
					</p>
					<ul class="hosts">
						{#each settings.hosts as host (host.id)}
							<li class="host" class:unreachable={!host.reachable}>
								<div>
									<div class="host-name">
										<StatusDot status={host.reachable ? 'online' : 'unknown'} />{host.name}
									</div>
									<div class="muted small">
										{host.os} · {host.containers} conteneurs · agent
										<span class="mono">{host.agentVersion}</span>
										{host.arch}
									</div>
								</div>
								<div class="small">
									{#if host.reachable}
										<div>Connecté · vu {host.lastSeenLabel}</div>
										<div class="muted">Certificat valide jusqu'au {host.certExpiresLabel}</div>
									{:else}
										<div>Injoignable · vu {host.lastSeenLabel}</div>
										<div class="muted">Ses services sont affichés « inconnu »</div>
									{/if}
								</div>
							</li>
						{/each}
					</ul>
					<p class="muted small rights">
						Droits de l'agent : liste des conteneurs, labels Compose et événements. Il ne peut ni
						démarrer, ni arrêter, ni lire les variables d'environnement.
					</p>
				</section>

				<section class="card" id="discovery" aria-labelledby="discovery-title">
					<h2 id="discovery-title">Découverte</h2>
					<div class="set">
						<span class="lbl" id="events-label"
							>Suivre les événements Docker<small
								>Un nouveau conteneur apparaît en quelques secondes.</small
							></span
						>
						<button
							type="button"
							class="switch"
							role="switch"
							aria-checked={settings.discovery.followEvents}
							aria-labelledby="events-label"
							disabled={!settings.editable}><span></span></button
						>
					</div>
					<div class="set">
						<label for="rescan"
							>Rescan de réconciliation<small>Rattrape ce que les événements auraient manqué.</small
							></label
						>
						<select
							id="rescan"
							disabled={!settings.editable}
							value={settings.discovery.rescanMinutes}
						>
							{#each new Set([...rescanChoices, settings.discovery.rescanMinutes]) as m (m)}
								<option value={m}>{minutesLabel(m)}</option>
							{/each}
						</select>
					</div>
					<div class="set">
						<label for="removal"
							>Quand un service disparaît<small
								>Ses réglages, favoris et URL sont toujours conservés.</small
							></label
						>
						<select id="removal" disabled={!settings.editable} value={settings.discovery.onRemoval}>
							{#each removalPolicies as p (p)}
								<option value={p}>{removalLabel(p, settings.discovery.archiveAfterDays)}</option>
							{/each}
						</select>
					</div>
					<div class="set">
						<label for="confirm"
							>Demander confirmation si la confiance est sous<small
								>Sous ce seuil, la carte reste « à confirmer ».</small
							></label
						>
						<select
							id="confirm"
							disabled={!settings.editable}
							value={settings.discovery.confirmBelowPercent}
						>
							{#each new Set([...confirmChoices, settings.discovery.confirmBelowPercent]) as p (p)}
								<option value={p}>{p} %</option>
							{/each}
						</select>
					</div>
					<div class="set">
						<span class="lbl" id="infer-label"
							>Déduire une URL d'un port publié<small
								>Désactivé par sécurité : seuls le label <span class="mono">homelab.url</span> ou ta saisie
								font foi.</small
							></span
						>
						<button
							type="button"
							class="switch"
							role="switch"
							aria-checked={settings.discovery.inferUrlFromPorts}
							aria-labelledby="infer-label"
							disabled><span></span></button
						>
					</div>
				</section>
			{:catch err}
				<ErrorPanel
					title="Impossible de charger les réglages"
					detail={err instanceof Error ? err.message : String(err)}
					onretry={() => invalidateAll()}
				/>
			{/await}
		</main>
	</div>
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 36px;
		min-height: 100vh;
		padding: 28px var(--space-page) 64px;
		background: radial-gradient(80% 40% at 75% 0%, #1b2430 0%, rgba(7, 8, 10, 0) 70%), var(--bg);
	}
	h1 {
		margin: 0;
		font-size: clamp(2.5rem, 5vw, 3.5rem);
		font-weight: 700;
		letter-spacing: -0.025em;
	}
	.layout {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: 40px;
	}
	.sections {
		display: flex;
		flex: 1 1 280px;
		flex-direction: column;
		gap: 6px;
		max-width: 340px;
	}
	.sec {
		display: flex;
		align-items: center;
		min-height: 56px;
		padding: 0 18px;
		border-radius: 16px;
		font-size: 16px;
		font-weight: 500;
		transition:
			transform var(--dur) var(--ease),
			background var(--dur) var(--ease);
	}
	a.sec:hover,
	a.sec:focus-visible {
		background: rgba(255, 255, 255, 0.92);
		color: var(--text-inverse);
		box-shadow: 0 18px 36px rgba(0, 0, 0, 0.45);
		transform: scale(1.03);
		outline: none;
	}
	.sec[aria-disabled='true'] {
		color: var(--text-2);
		opacity: 0.55;
	}
	.panels {
		display: flex;
		flex: 999 1 560px;
		flex-direction: column;
		gap: 24px;
		min-width: 0;
	}
	.readonly {
		margin: 0;
		padding: 14px 18px;
		border: 1px solid rgba(232, 201, 166, 0.25);
		border-radius: 16px;
		background: rgba(232, 201, 166, 0.08);
		font-size: 14px;
		line-height: 1.5;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
		scroll-margin-top: 24px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.intro {
		margin: 0 0 6px;
		font-size: 14px;
	}
	.small {
		font-size: var(--fs-small);
	}
	.hosts {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.host {
		display: grid;
		grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
		gap: 16px;
		align-items: center;
		padding: 18px 0;
		border-top: 1px solid var(--border-1);
	}
	.host:first-child {
		border-top: 0;
	}
	.host.unreachable {
		margin: 0 -22px;
		padding: 18px 22px;
		background: var(--hatch);
	}
	.host-name {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 3px;
		font-size: 16px;
		font-weight: 600;
	}
	.rights {
		margin: 8px 0 0;
		line-height: 1.5;
	}
	.set {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 24px;
		padding: 16px 0;
		border-top: 1px solid var(--border-1);
	}
	h2 + .set {
		border-top: 0;
	}
	.set label,
	.lbl {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 15px;
	}
	small {
		color: var(--text-2);
		font-size: var(--fs-small);
	}
	select {
		min-height: 44px;
		max-width: 50%;
		padding: 0 12px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 14px;
	}
	select:disabled {
		opacity: 0.75;
	}
	.switch {
		position: relative;
		flex: none;
		width: 51px;
		height: 31px;
		border: 0;
		border-radius: 99px;
		background: var(--status-online);
		cursor: pointer;
	}
	.switch span {
		position: absolute;
		top: 2px;
		right: 2px;
		width: 27px;
		height: 27px;
		border-radius: 99px;
		background: #fff;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
	}
	.switch[aria-checked='false'] {
		background: rgba(255, 255, 255, 0.16);
	}
	.switch[aria-checked='false'] span {
		right: auto;
		left: 2px;
	}
	.switch:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	@media (max-width: 720px) {
		.page {
			gap: 20px;
		}
		.sections {
			display: none;
		}
		.card {
			padding: 18px;
		}
		.host {
			grid-template-columns: 1fr;
			gap: 6px;
		}
		.host.unreachable {
			margin: 0 -18px;
			padding: 18px;
		}
		.set {
			flex-wrap: wrap;
			gap: 10px;
		}
		select {
			max-width: 100%;
		}
	}
</style>
