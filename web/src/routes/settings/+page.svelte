<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import DiscoveryForm from '#lib/components/DiscoveryForm.svelte';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import PasskeysList from '#lib/components/PasskeysList.svelte';
	import PasswordForm from '#lib/components/PasswordForm.svelte';
	import SessionsList from '#lib/components/SessionsList.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import StatusDot from '#lib/components/StatusDot.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let sessionsVersion = $state(0);

	// Seules « Hôtes et agents », « Découverte » et « Sécurité et accès » ont du contenu ; le
	// reste arrive par jalons.
	const sections = [
		{ label: 'Général', id: null },
		{ label: 'Hôtes et agents', id: 'hosts' },
		{ label: 'Découverte', id: 'discovery' },
		{ label: 'Widgets et connexions', id: null },
		{ label: 'Notifications', id: null },
		{ label: 'Apparence', id: null },
		{ label: 'Sécurité et accès', id: 'security' },
		{ label: 'Sauvegarde', id: null }
	] as const;
</script>

<svelte:head><title>Réglages · Hub</title></svelte:head>

<div class="page">
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

		<div class="panels">
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
					<DiscoveryForm initial={settings.discovery} editable={settings.editable} />
				</section>
			{:catch err}
				<ErrorPanel
					title="Impossible de charger les réglages"
					detail={err instanceof Error ? err.message : String(err)}
					onretry={() => invalidateAll()}
				/>
			{/await}

			<section class="card" id="security" aria-labelledby="security-title">
				<h2 id="security-title">Sécurité et accès</h2>
				<p class="muted intro">
					Connecté en tant que <strong>{data.auth.username}</strong>. Chaque navigateur connecté
					ouvre une session de 30 jours, prolongée à chaque visite.
				</p>
				<h3 class="sub">Mot de passe</h3>
				<PasswordForm username={data.auth.username ?? ''} onchanged={() => sessionsVersion++} />
				<h3 class="sub">Passkeys</h3>
				<PasskeysList enabled={data.auth.passkeys} />
				<h3 class="sub">Sessions ouvertes</h3>
				<!-- Rechargée après un changement de mot de passe, qui ferme les autres sessions. -->
				{#key sessionsVersion}
					<SessionsList />
				{/key}
			</section>
			<p class="muted small credits">
				Icônes des services :
				<a href="https://selfh.st/icons" rel="noreferrer" target="_blank">selfh.st/icons</a>, sous
				licence CC BY 4.0. Les logos restent des marques de leurs éditeurs.
			</p>
		</div>
	</div>
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 36px;
		min-height: 100vh;
		padding: calc(var(--shell-h) + 36px) var(--space-page) 64px;
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
		/* Moitié de l'agrandissement des cartes, nul en mouvement réduit. */
		transform: scale(calc(1 + (var(--focus-scale) - 1) / 2));
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
	.sub {
		margin: 18px 0 4px;
		font-size: 16px;
		font-weight: 600;
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
	.credits {
		margin: 0;
		line-height: 1.5;
	}
	.credits a {
		text-decoration: underline;
	}
	.rights {
		margin: 8px 0 0;
		line-height: 1.5;
	}
	@media (max-width: 720px) {
		.page {
			gap: 20px;
			padding-top: calc(var(--shell-h) + 20px);
		}
		/* Rangée de pilules qui défile, comme les filtres du catalogue. Les sections sans contenu
		   n'y figurent pas : une pilule inerte prendrait la place d'une vraie. */
		.sections {
			flex: 1 1 100%;
			flex-direction: row;
			gap: 8px;
			max-width: 100%;
			margin: 0 calc(-1 * var(--space-page));
			padding: 2px var(--space-page);
			overflow-x: auto;
			scrollbar-width: none;
		}
		.sec {
			flex: none;
			min-height: 44px;
			padding: 0 14px;
			border: 1px solid rgba(255, 255, 255, 0.1);
			border-radius: var(--r-pill);
			background: var(--surface-1);
			font-size: 14px;
		}
		.sec[aria-disabled='true'] {
			display: none;
		}
		.layout {
			gap: 20px;
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
	}
</style>
