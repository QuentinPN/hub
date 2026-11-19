<script lang="ts">
	import { tick } from 'svelte';
	import { errorMessage, approveRequest, declineRequest } from '#lib/api/client.ts';
	import type { PendingRequest, PendingRequests } from '#lib/api/types.ts';
	import { REASON_MAX, cleanReason, pendingPoster, readOnlyHint } from '#lib/request.ts';
	import PosterWithActions from './PosterWithActions.svelte';
	import Shelf from './Shelf.svelte';

	let {
		slug,
		pending,
		canAct = true,
		onwrite
	}: { slug: string; pending: PendingRequests; canAct?: boolean; onwrite?: () => void } = $props();

	// Les demandes traitées d'ici : la relecture du serveur les retire à son tour, mais une
	// relecture arrivée trop tôt ne doit pas les faire revenir.
	let handled = $state<number[]>([]);
	const items = $derived(pending.items.filter((i) => !handled.includes(i.id)));
	const overflow = $derived(Math.max(0, pending.total - pending.items.length));

	let busy = $state(false);
	let declining = $state<PendingRequest | null>(null);
	let reason = $state('');
	let error = $state('');
	let announce = $state('');
	let root: HTMLElement | undefined = $state();
	let field: HTMLTextAreaElement | undefined = $state();

	/** Le focus ne tombe pas dans le vide : il passe à la demande suivante. */
	async function settle(item: PendingRequest, done: string) {
		const at = items.findIndex((i) => i.id === item.id);
		handled.push(item.id);
		declining = null;
		reason = '';
		announce = done;
		onwrite?.();
		await tick();
		const buttons = root?.querySelectorAll<HTMLButtonElement>('[data-approve]');
		buttons?.[Math.min(at, buttons.length - 1)]?.focus();
	}

	async function approve(item: PendingRequest) {
		busy = true;
		error = '';
		announce = '';
		try {
			await approveRequest(fetch, slug, item.id);
			await settle(item, `${item.title} : demande approuvée.`);
		} catch (err) {
			error = `${item.title} : ${errorMessage(err)}`;
		} finally {
			busy = false;
		}
	}

	async function openDecline(item: PendingRequest) {
		error = '';
		announce = '';
		declining = item;
		reason = '';
		await tick();
		field?.focus();
	}

	async function cancelDecline() {
		const item = declining;
		declining = null;
		error = '';
		await tick();
		if (item) root?.querySelector<HTMLButtonElement>(`[data-decline="${item.id}"]`)?.focus();
	}

	async function decline() {
		const item = declining;
		const clean = cleanReason(reason);
		if (!item || clean === null) return;
		busy = true;
		error = '';
		try {
			await declineRequest(fetch, slug, item.id, clean);
			await settle(item, `${item.title} : demande refusée.`);
		} catch (err) {
			error = `${item.title} : ${errorMessage(err)}`;
		} finally {
			busy = false;
		}
	}
</script>

<div class="pending" bind:this={root}>
	{#if items.length > 0}
		<Shelf title="À approuver" id="w-pending">
			{#each items as item (item.id)}
				<PosterWithActions item={pendingPoster(item, slug)}>
					{#if canAct}
						<button
							type="button"
							class="btn btn-base primary"
							data-approve
							aria-label="Approuver {item.title}"
							disabled={busy}
							onclick={() => approve(item)}>Approuver</button
						>
						<button
							type="button"
							class="btn btn-base"
							data-decline={item.id}
							aria-label="Refuser {item.title}"
							aria-expanded={declining?.id === item.id}
							disabled={busy}
							onclick={() => openDecline(item)}>Refuser</button
						>
					{/if}
				</PosterWithActions>
			{/each}
		</Shelf>
		{#if !canAct}
			<p class="muted more">{readOnlyHint('approuver')}</p>
		{/if}
		{#if overflow > 0}
			<p class="muted more">
				{overflow === 1 ? '1 autre demande attend' : `${overflow} autres demandes attendent`} dans Request.
			</p>
		{/if}
	{:else}
		<section class="card" aria-labelledby="w-pending-empty">
			<h2 id="w-pending-empty">À approuver</h2>
			<p class="muted">Aucune demande n’attend d’approbation.</p>
		</section>
	{/if}

	{#if declining}
		<form
			class="decline"
			onsubmit={(e) => {
				e.preventDefault();
				void decline();
			}}
		>
			<label>
				Motif du refus de {declining.title}
				<textarea
					bind:this={field}
					bind:value={reason}
					rows="2"
					required
					maxlength={REASON_MAX}
					disabled={busy}
					aria-describedby="decline-help"
					onkeydown={(e) => {
						if (e.key === 'Escape') void cancelDecline();
					}}></textarea>
			</label>
			<span id="decline-help" class="muted help tabular">
				Obligatoire, il sera montré à la personne qui a demandé. {reason.trim().length}/{REASON_MAX}
			</span>
			<span class="buttons">
				<button
					type="submit"
					class="btn btn-base primary"
					disabled={busy || cleanReason(reason) === null}>Confirmer le refus</button
				>
				<button type="button" class="btn btn-base" disabled={busy} onclick={cancelDecline}
					>Annuler</button
				>
			</span>
		</form>
	{/if}

	<p class="error" role="alert">{error}</p>
	<p class="visually-hidden" role="status" aria-live="polite">{announce}</p>
</div>

<style>
	.pending {
		display: flex;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
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
	p {
		margin: 0;
	}
	.more {
		font-size: 14px;
	}
	.decline {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 18px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 7px;
		font-size: 15px;
		font-weight: 500;
	}
	textarea {
		box-sizing: border-box;
		width: 100%;
		padding: 12px 14px;
		border: 1px solid var(--border-2);
		border-radius: 14px;
		background: var(--field-bg);
		color: var(--text);
		font: inherit;
		font-size: 16px;
		font-weight: 400;
		resize: vertical;
	}
	.help {
		font-size: 14px;
	}
	.buttons {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
	}
	.btn {
		padding: 0 6px;
		font-size: 14px;
	}
	.buttons .btn {
		padding: 0 18px;
		font-size: 15px;
	}
	.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	.error {
		color: var(--text-danger);
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
</style>
