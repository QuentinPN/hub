<script lang="ts">
	import '@fontsource-variable/inter';
	import '#lib/styles/global.css';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { setUnauthorizedHandler } from '#lib/api/client.ts';
	import { SEARCH_INPUT_ID } from '#lib/catalogue.ts';
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();

	// Session expirée ou révoquée depuis un autre appareil : retour à la connexion.
	setUnauthorizedHandler(() => {
		if (page.url.pathname !== resolve('/login')) void goto(resolve('/login'));
	});

	// Ctrl+K ouvre la recherche du catalogue depuis n'importe quelle page. Raccourci Windows
	// voulu par l'utilisateur (DESIGN.md) : on teste ctrlKey, jamais metaKey.
	async function onkeydown(event: KeyboardEvent) {
		if (!event.ctrlKey || event.altKey || event.metaKey || event.key.toLowerCase() !== 'k') return;
		event.preventDefault();
		if (page.url.pathname !== '/services') await goto(resolve('/services'));
		const input = document.getElementById(SEARCH_INPUT_ID);
		if (input instanceof HTMLInputElement) {
			input.focus();
			input.select();
		}
	}
</script>

<svelte:window {onkeydown} />

{@render children()}
