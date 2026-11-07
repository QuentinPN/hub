<script lang="ts">
	import { getGeneral, saveGeneral } from '#lib/api/client.ts';
	import type { General, GeneralSettings } from '#lib/api/types.ts';
	import {
		browserTimeZone,
		displayNameError,
		DISPLAY_NAME_MAX,
		HISTORY_RETENTIONS,
		REFRESH_MODES,
		START_PAGES,
		timeZoneChoices
	} from '#lib/general.ts';
	import { shell } from '#lib/shell.svelte.ts';
	import Skeleton from './Skeleton.svelte';

	let settings = $state<GeneralSettings | null>(null);
	let loadError = $state('');
	let message = $state<{ ok: boolean; text: string } | null>(null);
	let name = $state('');

	const errText = (err: unknown) => (err instanceof Error ? err.message : String(err));

	$effect(() => {
		getGeneral(fetch)
			.then((s) => {
				settings = s;
				name = s.displayName;
			})
			.catch((err: unknown) => (loadError = `Impossible de lire les réglages : ${errText(err)}`));
	});

	const nameError = $derived(displayNameError(name));
	const browserZone = browserTimeZone();
	const zones = $derived(
		settings ? timeZoneChoices(settings.timeZone, settings.about.serverTimeZone) : []
	);

	// Chaque choix s'enregistre seul, comme les réglages de la sauvegarde ; le nom, quand on
	// quitte le champ ou valide avec Entrée.
	async function save(change: Partial<General>) {
		if (!settings) return;
		message = null;
		const body: General = {
			displayName: settings.displayName,
			startPage: settings.startPage,
			timeZone: settings.timeZone,
			widgetRefresh: settings.widgetRefresh,
			historyDays: settings.historyDays,
			...change
		};
		try {
			settings = await saveGeneral(fetch, body);
			name = settings.displayName;
			shell.displayName = settings.displayName;
			shell.startPage = settings.startPage;
			shell.widgetRefresh = settings.widgetRefresh;
			message = { ok: true, text: 'Enregistré.' };
		} catch (err) {
			message = { ok: false, text: `Échec de l'enregistrement : ${errText(err)}` };
		}
	}

	function saveName() {
		if (!settings || nameError || name.trim() === settings.displayName) return;
		void save({ displayName: name.trim() });
	}

	const zoneLabel = (z: string) => (z === browserZone ? `${z} (celui du navigateur)` : z);

	type Row = { label: string; value: string; unset?: boolean };
	const rows = $derived.by((): Row[] => {
		if (!settings) return [];
		const a = settings.about;
		return [
			{ label: 'Version', value: a.version || 'Inconnue', unset: !a.version },
			{ label: 'Adresse de Hub', value: a.origin || 'Non définie (HUB_ORIGIN)', unset: !a.origin },
			{
				label: 'Adresse des agents',
				value: a.agentUrl || 'Écoute des agents arrêtée (HUB_AGENT_CA_FILE absent)',
				unset: !a.agentUrl
			},
			{ label: 'Hôte', value: a.host || 'Non renseigné (HUB_HOST_NAME)', unset: !a.host }
		];
	});
</script>

