<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, getAuthState, login, loginWithPasskey, setup } from '#lib/api/client.ts';
	import { isCancelled, passkeysSupported } from '#lib/api/webauthn.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Mêmes bornes que le serveur (internal/auth) : on prévient avant l'aller-retour.
	const MIN_PASSWORD = 12;
	const MAX_USERNAME = 64;

	// Un 409 (compte créé entre-temps, depuis un autre onglet) bascule en connexion.
	let setupDone = $state(false);
	const setupMode = $derived(data.auth.setupRequired && !setupDone);
	// Le serveur doit les avoir configurées, et le navigateur les gérer (contexte sécurisé).
	const passkeyAvailable = $derived(!setupMode && data.auth.passkeys && passkeysSupported());

	let code = $state('');
	let username = $state('');
	let password = $state('');
	let confirm = $state('');
	let busy = $state(false);
	let error = $state('');
	let notice = $state('');

	function validate(): string {
		const name = username.trim();
		if (!name || [...name].length > MAX_USERNAME)
			return `L'identifiant doit faire entre 1 et ${MAX_USERNAME} caractères.`;
		if (!setupMode) return password ? '' : 'Saisis ton mot de passe.';
		if (!code.trim()) return 'Saisis le code de configuration.';
		if ([...password].length < MIN_PASSWORD)
			return `Le mot de passe doit faire au moins ${MIN_PASSWORD} caractères.`;
		if (password !== confirm) return 'Les deux mots de passe ne correspondent pas.';
		return '';
	}

	function describe(err: unknown): string {
		if (!(err instanceof ApiError)) return 'Serveur injoignable. Vérifie ta connexion et réessaie.';
		switch (err.status) {
			case 400:
				return err.message.startsWith('password')
					? `Le mot de passe doit faire entre ${MIN_PASSWORD} et 256 caractères.`
					: `L'identifiant doit faire entre 1 et ${MAX_USERNAME} caractères.`;
			case 401:
				return 'Identifiant ou mot de passe incorrect.';
			case 403:
				return 'Code de configuration incorrect. Il est affiché dans les logs du serveur.';
			case 429: {
				const minutes = Math.max(1, Math.ceil((err.retryAfter ?? 600) / 60));
				return `Trop de tentatives, réessaie dans ${minutes} min.`;
			}
			default:
				return `Erreur inattendue du serveur (${err.status}).`;
		}
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		error = validate();
		notice = '';
		if (error) return;
		busy = true;
		try {
			if (setupMode) await setup(fetch, code.trim(), username.trim(), password);
			else await login(fetch, username.trim(), password);
			await goto(resolve('/'), { invalidateAll: true });
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				const state = await getAuthState(fetch).catch(() => null);
				setupDone = state ? !state.setupRequired : true;
				password = confirm = code = '';
				notice = 'Le compte existe déjà : connecte-toi.';
			} else {
				error = describe(err);
			}
		} finally {
			busy = false;
		}
	}

	async function signInWithPasskey() {
		error = notice = '';
		busy = true;
		try {
			await loginWithPasskey(fetch);
			await goto(resolve('/'), { invalidateAll: true });
		} catch (err) {
			// Fenêtre du système fermée : l'utilisateur le sait déjà.
			if (isCancelled(err)) return;
			if (err instanceof ApiError && err.status === 401)
				error = 'Passkey refusée. Elle a peut-être été supprimée de ton compte.';
			else if (err instanceof ApiError) error = describe(err);
			else error = 'Cette passkey n’a pas pu être utilisée. Réessaie ou utilise ton mot de passe.';
		} finally {
			busy = false;
		}
	}

	// Focus sur le premier champ du formulaire affiché (le code en configuration).
	function autofocus(node: HTMLInputElement, enabled = true) {
		if (enabled) node.focus();
	}
</script>

<svelte:head><title>{setupMode ? 'Premier démarrage' : 'Connexion'} · Hub</title></svelte:head>

