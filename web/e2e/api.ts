import { existsSync, readFileSync } from 'node:fs';
import type { Page } from '@playwright/test';

// Réponses enregistrées depuis le serveur Go (données d'exemple de server/internal/fixture).
export const fixture = (name: string): unknown =>
	JSON.parse(readFileSync(new URL(`./fixtures/${name}.json`, import.meta.url), 'utf8'));

const routes: Record<string, string> = {
	'auth/state': 'auth-state',
	home: 'home',
	services: 'services',
	'services/emby': 'services-emby',
	'services/prowlarr': 'services-prowlarr',
	settings: 'settings',
	'auth/sessions': 'auth-sessions',
	'auth/passkeys': 'auth-passkeys'
};

/** Simule l'API ; `authenticated: false` reproduit un visiteur sans session. */
export async function mockApi(page: Page, { authenticated = true } = {}) {
	await page.route('**/api/v1/**', async (route) => {
		const path = new URL(route.request().url()).pathname.replace('/api/v1/', '');
		if (path === 'auth/state') {
			const state = fixture('auth-state') as Record<string, unknown>;
			return route.fulfill({ json: { ...state, authenticated } });
		}
		if (path.startsWith('icons/') && authenticated) {
			// Mêmes fichiers que ceux embarqués dans le binaire Go.
			const ref = path.slice('icons/'.length);
			for (const [ext, contentType] of [
				['svg', 'image/svg+xml'],
				['webp', 'image/webp']
			] as const) {
				const file = new URL(`../../server/internal/icons/bundled/${ref}.${ext}`, import.meta.url);
				if (/^[a-z0-9-]+$/.test(ref) && existsSync(file))
					return route.fulfill({ contentType, body: readFileSync(file) });
			}
			return route.fulfill({ status: 404, json: { error: 'not found' } });
		}
		if (!authenticated)
			return route.fulfill({ status: 401, json: { error: 'authentication required' } });
		const name = routes[path];
		if (!name) return route.fulfill({ status: 404, json: { error: 'not found' } });
		return route.fulfill({ json: fixture(name) });
	});
}
