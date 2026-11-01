import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { getAuthState } from '#lib/api/client.ts';
import type { LayoutLoad } from './$types';

// SPA pure : le dashboard est privé, servi par le binaire Go, sans rendu serveur.
export const ssr = false;
export const prerender = false;

// Le serveur protège déjà chaque route /api/ ; cette garde évite seulement d'afficher une
// page vide de données à qui n'est pas connecté.
export const load: LayoutLoad = async ({ fetch, url }) => {
	const auth = await getAuthState(fetch);
	const onLogin = url.pathname === resolve('/login');
	if (!auth.authenticated && !onLogin) redirect(307, resolve('/login'));
	if (auth.authenticated && onLogin) redirect(307, resolve('/'));
	return { auth };
};