<section class="card" id="general" aria-labelledby="general-title" aria-busy={!settings}>
	<h2 id="general-title">Général</h2>
	{#if loadError}
		<p class="error" role="alert">{loadError}</p>
	{:else if !settings}
		<div class="grid">
			<Skeleton height="76px" radius="14px" />
			<Skeleton height="76px" radius="14px" />
			<Skeleton height="76px" radius="14px" />
			<Skeleton height="76px" radius="14px" />
		</div>
	{:else}
		<div class="grid">
			<label
				>Nom affiché
				<input
					type="text"
					autocomplete="off"
					maxlength={DISPLAY_NAME_MAX * 2}
					aria-invalid={!!nameError}
					aria-describedby="general-name-help"
					bind:value={name}
					onchange={saveName}
				/>
				<small id="general-name-help" class:error={nameError}
					>{nameError || 'Dans l’onglet du navigateur et l’écran de connexion.'}</small
				>
			</label>
			<label
				>Page d’ouverture
				<select
					value={settings.startPage}
					onchange={(e) => save({ startPage: e.currentTarget.value as General['startPage'] })}
				>
					{#each START_PAGES as p (p.value)}
						<option value={p.value}>{p.label}</option>
					{/each}
				</select>
			</label>
			<label
				>Fuseau horaire
				<select
					value={settings.timeZone}
					aria-describedby="general-zone-help"
					onchange={(e) => save({ timeZone: e.currentTarget.value })}
				>
					<option value=""
						>Celui du serveur{settings.about.serverTimeZone
							? ` (${settings.about.serverTimeZone})`
							: ''}</option
					>
					{#each zones as z (z)}
						<option value={z}>{zoneLabel(z)}</option>
					{/each}
				</select>
				<small id="general-zone-help">Pour les horaires des sauvegardes et des heures calmes.</small
				>
			</label>
			<label
				>Rafraîchissement des widgets
				<select
					value={settings.widgetRefresh}
					onchange={(e) =>
						save({ widgetRefresh: e.currentTarget.value as General['widgetRefresh'] })}
				>
					{#each REFRESH_MODES as m (m.value)}
						<option value={m.value}>{m.label}</option>
					{/each}
				</select>
			</label>
			<label
				>Historique des services
				<select
					value={settings.historyDays}
					aria-describedby="general-history-help"
					onchange={(e) =>
						save({ historyDays: Number(e.currentTarget.value) as General['historyDays'] })}
				>
					{#each HISTORY_RETENTIONS as r (r.value)}
						<option value={r.value}>Conserver {r.label}</option>
					{/each}
				</select>
				<small id="general-history-help"
					>Pannes, événements et disponibilité. Raccourcir efface aussitôt le plus ancien.</small
				>
			</label>
		</div>
	{/if}
	<p class="status" class:error={message && !message.ok} role="status">{message?.text ?? ''}</p>
</section>

{#if settings}
	<section class="card about" id="about" aria-labelledby="about-title">
		<h2 id="about-title">À propos</h2>
		<dl>
			{#each rows as r (r.label)}
				<div class="row">
					<dt>{r.label}</dt>
					<dd class:unset={r.unset}>{r.value}</dd>
				</div>
			{/each}
		</dl>
		<p class="muted note">
			Ces valeurs viennent du fichier <code>.env</code> de l’hôte : elles se changent là, puis au redémarrage
			de Hub.
		</p>
	</section>
{/if}

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: 18px;
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
	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 7px;
		font-size: 15px;
		font-weight: 500;
	}
	input,
	select {
		box-sizing: border-box;
		width: 100%;
		min-height: 48px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 14px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-weight: 400;
	}
	select {
		padding: 0 12px;
	}
	input:focus-visible,
	select:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 1px;
	}
	input[aria-invalid='true'] {
		border-color: var(--text-danger);
	}
	small {
		color: var(--text-2);
		font-size: 14px;
		font-weight: 400;
	}
	.error {
		color: var(--text-danger);
	}
	.status {
		min-height: 1.4em;
		margin: -6px 0 0;
		color: var(--status-online);
		font-size: 14px;
	}
	.status:empty {
		display: none;
	}
	.status.error {
		color: var(--text-danger);
	}
	.about {
		gap: 4px;
	}
	.about h2 {
		margin-bottom: 6px;
		font-size: 18px;
	}
	dl {
		margin: 0;
	}
	.row {
		display: grid;
		grid-template-columns: 240px minmax(0, 1fr);
		align-items: center;
		gap: 14px;
		min-height: 46px;
		border-top: 1px solid var(--border-1);
	}
	dt {
		color: var(--text-2);
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	dd.unset {
		color: var(--text-2);
	}
	.note {
		margin: 10px 0 0;
		font-size: 14px;
		line-height: 1.5;
	}
	@media (max-width: 720px) {
		.card {
			padding: 18px;
		}
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}
		.row {
			grid-template-columns: minmax(0, 1fr);
			gap: 2px;
			padding: 10px 0;
		}
	}
</style>
