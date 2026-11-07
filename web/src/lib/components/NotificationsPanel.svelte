<script lang="ts">
	import {
		getNotifications,
		removeNotifyChannel,
		saveNotifyAlerts,
		saveNtfy,
		saveSmtp,
		testNotifyChannel
	} from '#lib/api/client.ts';
	import type { NotifyAlertId, NotifyChannel, NotifySend, NotifyStatus } from '#lib/api/types.ts';
	import Skeleton from './Skeleton.svelte';

	let status = $state<NotifyStatus | null>(null);
	let loadError = $state('');
	let editing = $state<NotifyChannel | null>(null);
	let busy = $state<string | null>(null);
	let formError = $state('');
	let message = $state<{ ok: boolean; text: string } | null>(null);

	// Formulaires : le sujet et le mot de passe partent au serveur et n'en reviennent jamais.
	let ntfyUrl = $state('');
	let ntfyTopic = $state('');
	let smtp = $state({ host: '', port: 587, username: '', password: '', from: '', to: '' });

	const errText = (err: unknown) => (err instanceof Error ? err.message : String(err));

	async function load() {
		try {
			status = await getNotifications(fetch);
			loadError = '';
		} catch (err) {
			loadError = `Impossible de lire les notifications : ${errText(err)}`;
		}
	}

	// Les envois du moteur changent l'état des canaux sans geste ici.
	$effect(() => {
		void load();
		const poll = setInterval(load, 30_000);
		return () => clearInterval(poll);
	});

	function edit(ch: NotifyChannel) {
		if (!status) return;
		formError = '';
		message = null;
		if (ch === 'ntfy') {
			ntfyUrl = status.ntfy.url || 'https://ntfy.sh';
			ntfyTopic = '';
		} else {
			const s = status.smtp;
			smtp = {
				host: s.host,
				port: s.configured ? s.port : 587,
				username: s.username,
				password: '',
				from: s.from,
				to: s.to
			};
		}
		editing = ch;
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!editing) return;
		busy = 'save';
		formError = '';
		try {
			status =
				editing === 'ntfy'
					? await saveNtfy(fetch, ntfyUrl, ntfyTopic)
					: await saveSmtp(fetch, { ...smtp, port: Number(smtp.port) });
			message = { ok: true, text: 'Enregistré. Envoie un test pour vérifier.' };
			editing = null;
		} catch (err) {
			formError = errText(err);
		} finally {
			ntfyTopic = '';
			smtp.password = '';
			busy = null;
		}
	}

	async function remove(ch: NotifyChannel) {
		busy = 'remove';
		formError = '';
		try {
			status = await removeNotifyChannel(fetch, ch);
			message = { ok: true, text: ch === 'ntfy' ? 'ntfy retiré.' : 'E-mail retiré.' };
			editing = null;
		} catch (err) {
			formError = errText(err);
		} finally {
			busy = null;
		}
	}

	async function test(ch: NotifyChannel) {
		busy = `test-${ch}`;
		message = null;
		try {
			status = await testNotifyChannel(fetch, ch);
			const last = ch === 'ntfy' ? status.ntfy.last : status.smtp.last;
			message = last?.ok
				? { ok: true, text: 'Test envoyé.' }
				: { ok: false, text: `Test non parti : ${last?.error || 'cause inconnue'}` };
		} catch (err) {
			message = { ok: false, text: errText(err) };
		} finally {
			busy = null;
		}
	}

	async function toggle(change: { id?: NotifyAlertId; enabled?: boolean; quiet?: boolean }) {
		if (!status) return;
		const alerts = Object.fromEntries(status.alerts.map((a) => [a.id, a.enabled])) as Record<
			NotifyAlertId,
			boolean
		>;
		if (change.id && change.enabled !== undefined) alerts[change.id] = change.enabled;
		message = null;
		try {
			status = await saveNotifyAlerts(fetch, alerts, change.quiet ?? status.quietHours.enabled);
		} catch (err) {
			message = { ok: false, text: `Échec de l'enregistrement : ${errText(err)}` };
			await load();
		}
	}

	const ALERTS: Record<NotifyAlertId, { title: string; short: string; hint: string }> = {
		service: {
			title: 'Un service est arrêté ou injoignable',
			short: 'Service arrêté ou injoignable',
			hint: 'Et quand il revient.'
		},
		host: {
			title: 'Un hôte ou son agent ne répond plus',
			short: 'Hôte ou agent muet',
			hint: 'Mini-PC, QNAP, Raspberry Pi.'
		},
		widget: {
			title: 'Une connexion de widget échoue',
			short: 'Connexion de widget en échec',
			hint: 'Clé refusée, certificat, adresse changée.'
		},
		backup: {
			title: 'Une sauvegarde échoue',
			short: 'Sauvegarde en échec',
			hint: 'Ou n’a pas tourné depuis 2 jours.'
		},
		disk: {
			title: 'Un disque dépasse 90 %',
			short: 'Disque au-delà de 90 %',
			hint: 'D’après Beszel.'
		},
		login: {
			title: 'Une nouvelle connexion à Hub',
			short: 'Nouvelle connexion à Hub',
			hint: 'Depuis un appareil jamais vu.'
		}
	};

	const delay = (s: number) => (s === 0 ? 'tout de suite' : `après ${Math.round(s / 60)} min`);

	const hm = new Intl.DateTimeFormat('fr-FR', { hour: '2-digit', minute: '2-digit' });
	const dm = new Intl.DateTimeFormat('fr-FR', { day: 'numeric', month: 'short' });
	function when(iso: string): string {
		const at = new Date(iso);
		const min = Math.round((Date.now() - at.getTime()) / 60_000);
		if (min < 1) return 'à l’instant';
		if (min < 60) return `il y a ${min} min`;
		const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
		const days = Math.round((day(new Date()) - day(at)) / 86_400_000);
		if (days === 0) return `aujourd’hui à ${hm.format(at)}`;
		if (days === 1) return 'hier';
		return `le ${dm.format(at)}`;
	}

	type Shown = { tone: 'ok' | 'bad' | 'none'; label: string };
	function shown(configured: boolean, last: NotifySend | null): Shown {
		if (!configured) return { tone: 'none', label: 'Pas configuré' };
		if (!last) return { tone: 'none', label: 'Pas encore testé' };
		if (!last.ok) return { tone: 'bad', label: `Échec ${when(last.at)}` };
		return { tone: 'ok', label: `${last.test ? 'Envoi testé' : 'Dernier envoi'} ${when(last.at)}` };
	}
