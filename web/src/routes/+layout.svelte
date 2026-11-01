<script lang="ts">
	import '@fontsource-variable/inter';
	import '#lib/styles/global.css';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { setUnauthorizedHandler } from '#lib/api/client.ts';
	import { SEARCH_INPUT_ID } from '#lib/catalogue.ts';
	import TopNav from '#lib/components/TopNav.svelte';
	import { shell } from '#lib/shell.svelte.ts';
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();

	const onLogin = $derived(page.url.pathname === resolve('/login'));

	// La barre du haut flotte sur le hero de chaque page : sa hauteur réelle (elle passe sur
	// deux lignes entre mobile et desktop) devient --shell-h, que les pages ajoutent à leur marge.
	let shellHeight = $state(0);

	// Session expirée ou révoquée depuis un autre appareil : retour à la connexion.
	setUnauthorizedHandler(() => {
		if (!onLogin) void goto(resolve('/login'));
	});

	// Ctrl+K ouvre la recherche du catalogue depuis n'importe quelle page. Raccourci Windows
	// voulu par l'utilisateur (DESIGN.md) : on teste ctrlKey, jamais metaKey.
	async function onkeydown(event: KeyboardEvent) {
		if (!event.ctrlKey || event.altKey || event.metaKey || event.key.toLowerCase() !== 'k') return;
		if (onLogin) return;
		event.preventDefault();
		if (page.url.pathname !== resolve('/services')) await goto(resolve('/services'));
		const input = document.getElementById(SEARCH_INPUT_ID);
		if (input instanceof HTMLInputElement) {
			input.focus();
			input.select();
		}
	}
</script>

<svelte:window {onkeydown} />

{#if onLogin}
	{@render children()}
{:else}
	<a class="skip" href="#main">Aller au contenu</a>
	<div class="shell" bind:clientHeight={shellHeight}>
		<TopNav online={shell.summary?.online} total={shell.summary?.total} />
	</div>
	<main id="main" tabindex="-1" style:--shell-h="{shellHeight}px">
		{@render children()}
	</main>
{/if}

<style>
	.skip {
		position: absolute;
		z-index: 20;
		top: 12px;
		left: 12px;
		padding: 10px 16px;
		border-radius: var(--r-button);
		background: var(--text);
		color: var(--text-inverse);
		font-weight: 600;
		transform: translateY(-200%);
	}
	.skip:focus-visible {
		transform: none;
	}
	.shell {
		position: absolute;
		z-index: 5;
		top: 0;
		right: 0;
		left: 0;
		padding: 28px var(--space-page) 0;
	}
	main:focus {
		outline: none;
	}
</style>
