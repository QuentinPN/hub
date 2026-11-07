<script lang="ts">
	import {
		getBackup,
		runBackup,
		saveBackupSchedule,
		setBackupPassphrase
	} from '#lib/api/client.ts';
	import type {
		BackupFrequency,
		BackupRetention,
		BackupRun,
		BackupStatus
	} from '#lib/api/types.ts';
	import Skeleton from './Skeleton.svelte';

	const MIN_PASSPHRASE = 12;

	let status = $state<BackupStatus | null>(null);
	let loadError = $state('');
	let message = $state<{ ok: boolean; text: string } | null>(null);
	let busy = $state(false);
	let changing = $state(false);
	let pass1 = $state('');
	let pass2 = $state('');

	const errText = (err: unknown) => (err instanceof Error ? err.message : String(err));

	async function load() {
		try {
			status = await getBackup(fetch);
			loadError = '';
		} catch (err) {
			loadError = `Impossible de lire l'état des sauvegardes : ${errText(err)}`;
		}
	}

	// Relue vite pendant une sauvegarde, pour voir sa fin. Ne dépend que de « en cours » : un
	// enregistrement ne relance pas de lecture, qui pourrait le précéder.
	const running = $derived(status?.running ?? false);
	$effect(() => {
		void load();
		const every = running ? 2_000 : 30_000;
		const poll = setInterval(load, every);
		return () => clearInterval(poll);
	});

	async function schedule(change: {
		enabled?: boolean;
		frequency?: BackupFrequency;
		retention?: BackupRetention;
	}) {
		if (!status) return;
		message = null;
		try {
			status = await saveBackupSchedule(fetch, {
				enabled: status.enabled,
				frequency: status.frequency,
				retention: status.retention,
				...change
			});
			message = { ok: true, text: 'Enregistré.' };
		} catch (err) {
			message = { ok: false, text: `Échec de l'enregistrement : ${errText(err)}` };
		}
	}

	async function now() {
		busy = true;
		message = null;
		try {
			status = await runBackup(fetch);
		} catch (err) {
			message = { ok: false, text: errText(err) };
		} finally {
			busy = false;
		}
	}

	const passError = $derived(
		pass1.length > 0 && [...pass1].length < MIN_PASSPHRASE
			? `${MIN_PASSPHRASE} caractères au moins.`
			: pass2.length > 0 && pass1 !== pass2
				? 'Les deux phrases diffèrent.'
				: ''
	);

	async function savePassphrase(event: SubmitEvent) {
		event.preventDefault();
		if (passError || pass1 !== pass2) return;
		busy = true;
		message = null;
		try {
			status = await setBackupPassphrase(fetch, pass1);
			pass1 = pass2 = '';
			changing = false;
			message = { ok: true, text: 'Phrase de passe enregistrée. Note-la : Hub ne la garde pas.' };
		} catch (err) {
			message = { ok: false, text: errText(err) };
		} finally {
			busy = false;
		}
	}

	const when = new Intl.DateTimeFormat('fr-FR', {
		weekday: 'short',
		day: 'numeric',
		month: 'short',
		hour: '2-digit',
		minute: '2-digit'
	});
	const fmt = (iso: string) => when.format(new Date(iso));

	function size(bytes: number): string {
		if (bytes < 1024) return `${bytes} o`;
		if (bytes < 1024 * 1024)
			return `${(bytes / 1024).toLocaleString('fr-FR', { maximumFractionDigits: 0 })} Ko`;
		return `${(bytes / 1024 / 1024).toLocaleString('fr-FR', { maximumFractionDigits: 1 })} Mo`;
	}

	function duration(r: BackupRun): string {
		if (!r.finishedAt) return '';
		const s = Math.max(0, Math.round((Date.parse(r.finishedAt) - Date.parse(r.startedAt)) / 1000));
		return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${s % 60} s`;
	}

	type Shown = { tone: 'ok' | 'bad' | 'run'; label: string };
	function shown(r: BackupRun, i: number): Shown {
		if (!r.finishedAt) {
			return status?.running && i === 0
				? { tone: 'run', label: 'En cours' }
				: { tone: 'bad', label: 'Interrompue (Hub arrêté pendant la sauvegarde)' };
		}
		if (r.ok) return { tone: 'ok', label: `Réussie · relue${r.manual ? ' · à la main' : ''}` };
		return { tone: 'bad', label: `Échec · ${r.error || 'cause inconnue'}` };
	}

	const last = $derived(status?.runs.find((r) => r.finishedAt) ?? null);

	const FREQUENCIES: [BackupFrequency, string][] = [
		['daily', 'Tous les jours à 03:00'],
		['weekly', 'Toutes les semaines, le dimanche à 03:00']
	];
	const RETENTIONS: [BackupRetention, string][] = [
		['7d4w', '7 jours et 4 semaines'],
		['30d', '30 jours'],
		['last3', 'Les 3 dernières']
	];
	const retentionShort = (r: BackupRetention) =>
		({ '7d4w': '7 jours + 4 semaines', '30d': '30 jours', last3: 'les 3 dernières' })[r];
</script>

<div class="head">
	<div>
		<h2 id="backup-title">Sauvegarde</h2>
		<p class="muted intro">La base et les clés, ensemble, hors de l’hôte de Hub.</p>
	</div>
	{#if status}
		<button
			type="button"
			class="btn primary"
			disabled={!status.configured || status.running || busy || status.dirError !== ''}
			onclick={now}>{status.running ? 'Sauvegarde en cours…' : 'Sauvegarder maintenant'}</button
		>
	{/if}
</div>

{#if loadError}
	<p class="error" role="alert">{loadError}</p>
{:else if !status}
	<div class="tiles" aria-busy="true">
		<Skeleton height="86px" radius="16px" />
		<Skeleton height="86px" radius="16px" />
		<Skeleton height="86px" radius="16px" />
	</div>
{:else}
	{#if status.dirError}
		<p class="notice bad" role="alert">
			{status.dirError}. Monter un dossier du QNAP dans le compose de Hub, puis le redémarrer : la
			marche à suivre est dans <code>docs/EXPLOITATION.md</code>.
		</p>
	{/if}

	{#if !status.configured || changing}
		<form class="pass" onsubmit={savePassphrase}>
			<p class="note">
				{#if status.configured}
					Les sauvegardes déjà faites gardent l’ancienne phrase : chacune s’ouvre avec celle sous
					laquelle elle a été faite.
				{:else}
					Choisis une phrase de passe pour chiffrer les sauvegardes. Hub ne la garde nulle part :
					sans elle, une sauvegarde ne se restaure pas. Note-la dans ton gestionnaire de mots de
					passe.
				{/if}
			</p>
			<input
				type="text"
				name="username"
				autocomplete="username"
				value="Sauvegardes Hub"
				hidden
				readonly
			/>
			<label
				>Phrase de passe
				<input
					type="password"
					autocomplete="new-password"
					minlength={MIN_PASSPHRASE}
					required
					bind:value={pass1}
				/></label
			>
			<label
				>Encore une fois
				<input type="password" autocomplete="new-password" required bind:value={pass2} /></label
			>
			<div class="form-actions">
				<span class="status" class:error={passError} role="status">{passError}</span>
				{#if changing}
					<button type="button" class="btn ghost" onclick={() => (changing = false)}>Annuler</button
					>
				{/if}
				<button
					type="submit"
					class="btn primary"
					disabled={busy || !!passError || pass1.length === 0 || pass1 !== pass2}
					>Enregistrer la phrase</button
				>
			</div>
		</form>
	{/if}

	{#if status.configured}
		<div class="set">
			<span class="lbl" id="auto-label"
				>Sauvegarde automatique<small
					>{status.enabled
						? 'Hub sauvegarde seul, au créneau choisi.'
						: 'Désactivée : Hub ne sauvegarde plus que sur « Sauvegarder maintenant », et État de Hub le signale.'}</small
				></span
			>
			<button
				type="button"
				class="switch"
				role="switch"
				aria-checked={status.enabled}
				aria-labelledby="auto-label"
				onclick={() => schedule({ enabled: !status?.enabled })}><span></span></button
			>
		</div>
	{/if}

	<div class="tiles">
		<div class="tile" class:good={last?.ok} class:fail={last && !last.ok}>
			<span class="muted small">Dernière sauvegarde</span>
			<strong>{last ? fmt(last.startedAt) : 'Aucune'}</strong>
			<span class="muted tiny"
				>{last
					? last.ok
						? `réussie · ${size(last.size)} · ${duration(last)}`
						: 'en échec'
					: 'pas encore'}</span
			>
		</div>
		<div class="tile">
			<span class="muted small">Prochaine</span>
			<strong>{status.next ? fmt(status.next) : status.configured ? 'Aucune' : '—'}</strong>
			<span class="muted tiny"
				>{!status.configured
					? 'après la phrase de passe'
					: status.enabled
						? status.frequency === 'daily'
							? 'tous les jours'
							: 'le dimanche'
						: 'automatique désactivée'}</span
			>
		</div>
		<div class="tile">
			<span class="muted small">Gardées</span>
			<strong class="tabular"
				>{status.kept}
				{status.kept > 1 ? 'sauvegardes' : 'sauvegarde'}</strong
			>
			<span class="muted tiny">{size(status.keptBytes)} · {retentionShort(status.retention)}</span>
		</div>
	</div>

	<div class="grid">
		<label
			>Fréquence
			<select
				value={status.frequency}
				disabled={!status.configured}
				onchange={(e) => schedule({ frequency: e.currentTarget.value as BackupFrequency })}
			>
				{#each FREQUENCIES as [v, label] (v)}
					<option value={v}>{label}</option>
				{/each}
			</select>
		</label>
		<label
			>Garder
			<select
				value={status.retention}
				disabled={!status.configured}
				onchange={(e) => schedule({ retention: e.currentTarget.value as BackupRetention })}
			>
				{#each RETENTIONS as [v, label] (v)}
					<option value={v}>{label}</option>
				{/each}
			</select>
		</label>
		<div class="field">
			<span class="flabel">Destination</span>
			<span class="value mono">{status.dir || 'non configurée'}</span>
			<small
				>Un dossier du conteneur (<code>HUB_BACKUP_DIR</code>), monté depuis le QNAP. Hub n’y écrit
				que ses sauvegardes.</small
			>
		</div>
		<div class="field">
			<span class="flabel">Chiffrement</span>
			<span class="value" class:okv={status.configured}
				>{#if status.configured}<span class="dot" aria-hidden="true"></span>Chiffrées avec ta phrase
					de passe{:else}Pas encore de phrase de passe{/if}</span
			>
			{#if status.configured && !changing}
				<small
					>Elle n’est gardée nulle part : sans elle, une sauvegarde ne se restaure pas.
					<button type="button" class="linkish" onclick={() => (changing = true)}>Changer</button
					></small
				>
			{/if}
		</div>
	</div>

	<p class="status-line" class:error={message && !message.ok} role="status">
		{message?.text ?? ''}
	</p>

	<h3 class="sub">Dernières sauvegardes</h3>
	{#if status.runs.length === 0}
		<p class="muted small">Aucune pour l’instant.</p>
	{:else}
		<ul class="runs">
			{#each status.runs as r, i (r.id)}
				{@const s = shown(r, i)}
				<li>
					<span class="dot {s.tone}" aria-hidden="true"></span>
					<strong>{fmt(r.startedAt)}</strong>
					<span class="detail">{s.label}</span>
					<span class="muted small size">{r.ok ? size(r.size) : '—'}</span>
				</li>
			{/each}
		</ul>
	{/if}
	<p class="muted small">
		Restaurer se fait sur l’hôte, Hub arrêté, avec <code>hub-server -restore</code> : la marche à
		suivre est dans <code>docs/EXPLOITATION.md</code>. <code>hub-server -verify-backup</code> vérifie
		une sauvegarde sans rien arrêter.
	</p>
{/if}

<style>
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: 12px 16px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.intro {
		margin: 4px 0 0;
	}
	.sub {
		margin: 20px 0 6px;
		font-size: 17px;
		font-weight: 600;
	}
	.error,
	.notice.bad {
		margin: 16px 0 0;
		padding: 12px 14px;
		border: 1px solid rgba(255, 69, 58, 0.35);
		border-radius: 14px;
		background: rgba(255, 69, 58, 0.08);
		font-size: 14px;
		line-height: 1.5;
	}
	.pass {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 14px 16px;
		margin-top: 16px;
		padding: 16px;
		border: 1px solid var(--border-2);
		border-radius: 16px;
		background: var(--surface-1);
	}
	.note,
	.form-actions {
		grid-column: 1 / -1;
	}
	.note {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
	}
	.form-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 8px;
	}
	.form-actions .status {
		flex: 1;
		color: var(--text-2);
		font-size: 14px;
	}
	.status.error,
	.status-line.error {
		color: #ff8a80;
	}
	label,
	.field {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 14px;
		font-weight: 500;
	}
	input,
	select,
	.value {
		box-sizing: border-box;
		width: 100%;
		min-width: 0;
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 15px;
		font-weight: 400;
	}
	select:disabled {
		opacity: 0.6;
	}
	input:focus-visible,
	select:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 1px;
	}
	.value {
		display: flex;
		align-items: center;
		gap: 10px;
		overflow-wrap: anywhere;
	}
	.value.okv {
		border-color: rgba(48, 209, 88, 0.3);
		background: rgba(48, 209, 88, 0.06);
	}
	.mono {
		font-family: ui-monospace, monospace;
		font-size: 14px;
	}
	small {
		color: var(--text-2);
		font-size: var(--fs-small);
		font-weight: 400;
		line-height: 1.45;
	}
	.linkish {
		padding: 0;
		border: 0;
		background: none;
		color: var(--text);
		font: inherit;
		text-decoration: underline;
		cursor: pointer;
	}
	.set {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 24px;
		margin-top: 16px;
		padding: 14px 16px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-1);
	}
	.lbl {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 15px;
		font-weight: 600;
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
		transition:
			left var(--dur) var(--ease),
			right var(--dur) var(--ease);
	}
	.switch[aria-checked='false'] {
		background: rgba(255, 255, 255, 0.16);
	}
	.switch[aria-checked='false'] span {
		right: auto;
		left: 2px;
	}
	.switch:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 2px;
	}
	.tiles {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
		margin-top: 16px;
	}
	.tile {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 14px 16px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: rgba(0, 0, 0, 0.2);
	}
	.tile.good {
		border-color: rgba(48, 209, 88, 0.3);
		background: rgba(48, 209, 88, 0.07);
	}
	.tile.fail {
		border-color: rgba(255, 69, 58, 0.35);
		background: rgba(255, 69, 58, 0.07);
	}
	.tile strong {
		font-size: 20px;
		font-weight: 700;
	}
	.small {
		font-size: 14px;
	}
	.tiny {
		font-size: 13px;
	}
	.grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
		margin-top: 18px;
	}
	.status-line {
		min-height: 1.4em;
		margin: 10px 0 0;
		color: var(--text-2);
		font-size: 14px;
	}
	.runs {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.runs li {
		display: grid;
		grid-template-columns: 14px minmax(150px, 200px) minmax(0, 1fr) auto;
		align-items: center;
		gap: 12px;
		min-height: 46px;
		border-top: 1px solid var(--border-1);
	}
	.dot {
		width: 9px;
		height: 9px;
		border-radius: 99px;
		background: var(--status-online);
	}
	.dot.bad {
		background: var(--status-down);
	}
	.dot.run {
		background: var(--status-degraded);
	}
	.detail {
		color: var(--text-2);
		font-size: 14px;
	}
	.size {
		text-align: right;
	}
	.btn {
		flex: none;
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
	@media (max-width: 720px) {
		.tiles,
		.grid,
		.pass {
			grid-template-columns: 1fr;
		}
		.head .btn {
			width: 100%;
		}
		.runs li {
			grid-template-columns: 14px minmax(0, 1fr) auto;
			padding: 8px 0;
		}
		.runs .size {
			grid-row: 1;
			grid-column: 3;
		}
		.runs .detail {
			grid-row: 2;
			grid-column: 2 / -1;
		}
	}
</style>
