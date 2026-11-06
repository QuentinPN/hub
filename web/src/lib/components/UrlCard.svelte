<script lang="ts">
	import { ApiError, clearServiceUrl, getService, saveServiceUrl } from '#lib/api/client.ts';
	import type { ServiceDetail, UrlCheckResponse } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';

	let {
		service,
		onchange
	}: { service: ServiceDetail; onchange: (service: ServiceDetail) => void } = $props();

	const ports = $derived(service.ports ?? []);

	let editing = $state(false);
	let address = $state('');
	let busy = $state(false);
	let attempt = $state<UrlCheckResponse | null>(null);
	let refused = $state('');
	let input = $state<HTMLInputElement>();

	const open = $derived(editing || !service.url);

	function edit() {
		address = service.url ?? '';
		attempt = null;
		refused = '';
		editing = true;
	}

	// Le serveur ne connaît pas l'IP de l'hôte : le port s'ajoute à l'adresse tapée, ou prépare
	// le champ pour la taper.
	function usePort(port: number) {
		const parts = /^(https?:\/\/[^/:?#]+)(?::\d+)?(.*)$/.exec(address.trim());
		if (parts) {
			address = `${parts[1]}:${port}${parts[2]}`;
			return;
		}
		address = `http://:${port}`;
		requestAnimationFrame(() => {
			input?.focus();
			input?.setSelectionRange(7, 7);
		});
	}

	async function submit(options: { force?: boolean; acceptCert?: string } = {}) {
		busy = true;
		attempt = null;
		refused = '';
		try {
			const res = await saveServiceUrl(fetch, service.slug, { url: address, ...options });
			if (res.saved) {
				editing = false;
				onchange(await getService(fetch, service.slug));
			} else {
				attempt = res;
			}
		} catch (err) {
			refused = err instanceof ApiError ? err.message : String(err);
		} finally {
			busy = false;
		}
	}

	async function remove() {
		busy = true;
		try {
			onchange(await clearServiceUrl(fetch, service.slug));
			editing = false;
			attempt = null;
			address = '';
		} catch (err) {
			refused = err instanceof ApiError ? err.message : String(err);
		} finally {
			busy = false;
		}
	}

	const answer = $derived.by(() => {
		const check = service.urlCheck;
		if (!check) return '';
		switch (check.result) {
			case 'ok':
				return `${check.status} en ${check.latencyMs} ms`;
			case 'auth':
				return `${check.status} : connexion demandée`;
			case 'cert':
				return 'certificat non reconnu';
			case 'error':
				return `erreur ${check.status}`;
			default:
				return 'injoignable depuis le serveur';
		}
	});
</script>

{#if open}
	<section class="card ask" aria-labelledby="url-title">
		<div>
			<h2 id="url-title">Adresse de {service.name}</h2>
			<p class="muted">
				Celle que tu ouvres dans ton navigateur. Hub la vérifie depuis le serveur avant de
				l’utiliser.
			</p>
		</div>
		<form
			class="row"
			onsubmit={(e) => {
				e.preventDefault();
				void submit();
			}}
		>
			<label>
				Adresse
				<input
					bind:this={input}
					bind:value={address}
					type="text"
					inputmode="url"
					autocomplete="off"
					spellcheck="false"
					placeholder="https://service.exemple.fr"
					required
				/>
			</label>
			<button type="submit" class="btn primary" disabled={busy}>
				{busy ? 'Vérification…' : 'Vérifier et enregistrer'}
			</button>
		</form>

		{#if ports.length > 0}
			<div class="hints muted">
				Ports publiés sur {service.hostId}, à titre d’indice :
				{#each ports as port (port)}
					<button
						type="button"
						class="chip"
						aria-label="Utiliser le port {port}"
						onclick={() => usePort(port)}>{port}</button
					>
				{/each}
			</div>
		{/if}

		<div aria-live="polite">
			{#if refused}
				<p class="result bad">
					<strong>Adresse refusée</strong><span class="muted">{refused}</span>
				</p>
			{:else if attempt?.result === 'cert'}
				<div class="result warn">
					<p>
						<strong>Certificat non reconnu</strong>
						<span class="muted">
							Certificat pour {attempt.certName || 'un nom inconnu'}. Accepte-le seulement si tu
							reconnais ce service.
						</span>
						<span class="muted fingerprint">{attempt.certSha256}</span>
					</p>
					<button
						type="button"
						class="btn"
						disabled={busy}
						onclick={() => submit({ acceptCert: attempt?.certSha256 })}
						>Accepter ce certificat</button
					>
				</div>
			{:else if attempt}
				<div class="result bad">
					<p>
						<strong>
							{attempt.result === 'error'
								? `Le service répond par une erreur ${attempt.status}`
								: 'Injoignable depuis le serveur'}
						</strong>
						<span class="muted">Il peut être filtré pour Hub tout en marchant pour toi.</span>
					</p>
					<button type="button" class="btn" disabled={busy} onclick={() => submit({ force: true })}
						>Enregistrer quand même</button
					>
				</div>
			{/if}
		</div>

		{#if service.url}
			<div class="foot">
				<button type="button" class="link" onclick={() => (editing = false)}>Annuler</button>
				<button type="button" class="link" disabled={busy} onclick={remove}
					>Retirer l’adresse</button
				>
			</div>
		{/if}
	</section>
{:else}
	<section class="card" aria-labelledby="url-title">
		<div class="head">
			<h2 id="url-title">Adresse</h2>
			<button type="button" class="link" onclick={edit}>Changer</button>
		</div>
		<dl>
			<dt class="muted">URL</dt>
			<dd class="url">{service.url}</dd>
			<dt class="muted">Réponse</dt>
			<dd>{answer}</dd>
			{#if service.urlCheck?.checkedAt}
				<dt class="muted">Vérifiée</dt>
				<dd>{ago(service.urlCheck.checkedAt, Date.now())}, par le serveur Hub</dd>
			{/if}
			{#if service.urlCheck?.certPinned}
				<dt class="muted">Certificat</dt>
				<dd>accepté par toi</dd>
			{/if}
		</dl>
	</section>
{/if}

<style>
	.card {
		display: flex;
		flex: 1 1 320px;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.ask {
		flex-basis: 100%;
		border-color: rgba(232, 201, 166, 0.28);
		background: rgba(232, 201, 166, 0.07);
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	p {
		margin: 4px 0 0;
	}
	.head,
	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px;
	}
	label {
		display: flex;
		flex: 1 1 280px;
		flex-direction: column;
		gap: 7px;
		min-width: 0;
		font-size: 14px;
		font-weight: 500;
	}
	input {
		height: 52px;
		box-sizing: border-box;
		padding: 0 16px;
		border: 1px solid var(--border-2);
		border-radius: 14px;
		background: var(--surface-2);
		color: var(--text);
		font: inherit;
		font-size: 16px;
	}
	.btn {
		min-height: 52px;
		padding: 0 22px;
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
	.btn:disabled,
	.link:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.hints {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px;
		font-size: 14px;
	}
	.chip {
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text);
		font-size: 14px;
		cursor: pointer;
	}
	.link {
		min-height: 44px;
		padding: 0 4px;
		border: 0;
		background: transparent;
		color: var(--text);
		font-size: 14px;
		font-weight: 500;
		text-decoration: underline;
		cursor: pointer;
	}
	.result {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 14px;
		margin: 0;
		padding: 14px 16px;
		border: 1px solid;
		border-radius: 16px;
		font-size: 15px;
	}
	.result p {
		display: flex;
		flex: 1 1 260px;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
		margin: 0;
	}
	p.result {
		flex-direction: column;
		align-items: flex-start;
		gap: 3px;
	}
	.warn {
		border-color: rgba(255, 214, 10, 0.3);
		background: rgba(255, 214, 10, 0.06);
	}
	.bad {
		border-color: rgba(255, 69, 58, 0.32);
		background: rgba(255, 69, 58, 0.07);
	}
	.fingerprint {
		overflow-wrap: anywhere;
		font-size: 12px;
	}
	dl {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 10px 16px;
		margin: 0;
		font-size: 15px;
	}
	dd {
		margin: 0;
		text-align: right;
	}
	.url {
		overflow-wrap: anywhere;
	}
</style>
