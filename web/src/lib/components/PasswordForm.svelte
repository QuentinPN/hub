<script lang="ts">
	import { ApiError, changePassword } from '#lib/api/client.ts';

	/** Appelé après un changement réussi : les autres sessions viennent d'être fermées. */
	let { username, onchanged }: { username: string; onchanged?: () => void } = $props();

	const MIN_PASSWORD = 12;
	const MAX_BYTES = 256;

	let open = $state(false);
	let busy = $state(false);
	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let message = $state('');
	let error = $state('');

	function openForm() {
		open = true;
		current = next = confirm = message = error = '';
	}

	function check(): string {
		if (!current) return 'Saisis ton mot de passe actuel.';
		if ([...next].length < MIN_PASSWORD)
			return `Le nouveau mot de passe doit faire au moins ${MIN_PASSWORD} caractères.`;
		if (new TextEncoder().encode(next).length > MAX_BYTES)
			return `Le nouveau mot de passe doit faire au plus ${MAX_BYTES} octets.`;
		if (next !== confirm) return 'Les deux nouveaux mots de passe sont différents.';
		return '';
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		message = '';
		error = check();
		if (error) return;
		busy = true;
		try {
			await changePassword(fetch, current, next);
			open = false;
			message = 'Mot de passe changé. Les autres navigateurs ont été déconnectés.';
			onchanged?.();
		} catch (err) {
			if (err instanceof ApiError && err.status === 403) error = 'Mot de passe actuel incorrect.';
			else if (err instanceof ApiError && err.status === 429)
				error = `Trop de tentatives, réessaie dans ${Math.max(1, Math.ceil((err.retryAfter ?? 600) / 60))} min.`;
			else if (err instanceof ApiError && err.status === 400)
				error = 'Le serveur a refusé ce mot de passe. Vérifie sa longueur.';
			else error = `Échec du changement : ${err instanceof Error ? err.message : String(err)}`;
		} finally {
			current = next = confirm = '';
			busy = false;
		}
	}
</script>

{#if open}
	<form class="change" onsubmit={submit} novalidate>
		<!-- Pour le gestionnaire de mots de passe : il sait ainsi quel compte mettre à jour. -->
		<input type="text" name="username" autocomplete="username" value={username} hidden readonly />
		<label class="wide">
			Mot de passe actuel
			<input
				type="password"
				name="current-password"
				autocomplete="current-password"
				required
				bind:value={current}
			/>
		</label>
		<label>
			<span
				>Nouveau mot de passe <small class="muted">{MIN_PASSWORD} caractères minimum</small></span
			>
			<input
				type="password"
				name="new-password"
				autocomplete="new-password"
				minlength={MIN_PASSWORD}
				required
				bind:value={next}
			/>
		</label>
		<label>
			Confirmation du nouveau mot de passe
			<input
				type="password"
				name="confirm-password"
				autocomplete="new-password"
				required
				bind:value={confirm}
			/>
		</label>
		<div class="form-actions">
			<button type="button" class="btn ghost" disabled={busy} onclick={() => (open = false)}
				>Annuler</button
			>
			<button type="submit" class="btn primary" disabled={busy}
				>{busy ? 'Enregistrement…' : 'Changer le mot de passe'}</button
			>
		</div>
	</form>
{:else}
	<p class="muted note">
		Changer le mot de passe déconnecte tous les autres navigateurs. Tes passkeys restent valables.
	</p>
{/if}

<div class="actions">
	<p class="status" class:error={!!error} role={error ? 'alert' : 'status'}>{error || message}</p>
	{#if !open}
		<button type="button" class="btn ghost" onclick={openForm}>Changer le mot de passe</button>
	{/if}
</div>

<style>
	.note {
		margin: 0 0 16px;
		font-size: 14px;
		line-height: 1.5;
	}
	.change {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
		padding: 16px 0;
	}
	.wide,
	.form-actions {
		grid-column: 1 / -1;
	}
	.form-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 14px;
		font-weight: 500;
	}
	small {
		font-weight: 400;
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
	@media (max-width: 720px) {
		.change {
			grid-template-columns: 1fr;
		}
	}
</style>
