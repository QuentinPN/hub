<script lang="ts">
	import { onMount } from 'svelte';
	import {
		ApiError,
		deleteWidgetConnection,
		saveWidgetConnection,
		testWidgetKey
	} from '#lib/api/client.ts';
	import type { ServiceDetail, WidgetManifest, WidgetTest } from '#lib/api/types.ts';
	import { refreshLabel, testFailure } from '#lib/widgets.ts';
	import ServiceIcon from './ServiceIcon.svelte';

	let {
		service,
		manifest,
		onclose,
		onsaved
	}: {
		service: ServiceDetail;
		manifest: WidgetManifest;
		onclose: () => void;
		onsaved: (service: ServiceDetail) => void;
	} = $props();

	// Le panneau part des valeurs à l'ouverture : c'est le rôle de cette copie.
	const initial = () => service.widget?.connection;
	const choices = () => service.widget?.refreshChoices ?? [];
	const offered = () => manifest.widgets.map((w) => w.id);
	const needsAccount = () => manifest.auth === 'password';
	let apiKey = $state('');
	let identity = $state('');
	let enabled = $state<string[]>(initial()?.enabled ?? offered());
	let refresh = $state(initial()?.refreshSeconds ?? choices()[0] ?? 30);
	let options = $state<Record<string, string>>({ ...(initial()?.options ?? {}) });

	let test = $state<WidgetTest | null>(null);
	let busy = $state(false);
	let error = $state('');
	let confirmRemove = $state(false);
	let dialog: HTMLDialogElement;

	const accepted = $derived(test?.result === 'ok');
	const canReuseKey = initial() !== undefined && initial()?.state !== 'address_changed';

	$effect(() => {
		dialog.showModal();
	});

	// Une connexion existante reprend sa clé : ses options se relisent sans la retaper.
	onMount(() => {
		if (canReuseKey) void runTest();
	});

	function failed(err: unknown) {
		error = err instanceof ApiError ? err.message : String(err);
	}

	async function runTest() {
		busy = true;
		error = '';
		test = null;
		try {
			test = await testWidgetKey(fetch, service.slug, apiKey, identity);
			for (const o of test.options ?? []) {
				if (!o.choices.some((c) => c.value === options[o.id]) && o.choices[0]) {
					options[o.id] = o.choices[0].value;
				}
			}
		} catch (err) {
			failed(err);
		} finally {
			busy = false;
		}
	}

	async function save() {
		busy = true;
		error = '';
		try {
			const res = await saveWidgetConnection(fetch, service.slug, {
				apiKey,
				...(identity ? { identity } : {}),
				widgets: enabled,
				refreshSeconds: refresh,
				options
			});
			if (res.saved && res.service) {
				onsaved(res.service);
				dialog.close();
				return;
			}
			test = { result: res.result, latencyMs: 0 };
		} catch (err) {
			failed(err);
		}
		busy = false;
	}

	async function remove() {
		if (!confirmRemove) {
			confirmRemove = true;
			return;
		}
		busy = true;
		error = '';
		try {
			onsaved(await deleteWidgetConnection(fetch, service.slug));
			dialog.close();
		} catch (err) {
			failed(err);
			busy = false;
		}
	}

	function toggle(id: string, on: boolean) {
		enabled = on ? [...enabled, id] : enabled.filter((w) => w !== id);
	}
</script>

