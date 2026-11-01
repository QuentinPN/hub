<script lang="ts">
	import { addPasskey, ApiError, deletePasskey, getPasskeys } from '#lib/api/client.ts';
	import type { PasskeyInfo } from '#lib/api/types.ts';
	import { isCancelled, passkeysSupported } from '#lib/api/webauthn.ts';
	import Skeleton from './Skeleton.svelte';

	/** Le serveur a les passkeys configurées (HUB_ORIGIN). */
	let { enabled }: { enabled: boolean } = $props();

	const MAX_NAME = 64;
	const supported = passkeysSupported();

	let passkeys = $state<PasskeyInfo[] | null>(null);
	let busy = $state<number | 'add' | null>(null);
	let confirming = $state<number | null>(null);
	let adding = $state(false);
	let name = $state('');
	let password = $state('');
	let message = $state('');
	let error = $state('');

	const dateFmt = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium' });
	const formatDate = (iso: string) => dateFmt.format(new Date(iso));

	// Nom proposé d'après le système : c'est en général son authentificateur qui répondra.
	function defaultName(): string {
		const ua = navigator.userAgent;
		if (ua.includes('Windows')) return 'Windows Hello';
		if (/iPhone|iPad/.test(ua)) return 'iPhone';
		if (ua.includes('Mac OS X')) return 'Mac';
		if (ua.includes('Android')) return 'Android';
		return 'Ma passkey';
	}

	const reason = (err: unknown) => (err instanceof Error ? err.message : String(err));

	async function load() {
		try {
			passkeys = await getPasskeys(fetch);
		} catch (err) {
			error = `Impossible de charger les passkeys : ${reason(err)}`;
		}
	}
	$effect(() => {
		if (enabled) void load();
	});

	function openForm() {
		adding = true;
		name = defaultName();
		password = message = error = '';
	}

	async function add(event: SubmitEvent) {
		event.preventDefault();
		message = error = '';
		const label = name.trim();
		if (!label || [...label].length > MAX_NAME) {
			error = `Le nom doit faire entre 1 et ${MAX_NAME} caractères.`;
			return;
		}
		if (!password) {
			error = 'Saisis ton mot de passe pour confirmer.';
			return;
		}
		busy = 'add';
		try {
			const created = await addPasskey(fetch, label, password);
			passkeys = [...(passkeys ?? []), created];
			adding = false;
			message = `Passkey « ${created.name} » ajoutée. Tu peux l’utiliser à la prochaine connexion.`;
		} catch (err) {
			if (isCancelled(err)) error = 'Création annulée.';
			else if (err instanceof ApiError && err.status === 403) error = 'Mot de passe incorrect.';
			else if (err instanceof ApiError && err.status === 409)
				error = 'Cette passkey est déjà enregistrée sur ton compte.';
			else if (err instanceof ApiError && err.status === 429)
				error = `Trop de tentatives, réessaie dans ${Math.max(1, Math.ceil((err.retryAfter ?? 600) / 60))} min.`;
			else if (err instanceof ApiError && err.status === 400)
				error = 'La passkey a été refusée par le serveur. Réessaie.';
			else error = `Échec de l’ajout : ${reason(err)}`;
		} finally {
			password = '';
			busy = null;
		}
	}

	async function remove(p: PasskeyInfo) {
		busy = p.id;
		message = error = '';
		try {
			await deletePasskey(fetch, p.id);
			passkeys = passkeys?.filter((x) => x.id !== p.id) ?? null;
			message = `Passkey « ${p.name} » supprimée. Pense à l’effacer aussi de l’appareil.`;
		} catch (err) {
			error = `Échec de la suppression : ${reason(err)}`;
		} finally {
			busy = confirming = null;
		}
	}
</script>

