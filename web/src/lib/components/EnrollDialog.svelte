<script lang="ts">
	import { ApiError, cancelEnrollment, createEnrollment } from '#lib/api/client.ts';
	import type { AgentHost, Enrollment } from '#lib/api/types.ts';
	import { countdown, hasAgent, HOST_ID_PATTERN, hostIdFrom, validHostId } from '#lib/agents.ts';

	interface Props {
		/** Hôte déjà connu (nouvel enrôlement) : son identifiant ne change plus. */
		host: AgentHost | null;
		/** L'hôte tel que la liste le voit en ce moment, pour annoncer le premier contact. */
		live: AgentHost | null;
		now: number;
		onclose: () => void;
		/** Un code a été émis ou annulé : la liste se recharge. */
		onchange: () => void;
		/** Le code est créé pour cet identifiant : la liste suit cet hôte. */
		onenrolled: (hostId: string) => void;
	}
	let { host, live, now, onclose, onchange, onenrolled }: Props = $props();

	const MAX_NAME = 80;

	let dialog: HTMLDialogElement;
	// Valeurs de départ figées à l'ouverture : la liste peut se recharger pendant la saisie.
	// svelte-ignore state_referenced_locally
	let name = $state(host?.name ?? '');
	// svelte-ignore state_referenced_locally
	let id = $state(host?.id ?? '');
	let idTouched = $state(false);
	let enrollment = $state<Enrollment | null>(null);
	let shown = $state(false);
	let busy = $state(false);
	let error = $state('');
	let copied = $state('');

	const locked = $derived(host !== null);
	const expired = $derived(enrollment !== null && Date.parse(enrollment.expiresAt) <= now);
	// Le premier contact : l'agent a un certificat, qu'il ait déjà parlé ou non. La liste ne
	// propose d'enrôler qu'un hôte sans agent qui fonctionne.
	const arrived = $derived(enrollment !== null && live !== null && hasAgent(live.state));
	const env = $derived(
		enrollment
			? `HUB_SERVER_URL=${enrollment.serverUrl}\nHUB_CA_PIN=${enrollment.pin}\nDOCKER_GID=          # stat -c %g /var/run/docker.sock`
			: ''
	);

	$effect(() => {
		dialog.showModal();
	});

	function onName() {
		if (!locked && !idTouched) id = hostIdFrom(name);
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		const label = name.trim();
		if ([...label].length > MAX_NAME) {
			error = `Le nom affiché fait ${MAX_NAME} caractères au plus.`;
			return;
		}
		if (!validHostId(id)) {
			error =
				'Identifiant : lettres minuscules, chiffres et tirets, 63 au plus, sans tiret au début.';
			return;
		}
		busy = true;
		try {
			enrollment = await createEnrollment(fetch, id, label);
			onenrolled(enrollment.hostId);
			onchange();
		} catch (err) {
			error =
				err instanceof ApiError && err.status === 400
					? 'Le serveur a refusé cet identifiant ou ce nom.'
					: `Impossible de créer le code : ${err instanceof Error ? err.message : String(err)}`;
		} finally {
			busy = false;
		}
	}

	async function copy(what: 'code' | 'env', text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = what;
		} catch {
			copied = '';
			error = 'Copie impossible ici : sélectionne le texte à la main.';
		}
	}

	async function cancel() {
		if (!enrollment) return;
		busy = true;
		try {
			await cancelEnrollment(fetch, enrollment.hostId);
			onchange();
			dialog.close();
		} catch (err) {
			error = `Impossible d’annuler le code : ${err instanceof Error ? err.message : String(err)}`;
		} finally {
			busy = false;
		}
	}
</script>