</script>

<div class="stack" id="notifications">
	<section class="card" aria-labelledby="notifications-title">
		<div>
			<h2 id="notifications-title">Notifications</h2>
			<p class="muted intro">Hub te prévient quand quelque chose tombe, et quand ça revient.</p>
		</div>

		{#if loadError}
			<p class="error" role="alert">{loadError}</p>
		{:else if !status}
			<div class="channels" aria-busy="true">
				<Skeleton height="170px" radius="16px" />
				<Skeleton height="170px" radius="16px" />
			</div>
		{:else}
			{@const n = shown(status.ntfy.configured, status.ntfy.last)}
			{@const m = shown(status.smtp.configured, status.smtp.last)}
			<div class="channels">
				<div
					class="channel"
					class:on={n.tone === 'ok'}
					class:fail={n.tone === 'bad'}
					class:empty={!status.ntfy.configured && editing !== 'ntfy'}
				>
					<div class="ch-head">
						<h3>ntfy</h3>
						<span class="state"
							>{#if n.tone !== 'none'}<span class="dot {n.tone}" aria-hidden="true"
								></span>{/if}{n.label}</span
						>
					</div>
					{#if editing === 'ntfy'}
						<form class="form" onsubmit={save}>
							<label
								>Adresse du serveur
								<input
									type="url"
									required
									bind:value={ntfyUrl}
									placeholder="https://ntfy.sh"
								/></label
							>
							<label
								>Sujet
								<input
									type="password"
									autocomplete="off"
									spellcheck="false"
									required={!status.ntfy.configured}
									maxlength="64"
									bind:value={ntfyTopic}
									placeholder={status.ntfy.configured ? 'Inchangé si vide' : 'hub-xxxxxxxx'}
								/></label
							>
							<small
								>Le sujet fait office de mot de passe : long et difficile à deviner. Lettres,
								chiffres, - et _. S’il change de serveur, il faut le saisir de nouveau.</small
							>
							{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
							<div class="actions">
								{#if status.ntfy.configured}
									<button
										type="button"
										class="btn ghost danger"
										disabled={busy !== null}
										onclick={() => remove('ntfy')}>Retirer</button
									>
								{/if}
								<button type="button" class="btn ghost" onclick={() => (editing = null)}
									>Annuler</button
								>
								<button type="submit" class="btn primary" disabled={busy !== null}
									>Enregistrer</button
								>
							</div>
						</form>
					{:else}
						{#if status.ntfy.configured}
							<span class="where"
								>{status.ntfy.url} · sujet
								<strong>{status.ntfy.topicHint}•••••••</strong></span
							>
						{/if}
						{#if status.ntfy.last && !status.ntfy.last.ok}
							<span class="last-error">{status.ntfy.last.error}</span>
						{/if}
						<span class="muted hint"
							>Notifications sur le téléphone, avec l’app ntfy. Le sujet est gardé chiffré.</span
						>
						<div class="actions start">
							{#if status.ntfy.configured}
								<button
									type="button"
									class="btn"
									disabled={busy !== null}
									onclick={() => test('ntfy')}
									>{busy === 'test-ntfy' ? 'Envoi…' : 'Envoyer un test'}</button
								>
								<button type="button" class="btn ghost" onclick={() => edit('ntfy')}
									>Modifier</button
								>
							{:else}
								<button type="button" class="btn" onclick={() => edit('ntfy')}>Configurer</button>
							{/if}
						</div>
					{/if}
				</div>

				<div
					class="channel"
					class:on={m.tone === 'ok'}
					class:fail={m.tone === 'bad'}
					class:empty={!status.smtp.configured && editing !== 'smtp'}
				>
					<div class="ch-head">
						<h3>E-mail</h3>
						<span class="state"
							>{#if m.tone !== 'none'}<span class="dot {m.tone}" aria-hidden="true"
								></span>{/if}{m.label}</span
						>
					</div>
					{#if editing === 'smtp'}
						<form class="form grid" onsubmit={save}>
							<label class="wide"
								>Serveur SMTP
								<input
									required
									bind:value={smtp.host}
									placeholder="smtp.exemple.fr"
									autocomplete="off"
								/></label
							>
							<label
								>Port
								<input type="number" required min="1" max="65535" bind:value={smtp.port} /></label
							>
							<label
								>Compte
								<input
									bind:value={smtp.username}
									autocomplete="off"
									placeholder="Vide : sans compte"
								/></label
							>
							<label class="wide"
								>Mot de passe
								<input
									type="password"
									autocomplete="off"
									bind:value={smtp.password}
									placeholder={status.smtp.passwordSet ? 'Inchangé si vide' : ''}
								/></label
							>
							<label
								>Expéditeur
								<input
									type="email"
									required
									bind:value={smtp.from}
									placeholder="hub@exemple.fr"
								/></label
							>
							<label
								>Destinataire
								<input required bind:value={smtp.to} placeholder="toi@exemple.fr" /></label
							>
							<small class="wide"
								>Port 465 : TLS dès la connexion ; tout autre port doit proposer STARTTLS, sinon Hub
								n’envoie rien. Le mot de passe n’est envoyé qu’au serveur et au compte pour lesquels
								il a été saisi.</small
							>
							{#if formError}<p class="form-error wide" role="alert">{formError}</p>{/if}
							<div class="actions wide">
								{#if status.smtp.configured}
									<button
										type="button"
										class="btn ghost danger"
										disabled={busy !== null}
										onclick={() => remove('smtp')}>Retirer</button
									>
								{/if}
								<button type="button" class="btn ghost" onclick={() => (editing = null)}
									>Annuler</button
								>
								<button type="submit" class="btn primary" disabled={busy !== null}
									>Enregistrer</button
								>
							</div>
						</form>
					{:else}
						{#if status.smtp.configured}
							<span class="where"
								>{status.smtp.host}:{status.smtp.port} · vers
								<strong>{status.smtp.to}</strong></span
							>
						{/if}
						{#if status.smtp.last && !status.smtp.last.ok}
							<span class="last-error">{status.smtp.last.error}</span>
						{/if}
						<span class="muted hint"
							>Par un serveur SMTP que tu fournis (adresse, port, compte). Le mot de passe est gardé
							chiffré et jamais réaffiché.</span
						>
						<div class="actions start">
							{#if status.smtp.configured}
								<button
									type="button"
									class="btn"
									disabled={busy !== null}
									onclick={() => test('smtp')}
									>{busy === 'test-smtp' ? 'Envoi…' : 'Envoyer un test'}</button
								>
								<button type="button" class="btn ghost" onclick={() => edit('smtp')}
									>Modifier</button
								>
							{:else}
								<button type="button" class="btn" onclick={() => edit('smtp')}>Configurer</button>
							{/if}
						</div>
					{/if}
				</div>
			</div>
			<p
				class="status-line"
				class:error={message && !message.ok}
				class:quiet-line={!message}
				role="status"
			>
				{message?.text ?? ''}
			</p>
		{/if}
	</section>

	{#if status}
		<section class="card" aria-labelledby="notify-alerts-title">
			<h2 id="notify-alerts-title" class="h3">Me prévenir quand</h2>
			<ul class="alerts">
				{#each status.alerts as a (a.id)}
					{@const t = ALERTS[a.id]}
					<li>
						<label>
							<span class="what"
								><strong class="long">{t.title}</strong><strong class="short">{t.short}</strong
								><span class="muted hint long">{t.hint}</span><span class="muted hint short"
									>{delay(a.delaySeconds)}</span
								></span
							>
							<span class="delay">{delay(a.delaySeconds)}</span>
							<input
								type="checkbox"
								checked={a.enabled}
								onchange={(e) => toggle({ id: a.id, enabled: e.currentTarget.checked })}
							/>
						</label>
					</li>
				{/each}
			</ul>
		</section>

		<section class="card quiet" aria-labelledby="notify-quiet-title">
			<label>
				<span class="what">
					<h2 id="notify-quiet-title" class="h3">Heures calmes</h2>
					<span class="muted hint long"
						>De {status.quietHours.from} à {status.quietHours.to}, Hub garde les alertes et les
						envoie en un seul message le matin. Un hôte injoignable passe quand même.</span
					>
					<span class="muted hint short"
						>{status.quietHours.from} à {status.quietHours.to}, résumé le matin</span
					>
					{#if status.pending > 0}
						<span class="muted hint"
							>{status.pending}
							{status.pending > 1 ? 'messages gardés' : 'message gardé'} pour le résumé.</span
						>
					{/if}
				</span>
				<input
					type="checkbox"
					checked={status.quietHours.enabled}
					onchange={(e) => toggle({ quiet: e.currentTarget.checked })}
				/>
			</label>
		</section>
	{/if}
</div>

<style>
	.stack {
		display: flex;
		flex-direction: column;
		gap: 20px;
		scroll-margin-top: 24px;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 16px;
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
	h2.h3 {
		font-size: 18px;
	}
	h3 {
		margin: 0;
		font-size: 17px;
		font-weight: 600;
	}
	.intro {
		margin: 4px 0 0;
	}
	.channels {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 14px;
	}
	.channel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
		padding: 18px;
		border: 1px solid var(--border-2);
		border-radius: 16px;
		background: var(--surface-1);
	}
	.channel.on {
		border-color: var(--tint-ok-border);
		background: var(--tint-ok-bg);
	}
	.channel.fail {
		border-color: var(--tint-danger-border);
		background: var(--tint-danger-bg);
	}
	.channel.empty {
		border-style: dashed;
		border-color: var(--border-dashed);
		background: var(--surface-inset);
	}
	.ch-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.state {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--text-2);
		font-size: 14px;
		text-align: right;
	}
	.channel.on .state,
	.channel.fail .state {
		color: var(--text-2);
	}
	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 99px;
		background: var(--status-online);
	}
	.dot.bad {
		background: var(--status-down);
	}
	.where {
		color: var(--text-2);
		overflow-wrap: anywhere;
	}
	.where strong {
		font-weight: 600;
	}
	.last-error {
		color: var(--text-danger);
		font-size: 14px;
	}
	.hint {
		font-size: 14px;
		line-height: 1.45;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 10px;
		margin-top: auto;
	}
	.actions.start {
		justify-content: flex-start;
	}
	.btn {
		min-height: 44px;
		padding: 0 16px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-1);
		color: var(--text);
		font: inherit;
		font-weight: 600;
		cursor: pointer;
	}
	.btn.ghost {
		background: transparent;
		font-weight: 500;
	}
	.btn.danger {
		margin-right: auto;
		color: var(--text-danger);
	}
	.btn.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.btn:focus-visible,
	input:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 2px;
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.form.grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
	}
	.form .wide {
		grid-column: 1 / -1;
	}
	.form label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
		font-size: 14px;
		font-weight: 500;
	}
	.form input {
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
	small {
		color: var(--text-2);
		font-size: var(--fs-small);
		line-height: 1.45;
	}
	.form-error,
	.error {
		margin: 0;
		padding: 10px 12px;
		border: 1px solid var(--tint-danger-border);
		border-radius: 12px;
		background: var(--tint-danger-bg);
		font-size: 14px;
	}
	.status-line {
		margin: -6px 0 0;
		color: var(--text-2);
		font-size: 14px;
	}
	/* Vide, la ligne reste dans la page pour être annoncée, sans prendre de place. */
	.status-line.quiet-line {
		margin-top: -16px;
	}
	.status-line.error {
		color: var(--text-danger);
	}
	.alerts {
		margin: -10px 0 0;
		padding: 0;
		list-style: none;
	}
	.alerts label {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 170px 60px;
		align-items: center;
		gap: 14px;
		min-height: 58px;
		padding: 8px 0;
		border-top: 1px solid var(--border-1);
		cursor: pointer;
	}
	.what {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.what strong {
		font-weight: 600;
	}
	.delay {
		color: var(--text-2);
		font-size: 14px;
		text-align: right;
	}
	input[type='checkbox'] {
		justify-self: end;
		flex: none;
		width: 22px;
		height: 22px;
		margin: 0;
		accent-color: var(--status-online);
		cursor: pointer;
	}
	.quiet {
		padding: 18px 22px;
	}
	.quiet label {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		cursor: pointer;
	}
	.short {
		display: none;
	}
	@media (max-width: 720px) {
		.channels {
			grid-template-columns: 1fr;
		}
		.card {
			padding: 14px 16px;
		}
		.channel {
			padding: 14px;
		}
		.channel .hint {
			display: none;
		}
		.form.grid {
			grid-template-columns: 1fr;
		}
		.long,
		.delay {
			display: none;
		}
		.short {
			display: inline;
		}
		.what .short.hint {
			display: block;
			font-size: 13px;
		}
		.alerts label {
			display: flex;
			justify-content: space-between;
			min-height: 62px;
		}
		.quiet .what .hint.short {
			display: block;
			font-size: 13px;
		}
	}
</style>