{#if !enabled}
	<p class="muted note">
		Les passkeys demandent une adresse en HTTPS avec un nom de domaine. Définis
		<code>HUB_ORIGIN</code> sur le serveur pour les activer.
	</p>
{:else}
	{#if passkeys === null && !error}
		<div aria-busy="true">
			<Skeleton height="64px" radius="14px" />
		</div>
	{:else if passkeys}
		{#if passkeys.length === 0}
			<p class="muted note">
				Aucune passkey. Ajoute-en une pour te connecter avec Windows Hello, ton téléphone ou ta clé
				de sécurité, sans mot de passe.
			</p>
		{:else}
			<ul class="list">
				{#each passkeys as p (p.id)}
					<li class="item">
						<div class="who">
							<div class="name">
								{p.name}
								<span class="badge">{p.backedUp ? 'Synchronisée' : 'Sur un appareil'}</span>
							</div>
							<div class="muted small">
								Ajoutée le {formatDate(p.createdAt)} ·
								{p.lastUsedAt ? `utilisée le ${formatDate(p.lastUsedAt)}` : 'jamais utilisée'}
							</div>
						</div>
						<div class="row-actions">
							{#if confirming === p.id}
								<button
									type="button"
									class="btn ghost"
									disabled={busy !== null}
									onclick={() => (confirming = null)}>Annuler</button
								>
								<button
									type="button"
									class="btn danger"
									disabled={busy !== null}
									aria-label="Confirmer la suppression de la passkey {p.name}"
									onclick={() => remove(p)}>{busy === p.id ? 'Suppression…' : 'Supprimer'}</button
								>
							{:else}
								<button
									type="button"
									class="btn ghost"
									disabled={busy !== null}
									aria-label="Supprimer la passkey {p.name}"
									onclick={() => (confirming = p.id)}>Supprimer</button
								>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}

	{#if adding}
		<form class="add" onsubmit={add} novalidate>
			<label>
				Nom de la passkey
				<input
					name="passkey-name"
					autocomplete="off"
					maxlength={MAX_NAME}
					required
					bind:value={name}
				/>
			</label>
			<label>
				Mot de passe actuel
				<input
					type="password"
					name="password"
					autocomplete="current-password"
					required
					bind:value={password}
				/>
			</label>
			<div class="form-actions">
				<button
					type="button"
					class="btn ghost"
					disabled={busy !== null}
					onclick={() => (adding = false)}>Annuler</button
				>
				<button type="submit" class="btn primary" disabled={busy !== null}
					>{busy === 'add' ? 'En attente de l’appareil…' : 'Créer la passkey'}</button
				>
			</div>
		</form>
	{/if}
{/if}

<div class="actions">
	<p class="status" class:error={!!error} role={error ? 'alert' : 'status'}>{error || message}</p>
	{#if enabled && !adding}
		<button
			type="button"
			class="btn primary"
			disabled={busy !== null || !supported || passkeys === null}
			title={supported ? undefined : 'Ce navigateur ne gère pas les passkeys sur cette adresse'}
			onclick={openForm}>Ajouter une passkey</button
		>
	{/if}
</div>

<style>
	.list {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 16px 0;
		border-top: 1px solid var(--border-1);
	}
	.item:first-child {
		border-top: 0;
	}
	.who {
		min-width: 0;
	}
	.name {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px;
		margin-bottom: 3px;
		font-size: 16px;
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.badge {
		padding: 2px 10px;
		border-radius: 99px;
		background: rgba(255, 255, 255, 0.12);
		font-size: 12px;
		font-weight: 600;
	}
	.small {
		font-size: var(--fs-small);
		line-height: 1.5;
	}
	.note {
		margin: 0 0 16px;
		font-size: 14px;
		line-height: 1.5;
	}
	code {
		font-family: var(--font-mono);
		font-size: 0.92em;
	}
	.row-actions,
	.form-actions {
		display: flex;
		flex: none;
		gap: 8px;
	}
	.add {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
		padding: 16px 0;
		border-top: 1px solid var(--border-1);
	}
	.form-actions {
		grid-column: 1 / -1;
		justify-content: flex-end;
	}
	label {
		display: flex;
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
	input:focus-visible {
		outline: 2px solid var(--text);
		outline-offset: 1px;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
		padding-top: 16px;
		border-top: 1px solid var(--border-1);
	}
	.status {
		flex: 1;
		min-width: 200px;
		margin: 0;
		color: var(--text-2);
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
	.danger {
		background: #ff8a80;
		color: #1a0b0a;
	}
	@media (max-width: 720px) {
		.item {
			flex-direction: column;
			align-items: flex-start;
			gap: 10px;
		}
		.add {
			grid-template-columns: 1fr;
		}
	}
</style>
