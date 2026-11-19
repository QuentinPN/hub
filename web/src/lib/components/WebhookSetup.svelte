<script lang="ts">
	import { errorMessage, createWebhookSecret, getService } from '#lib/api/client.ts';
	import type { ServiceDetail, WebhookSecret } from '#lib/api/types.ts';

	let {
		slug,
		configured,
		onrefresh
	}: { slug: string; configured: boolean; onrefresh: (service: ServiceDetail) => void } = $props();

	// Le secret n'existe qu'ici, le temps de cette ouverture : le serveur n'en rend plus que « configuré ».
	let created = $state<WebhookSecret | null>(null);
	let busy = $state(false);
	let error = $state('');
	let copied = $state('');

	const isConfigured = $derived(configured || created !== null);
	const fields = $derived(
		created
			? [
					{ id: 'url', label: 'URL', value: created.url },
					{ id: 'secret', label: 'Secret', value: created.secret }
				]
			: []
	);

	async function generate() {
		busy = true;
		error = '';
		copied = '';
		try {
			created = await createWebhookSecret(fetch, slug);
			// Le panneau qui l'a ouvert relit la connexion : une réouverture dira « configuré ».
			getService(fetch, slug)
				.then(onrefresh)
				.catch(() => {});
		} catch (err) {
			error = errorMessage(err);
		} finally {
			busy = false;
		}
	}

	async function copy(value: string, label: string) {
		try {
			await navigator.clipboard.writeText(value);
			copied = `${label} copié.`;
		} catch {
			copied = `Copie impossible : sélectionne le ${label.toLowerCase()} et copie-le à la main.`;
		}
	}
</script>

<section class="webhook" aria-labelledby="webhook-title">
	<div class="head">
		<h3 id="webhook-title">Webhook</h3>
		<span class="state" class:on={isConfigured}>
			{isConfigured ? 'Webhook configuré' : 'Webhook non configuré'}
		</span>
	</div>
	<p class="muted">
		Request prévient Hub à chaque changement : les demandes se mettent à jour sans attendre.
	</p>

	{#if created}
		<div class="secret">
			<p>
				<strong>Note-le maintenant : le secret ne sera plus affiché.</strong>
				Dans Request : Réglages › Webhook sortant, colle l’URL et mets le secret dans Authorization.
			</p>
			{#each fields as f (f.id)}
				<div class="row">
					<label for="webhook-{f.id}">{f.label}</label>
					<input id="webhook-{f.id}" class="mono" type="text" readonly value={f.value} />
					<button type="button" class="btn btn-base" onclick={() => copy(f.value, f.label)}
						>Copier<span class="visually-hidden"> {f.label.toLowerCase()}</span></button
					>
				</div>
			{/each}
		</div>
	{/if}

	<div>
		<button type="button" class="btn btn-base" disabled={busy} onclick={generate}>
			{busy ? 'Génération…' : isConfigured ? 'Régénérer le secret' : 'Générer le secret'}
		</button>
	</div>
	{#if isConfigured && !created}
		<p class="muted note">Régénérer coupe l’ancien secret : mets à jour Request ensuite.</p>
	{/if}
	<p class="error" role="alert">{error}</p>
	<p class="muted note" role="status" aria-live="polite">{copied}</p>
</section>

<style>
	.webhook {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 18px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-0);
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 4px 12px;
	}
	h3 {
		margin: 0;
		font-size: 18px;
		font-weight: 600;
	}
	p {
		margin: 0;
		font-size: 15px;
	}
	.state {
		color: var(--text-2);
		font-size: 14px;
	}
	.state.on {
		color: var(--text-ok);
	}
	.note {
		font-size: 14px;
	}
	.note:empty {
		display: none;
	}
	.secret {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 14px;
		border: 1px solid var(--tint-accent-border);
		border-radius: 14px;
		background: var(--tint-accent-bg);
	}
	.row {
		display: grid;
		grid-template-columns: 64px minmax(0, 1fr) auto;
		align-items: center;
		gap: 10px;
	}
	label {
		font-size: 14px;
		font-weight: 500;
	}
	input {
		min-width: 0;
		height: 44px;
		box-sizing: border-box;
		padding: 0 12px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--field-bg);
		color: var(--text);
	}
	.btn {
		padding: 0 16px;
		font-size: 15px;
	}
	.error {
		color: var(--text-danger);
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	@media (max-width: 520px) {
		.row {
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.row label {
			grid-column: 1 / -1;
		}
	}
</style>