<dialog bind:this={dialog} aria-labelledby="enroll-title" {onclose}>
	<div class="head">
		<div>
			<h2 id="enroll-title">{locked ? `Enrôler ${host?.name}` : 'Ajouter un hôte'}</h2>
			<p class="muted intro">
				L’agent tourne sur l’hôte, en lecture seule, derrière un proxy Docker. C’est lui qui ouvre
				la connexion vers Hub : rien à ouvrir sur l’hôte.
			</p>
		</div>
		<button type="button" class="close" aria-label="Fermer" onclick={() => dialog.close()}>
			<svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
				<path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" />
			</svg>
		</button>
	</div>

	{#if !enrollment}
		<form class="step" onsubmit={submit} novalidate>
			<span class="num" aria-hidden="true">1</span>
			<div class="body">
				<h3>L’hôte</h3>
				<div class="fields">
					<label>
						Nom affiché
						<input
							name="host-name"
							autocomplete="off"
							maxlength={MAX_NAME}
							placeholder="mini-PC Geekom"
							bind:value={name}
							oninput={onName}
						/>
					</label>
					<label>
						Identifiant
						<input
							name="host-id"
							class="mono"
							autocomplete="off"
							spellcheck="false"
							pattern={HOST_ID_PATTERN}
							required
							readonly={locked}
							aria-describedby="host-id-help"
							bind:value={id}
							oninput={() => (idTouched = true)}
						/>
					</label>
				</div>
				<p id="host-id-help" class="muted small">
					L’identifiant est gravé dans le certificat de l’agent : il ne change plus ensuite.
				</p>
				<div class="actions">
					<p class="status error" role="alert">{error}</p>
					<button type="button" class="btn ghost" onclick={() => dialog.close()}>Annuler</button>
					<button type="submit" class="btn primary" disabled={busy}
						>{busy ? 'Création…' : 'Créer un code'}</button
					>
				</div>
			</div>
		</form>
	{:else}
		<section class="step" aria-labelledby="code-title">
			<span class="num" aria-hidden="true">1</span>
			<div class="body">
				<div class="row">
					<h3 id="code-title">Code d’enrôlement</h3>
					<span class="chip" class:gone={expired && !arrived} class:used={arrived}>
						{arrived
							? 'Code utilisé'
							: expired
								? 'Expiré : crée un nouveau code'
								: `Usage unique · expire dans ${countdown(enrollment.expiresAt, now)}`}
					</span>
				</div>
				<div class="row">
					<output class="mono code-box" aria-label="Code d’enrôlement"
						>{shown ? enrollment.code : '•'.repeat(32)}</output
					>
					<button type="button" class="btn ghost small" onclick={() => (shown = !shown)}
						>{shown ? 'Masquer' : 'Afficher'}</button
					>
					<button
						type="button"
						class="btn ghost small"
						onclick={() => copy('code', enrollment?.code ?? '')}
						>{copied === 'code' ? 'Copié' : 'Copier'}</button
					>
				</div>
				<p class="muted small">
					Hub n’en garde qu’une empreinte : une fois cette fenêtre fermée, il faudra en demander un
					nouveau.
				</p>
			</div>
		</section>

		<section class="step" aria-labelledby="host-title">
			<span class="num" aria-hidden="true">2</span>
			<div class="body">
				<h3 id="host-title">Sur l’hôte, dans le dossier de l’agent</h3>
				<div class="row">
					<span class="muted small"
						>Fichier <span class="mono">.env</span>, à côté du
						<span class="mono">compose.yml</span> de <span class="mono">deploy/agent</span></span
					>
					<button type="button" class="btn ghost small" onclick={() => copy('env', env)}
						>{copied === 'env' ? 'Copié' : 'Copier'}</button
					>
				</div>
				<pre class="code">{env}</pre>
				<p class="muted small">
					Puis le code dans le fichier <span class="mono">enroll-code</span>, et :
				</p>
				<pre class="code">docker compose up -d</pre>
				<p class="muted small">
					L’agent vérifie l’empreinte de l’AC avant d’envoyer le code. Le fichier
					<span class="mono">enroll-code</span> peut ensuite être supprimé.
				</p>
			</div>
		</section>

		<p class="waiting" class:ok={arrived} role="status">
			<span class="spinner" aria-hidden="true"></span>
			{#if arrived}
				Agent enrôlé. Il apparaît dans la liste des hôtes.
			{:else}
				En attente du premier contact de l’agent…
				<span class="muted">Cette fenêtre se met à jour toute seule.</span>
			{/if}
		</p>

		<div class="actions">
			<p class="status error" role="alert">{error}</p>
			{#if !arrived && !expired}
				<button type="button" class="btn ghost" disabled={busy} onclick={cancel}
					>Annuler le code</button
				>
			{/if}
			<button type="button" class="btn primary" onclick={() => dialog.close()}>Fermer</button>
		</div>
	{/if}
</dialog>

<style>
	dialog {
		width: min(760px, calc(100vw - 32px));
		max-height: calc(100dvh - 48px);
		padding: 28px;
		border: 1px solid var(--border-2);
		border-radius: 24px;
		background: var(--glass-dialog);
		color: var(--text);
		box-shadow: var(--shadow-lift);
		-webkit-backdrop-filter: var(--blur-strong);
		backdrop-filter: var(--blur-strong);
	}
	dialog::backdrop {
		background: rgba(0, 0, 0, 0.55);
		-webkit-backdrop-filter: var(--blur-backdrop);
		backdrop-filter: var(--blur-backdrop);
	}
	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 20px;
	}
	h2 {
		margin: 0 0 6px;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	h3 {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
	}
	.intro,
	.small {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
	}
	.small {
		font-size: var(--fs-small);
	}
	.close {
		display: grid;
		flex: none;
		place-items: center;
		width: 44px;
		height: 44px;
		border: 1px solid var(--border-2);
		border-radius: 50%;
		background: transparent;
		cursor: pointer;
	}
	.step {
		display: grid;
		grid-template-columns: 32px minmax(0, 1fr);
		gap: 16px;
		margin-bottom: 22px;
	}
	.num {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: rgba(255, 255, 255, 0.1);
		font-size: 14px;
		font-weight: 700;
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 0;
	}
	.fields {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
	}
	label {
		display: flex;
		flex: 1 1 220px;
		flex-direction: column;
		gap: 6px;
		font-size: 14px;
		font-weight: 500;
	}
	input {
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 16px;
	}
	input[readonly] {
		color: var(--text-2);
	}
	input:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 1px;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
	}
	.chip {
		padding: 4px 12px;
		border-radius: var(--r-pill);
		background: rgba(232, 201, 166, 0.12);
		color: var(--accent-play);
		font-size: 13px;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	.chip.gone {
		background: rgba(255, 214, 10, 0.12);
		color: var(--status-degraded);
	}
	.chip.used {
		background: rgba(48, 209, 88, 0.12);
		color: var(--status-online);
	}
	.code-box {
		display: flex;
		flex: 1 1 260px;
		align-items: center;
		min-width: 0;
		min-height: 44px;
		padding: 10px 14px;
		border: 1px solid var(--border-1);
		border-radius: 12px;
		background: rgba(0, 0, 0, 0.45);
		overflow-wrap: anywhere;
	}
	/* Coupé à la ligne plutôt que défilant : rien à faire défiler au clavier, et le bouton
	   Copier rend le texte d'origine. */
	.code {
		margin: 0;
		padding: 14px 16px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		border: 1px solid var(--border-1);
		border-radius: 12px;
		background: rgba(0, 0, 0, 0.45);
		color: #e6e6ea;
		font-family: var(--font-mono);
		font-size: 13px;
		line-height: 1.7;
	}
	.waiting {
		display: flex;
		align-items: center;
		gap: 12px;
		margin: 0 0 20px;
		padding: 14px 16px;
		border-radius: 14px;
		background: var(--surface-1);
		font-size: 14px;
	}
	.spinner {
		flex: none;
		width: 12px;
		height: 12px;
		border: 2px solid var(--text-2);
		border-top-color: transparent;
		border-radius: 50%;
		animation: spin 900ms linear infinite;
	}
	.waiting.ok .spinner {
		border-color: var(--status-online);
		background: var(--status-online);
		animation: none;
	}
	@keyframes spin {
		to {
			transform: rotate(1turn);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.spinner {
			animation: none;
		}
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
	}
	.status {
		flex: 1;
		min-width: 200px;
		margin: 0;
		font-size: 14px;
	}
	.status.error {
		color: #ff8a80;
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
	.btn.small {
		padding: 0 14px;
		font-size: 14px;
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
		/* Feuille posée en bas de l'écran, comme la barre d'onglets. */
		dialog {
			width: 100vw;
			max-width: none;
			max-height: 92dvh;
			margin: auto 0 0;
			padding: 20px 16px 28px;
			border-radius: 24px 24px 0 0;
		}
		.step {
			grid-template-columns: 1fr;
			gap: 8px;
		}
		.num {
			display: none;
		}
	}
</style>