<main class="page">
	<div class="card">
		<div class="brand" aria-hidden="true">H</div>
		{#if setupMode}
			<h1>Bienvenue sur Hub</h1>
			<p class="muted lead">
				Crée le compte administrateur. Le code de configuration est affiché dans les logs du serveur
				au premier démarrage.
			</p>
		{:else}
			<h1>Connexion</h1>
			<p class="muted lead">Ton dashboard est privé : identifie-toi pour continuer.</p>
		{/if}

		{#key setupMode}
			<form onsubmit={submit} novalidate>
				{#if setupMode}
					<label>
						Code de configuration
						<input
							class="mono-input"
							name="code"
							autocomplete="off"
							autocapitalize="characters"
							spellcheck="false"
							required
							bind:value={code}
							use:autofocus
						/>
					</label>
				{/if}
				<label>
					Identifiant
					<input
						name="username"
						autocomplete="username"
						autocapitalize="none"
						spellcheck="false"
						required
						maxlength={MAX_USERNAME}
						bind:value={username}
						use:autofocus={!setupMode}
					/>
				</label>
				<label>
					Mot de passe
					{#if setupMode}<small>{MIN_PASSWORD} caractères minimum</small>{/if}
					<input
						type="password"
						name="password"
						autocomplete={setupMode ? 'new-password' : 'current-password'}
						required
						minlength={setupMode ? MIN_PASSWORD : undefined}
						bind:value={password}
					/>
				</label>
				{#if setupMode}
					<label>
						Confirmation
						<input
							type="password"
							name="confirm"
							autocomplete="new-password"
							required
							bind:value={confirm}
						/>
					</label>
				{/if}

				<p class="msg" class:error={!!error} role={error ? 'alert' : 'status'}>
					{error || notice}
				</p>

				<button type="submit" class="btn primary" disabled={busy}>
					{#if busy}
						{setupMode ? 'Création…' : 'Connexion…'}
					{:else}
						{setupMode ? 'Créer le compte' : 'Se connecter'}
					{/if}
				</button>
			</form>
		{/key}

		{#if passkeyAvailable}
			<div class="or" aria-hidden="true"><span>ou</span></div>
			<button type="button" class="btn secondary" disabled={busy} onclick={signInWithPasskey}>
				<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
					<circle cx="9" cy="8" r="4" fill="none" stroke="currentColor" stroke-width="1.8" />
					<path
						d="M2.5 20c.6-3.6 3.2-6 6.5-6 1.2 0 2.3.3 3.2.9M17 11.5a2.5 2.5 0 1 1 0 5 2.5 2.5 0 0 1 0-5Zm0 5V22m0-2.5h2"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
					/>
				</svg>
				Se connecter avec une passkey
			</button>
		{/if}
	</div>
</main>

<style>
	.page {
		display: grid;
		place-items: center;
		min-height: 100vh;
		padding: 32px var(--space-page);
		background: radial-gradient(80% 50% at 50% 0%, #1b2430 0%, rgba(7, 8, 10, 0) 70%), var(--bg);
	}
	.card {
		display: flex;
		flex-direction: column;
		width: 100%;
		max-width: 420px;
		padding: 32px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
		box-shadow: 0 30px 60px rgba(0, 0, 0, 0.45);
		backdrop-filter: blur(24px) saturate(1.2);
	}
	.brand {
		display: grid;
		place-items: center;
		width: 52px;
		height: 52px;
		margin-bottom: 20px;
		border-radius: 14px;
		background: var(--text);
		color: var(--text-inverse);
		font-size: 24px;
		font-weight: 700;
	}
	h1 {
		margin: 0;
		font-size: 1.75rem;
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	.lead {
		margin: 8px 0 24px;
		font-size: 15px;
		line-height: 1.5;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 14px;
		font-weight: 500;
	}
	small {
		margin-top: -4px;
		color: var(--text-2);
		font-size: var(--fs-small);
		font-weight: 400;
	}
	input {
		min-height: 48px;
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
	.mono-input {
		font-family: var(--font-mono);
		letter-spacing: 0.08em;
	}
	.msg {
		min-height: 1.4em;
		margin: 0;
		color: var(--status-online);
		font-size: 14px;
		line-height: 1.4;
	}
	.msg.error {
		color: #ff8a80;
	}
	.btn {
		min-height: 48px;
		padding: 0 18px;
		border: 0;
		border-radius: 12px;
		font-size: 16px;
		font-weight: 600;
		cursor: pointer;
		transition: transform var(--dur) var(--ease);
	}
	.btn:active:not(:disabled) {
		transform: scale(0.98);
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		background: var(--text);
		color: var(--text-inverse);
	}
	.secondary {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 10px;
		border: 1px solid var(--border-2);
		background: transparent;
		color: var(--text);
	}
	.or {
		display: flex;
		align-items: center;
		gap: 12px;
		margin: 20px 0 16px;
		color: var(--text-2);
		font-size: 13px;
	}
	.or::before,
	.or::after {
		content: '';
		flex: 1;
		border-top: 1px solid var(--border-1);
	}
	@media (max-width: 480px) {
		.card {
			padding: 24px 20px;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.btn {
			transition: none;
		}
	}
</style>
