import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { getAuthState } from '#lib/api/client.ts';
import { startPath } from '#lib/general.ts';
import { shell } from '#lib/shell.svelte.ts';
import type { LayoutLoad } from './$types';

// SPA pure : le dashboard est privé, servi par le binaire Go, sans rendu serveur.
export const ssr = false;
export const prerender = false;

// Vrai jusqu'à la première page vue connecté : ouvrir Hub (ou s'y connecter) sur « / » mène
// à la page d'ouverture choisie, alors qu'un clic sur « Accueil » reste sur l'accueil.
let opening = true;

// Le serveur protège déjà chaque route /api/ ; cette garde évite seulement d'afficher une
// page vide de données à qui n'est pas connecté.
export const load: LayoutLoad = async ({ fetch, url }) => {
	const auth = await getAuthState(fetch);
	shell.displayName = auth.displayName || 'Hub';
	shell.startPage = auth.startPage ?? 'home';
	shell.widgetRefresh = auth.widgetRefresh ?? 'normal';
	const onLogin = url.pathname === resolve('/login');
	if (!auth.authenticated) {
		opening = true;
		if (!onLogin) redirect(307, resolve('/login'));
		return { auth };
	}
	if (onLogin) redirect(307, resolve('/'));
	if (opening) {
		opening = false;
		const start = startPath(auth.startPage);
		if (url.pathname === resolve('/') && start !== '/') redirect(307, resolve(start));
	}
	return { auth };
};