<dialog bind:this={dialog} aria-labelledby="connect-title" {onclose}>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			void save();
		}}
	>
		<div class="main">
			<header>
				<ServiceIcon
					icon={service.icon}
					monogram={service.monogram}
					color={service.color}
					size={52}
				/>
				<div>
					<h2 id="connect-title">Connecter {service.name}</h2>
					<span class="muted address">{service.url}</span>
				</div>
				<button type="button" class="close" aria-label="Fermer" onclick={() => dialog.close()}
					>×</button
				>
			</header>

			<div class="key">
				{#if needsAccount()}
					<label class="account">
						E-mail du compte
						<input
							bind:value={identity}
							type="text"
							autocomplete="off"
							spellcheck="false"
							placeholder={initial()?.keyHint ?? ''}
							oninput={() => (test = null)}
						/>
					</label>
				{/if}
				<label>
					{needsAccount() ? 'Mot de passe' : 'Clé d’API'}
					<input
						bind:value={apiKey}
						type="password"
						autocomplete="off"
						spellcheck="false"
						placeholder={canReuseKey
							? needsAccount()
								? 'Laisse vide pour le garder'
								: initial()?.keyHint
									? `•••• ${initial()?.keyHint}, laisse vide pour la garder`
									: ''
							: ''}
						aria-describedby="key-help"
						oninput={() => (test = null)}
					/>
				</label>
				<button
					type="button"
					class="btn"
					disabled={busy ||
						(!apiKey.trim() && !canReuseKey) ||
						(needsAccount() && !!apiKey && !identity.trim())}
					onclick={runTest}
				>
					{busy && !test ? 'Test…' : needsAccount() ? 'Tester le compte' : 'Tester la clé'}
				</button>
				<span id="key-help" class="muted help">{manifest.keyHelp}</span>
			</div>

			<div aria-live="polite">
				{#if test && accepted}
					<div class="result good">
						<span class="dot"></span>
						<p>
							<strong>{needsAccount() ? 'Compte accepté' : 'Clé acceptée'}</strong>
							<span class="muted">{test.note}, réponse en {test.latencyMs} ms.</span>
						</p>
					</div>
				{:else if test}
					{@const failure = testFailure(test.result, needsAccount())}
					<div class="result bad">
						<span class="dot"></span>
						<p><strong>{failure.title}</strong><span class="muted">{failure.detail}</span></p>
					</div>
				{/if}
			</div>

			{#if accepted}
				<fieldset>
					<legend>Widgets à afficher</legend>
					{#each manifest.widgets as w (w.id)}
						<label class="check">
							<input
								type="checkbox"
								checked={enabled.includes(w.id)}
								onchange={(e) => toggle(w.id, e.currentTarget.checked)}
							/>
							<span>{w.label} <span class="muted">· {w.where}</span></span>
						</label>
					{/each}
				</fieldset>

				<div class="pair">
					<label>
						Rafraîchir
						<select bind:value={refresh}>
							{#each service.widget?.refreshChoices ?? [] as s (s)}
								<option value={s}>{refreshLabel(s)}</option>
							{/each}
						</select>
					</label>
					{#each test?.options ?? [] as o (o.id)}
						<label>
							{o.label}
							<select bind:value={options[o.id]}>
								{#each o.choices as c (c.value)}
									<option value={c.value}>{c.label}</option>
								{/each}
							</select>
						</label>
					{/each}
				</div>
			{/if}

			<p class="error" role="alert">{error}</p>

			<footer>
				{#if initial()}
					<button type="button" class="link" disabled={busy} onclick={remove}>
						{confirmRemove
							? needsAccount()
								? 'Confirmer : effacer le mot de passe'
								: 'Confirmer : effacer la clé'
							: 'Retirer la connexion'}
					</button>
				{/if}
				<span class="buttons">
					<button type="button" class="btn" disabled={busy} onclick={() => dialog.close()}
						>Annuler</button
					>
					<button
						type="submit"
						class="btn primary"
						disabled={busy || !accepted || enabled.length === 0}
					>
						{busy && accepted ? 'Enregistrement…' : 'Enregistrer'}
					</button>
				</span>
			</footer>
		</div>

		<aside aria-label="Ce que Hub fera">
			<h3>Ce que Hub fera</h3>
			<div>
				<strong>Lire, et rien d’autre</strong>
				<code>
					{#each manifest.reads as read (read)}
						<span>{read}</span>
					{/each}
				</code>
				<span class="muted"
					>{needsAccount()
						? 'Aucune écriture : c’est Hub qui se limite, et le rôle readonly le borne aussi.'
						: 'Aucune écriture. Une clé peut sans doute faire plus : c’est Hub qui se limite.'}</span
				>
			</div>
			<div>
				<strong
					>{needsAccount() ? 'Garder le mot de passe à l’abri' : 'Garder la clé à l’abri'}</strong
				>
				<span class="muted">
					{needsAccount()
						? 'Chiffré en base, jamais renvoyé au navigateur. Seul l’e-mail du compte reste visible.'
						: 'Chiffrée en base, jamais renvoyée au navigateur. Tu ne verras plus que ses 4 derniers caractères.'}
				</span>
			</div>
			<div>
				<strong>Parler depuis le serveur</strong>
				<span class="muted"
					>Ton navigateur ne contacte jamais le service pour le widget, il reçoit seulement les
					données filtrées.</span
				>
			</div>
			<div>
				<strong>Remplacer ou retirer</strong>
				<span class="muted"
					>{needsAccount()
						? 'Un nouveau mot de passe n’est gardé qu’après un test réussi. Retirer la connexion l’efface.'
						: 'Une nouvelle clé n’est gardée qu’après un test réussi. Retirer la connexion efface la clé.'}</span
				>
			</div>
		</aside>
	</form>
</dialog>

<style>
	dialog {
		width: min(1040px, calc(100vw - 48px));
		max-width: none;
		max-height: calc(100dvh - 48px);
		padding: 0;
		overflow: hidden;
		border: 1px solid var(--border-2);
		border-radius: 26px;
		background: #15181b;
		color: var(--text);
	}
	dialog::backdrop {
		background: rgba(0, 0, 0, 0.6);
	}
	form {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
		max-height: calc(100dvh - 48px);
		overflow-y: auto;
	}
	.main {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 30px;
	}
	aside {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 30px;
		background: rgba(255, 255, 255, 0.035);
		font-size: 15px;
	}
	aside div {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	aside strong {
		font-weight: 600;
	}
	h3 {
		margin: 0;
		font-size: 18px;
		font-weight: 600;
	}
	code {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 10px 12px;
		overflow-wrap: anywhere;
		border-radius: 10px;
		background: rgba(0, 0, 0, 0.35);
		font-size: 13px;
		line-height: 1.6;
	}
	header {
		display: flex;
		align-items: center;
		gap: 14px;
	}
	header > div {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	h2 {
		margin: 0;
		font-size: 26px;
		font-weight: 700;
		letter-spacing: -0.015em;
	}
	.address {
		overflow-wrap: anywhere;
		font-size: 15px;
	}
	.close {
		flex: none;
		width: 44px;
		height: 44px;
		border: 0;
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font-size: 20px;
		cursor: pointer;
	}
	.key {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: end;
		gap: 7px 12px;
	}
	.account {
		grid-column: 1 / -1;
	}
	.help {
		grid-column: 1 / -1;
		font-size: 14px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 7px;
		min-width: 0;
		font-size: 15px;
		font-weight: 500;
	}
	input[type='password'],
	input[type='text'],
	select {
		height: 50px;
		min-width: 0;
		box-sizing: border-box;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: 14px;
		background: #24282c;
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 400;
	}
	.result {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 14px 16px;
		border: 1px solid;
		border-radius: 16px;
	}
	.result p {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin: 0;
	}
	.dot {
		flex: none;
		width: 10px;
		height: 10px;
		border-radius: 99px;
		background: currentColor;
	}
	.good {
		border-color: rgba(48, 209, 88, 0.3);
		background: rgba(48, 209, 88, 0.07);
		color: var(--status-online);
	}
	.bad {
		border-color: rgba(255, 69, 58, 0.32);
		background: rgba(255, 69, 58, 0.07);
		color: var(--status-down);
	}
	.result strong {
		color: var(--text);
		font-weight: 600;
	}
	fieldset {
		display: flex;
		flex-direction: column;
		margin: 0;
		padding: 0;
		border: 0;
	}
	legend {
		margin-bottom: 6px;
		padding: 0;
		font-size: 15px;
		font-weight: 500;
	}
	.check {
		flex-direction: row;
		align-items: center;
		gap: 12px;
		min-height: 44px;
		font-weight: 400;
	}
	.check input {
		flex: none;
		width: 20px;
		height: 20px;
		accent-color: var(--accent-play);
	}
	.pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 14px;
	}
	.error {
		margin: 0;
		color: #ff8a80;
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
		margin-top: auto;
	}
	footer .link {
		margin-right: auto;
	}
	.buttons {
		display: flex;
		gap: 10px;
	}
	.btn {
		min-height: 50px;
		padding: 0 20px;
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
		opacity: 0.55;
		cursor: default;
	}
	.link {
		min-height: 44px;
		padding: 0 4px;
		border: 0;
		background: transparent;
		color: var(--text-2);
		font-size: 14px;
		text-decoration: underline;
		cursor: pointer;
	}
	@media (max-width: 860px) {
		dialog {
			width: 100vw;
			height: 100dvh;
			max-height: none;
			margin: 0;
			border: 0;
			border-radius: 0;
		}
		form {
			display: flex;
			flex-direction: column;
			height: 100%;
			max-height: none;
		}
		.main,
		aside {
			padding: 22px 16px;
		}
		.key {
			grid-template-columns: minmax(0, 1fr);
		}
		.pair {
			grid-template-columns: minmax(0, 1fr);
		}
		.buttons {
			flex: 1 1 100%;
		}
		.buttons .btn {
			flex: 1;
		}
	}
</style>
