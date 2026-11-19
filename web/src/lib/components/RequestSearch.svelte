<script lang="ts">
	import { errorMessage, requestMedia, searchRequests } from '#lib/api/client.ts';
	import type { RequestSearchItem } from '#lib/api/types.ts';
	import {
		SEARCH_DEBOUNCE_MS,
		isRequestable,
		readOnlyHint,
		searchKey,
		searchPoster,
		searchTerm
	} from '#lib/request.ts';
	import PosterWithActions from './PosterWithActions.svelte';
	import Shelf from './Shelf.svelte';
	import StatusPill from './StatusPill.svelte';

	// `query` est le champ de recherche de la page : Request y répond à son tour.
	let { slug, query, canAct = true }: { slug: string; query: string; canAct?: boolean } = $props();

	const term = $derived(searchTerm(query));

	let items = $state<RequestSearchItem[]>([]);
	let loading = $state(false);
	let error = $state('');
	// Les titres demandés d'ici, par clé : un résultat plus ancien ne les rend pas « Demander ».
	let requested = $state<string[]>([]);
	let busyKey = $state('');
	let announce = $state('');

	// Une recherche par pause dans la frappe ; la réponse d'une frappe dépassée est jetée.
	$effect(() => {
		const current = term;
		let stale = false;
		if (current === null) {
			items = [];
			error = '';
			loading = false;
			return;
		}
		const timer = setTimeout(async () => {
			loading = true;
			error = '';
			try {
				const found = await searchRequests(fetch, slug, current);
				if (stale) return;
				items = found;
				announce = `${found.length} résultat${found.length > 1 ? 's' : ''} dans Request.`;
			} catch (err) {
				if (stale) return;
				items = [];
				error = `Recherche impossible : ${errorMessage(err)}`;
			} finally {
				if (!stale) loading = false;
			}
		}, SEARCH_DEBOUNCE_MS);
		return () => {
			stale = true;
			clearTimeout(timer);
		};
	});

	async function request(item: RequestSearchItem) {
		const key = searchKey(item);
		busyKey = key;
		error = '';
		announce = '';
		try {
			await requestMedia(fetch, slug, { mediaType: item.mediaType, tmdbId: item.tmdbId });
			requested.push(key);
			announce = `${item.title} : demande envoyée.`;
		} catch (err) {
			error = `${item.title} : ${errorMessage(err)}`;
		} finally {
			busyKey = '';
		}
	}
</script>

{#if term !== null}
	<div class="search">
		{#if items.length > 0}
			<Shelf title="Demander dans Request" id="request-search">
				{#each items as item (searchKey(item))}
					<PosterWithActions item={searchPoster(item, slug)}>
						{#if requested.includes(searchKey(item))}
							<StatusPill status="requested" />
						{:else if isRequestable(item)}
							{#if canAct}
								<button
									type="button"
									class="btn btn-base"
									aria-label="Demander {item.title}"
									disabled={busyKey !== ''}
									onclick={() => request(item)}>Demander</button
								>
							{/if}
						{:else if item.status}
							<StatusPill status={item.status} />
						{/if}
					</PosterWithActions>
				{/each}
			</Shelf>
			{#if !canAct}
				<p class="muted">{readOnlyHint('demander')}</p>
			{/if}
		{:else}
			<section aria-labelledby="request-search-empty">
				<h2 id="request-search-empty">Demander dans Request</h2>
				<p class="muted">
					{loading ? 'Recherche…' : error ? '' : 'Aucun titre trouvé dans Request.'}
				</p>
			</section>
		{/if}
		<p class="error" role="alert">{error}</p>
		<p class="visually-hidden" role="status" aria-live="polite">{announce}</p>
	</div>
{/if}

<style>
	.search {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	h2 {
		margin: 0 0 8px;
		font-size: var(--fs-h2);
		font-weight: 600;
		letter-spacing: -0.01em;
	}
	p {
		margin: 0;
	}
	.btn {
		padding: 0 6px;
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
		font-size: 14px;
	}
	.error {
		color: var(--text-danger);
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
</style>
