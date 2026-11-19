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
	'settings/general': 'settings-general',
	widgets: 'widgets',
	status: 'status',
	'home-layout': 'home-layout',
	'sections/media': 'sections-media',
	'sections/downloads': 'sections-downloads',
	'sections/infra': 'sections-infra',
	'auth/sessions': 'auth-sessions',
	'auth/passkeys': 'auth-passkeys',
	hosts: 'hosts',
	backup: 'backup',
	notifications: 'notifications',
	'hosts/qnap/containers': 'hosts-qnap-containers'
};

// Les dates des hôtes sont écrites pour ce moment-là ; elles sont décalées à maintenant pour
// que « vu il y a 8 s » et le compte à rebours du code gardent leur sens.
const HOSTS_RECORDED_AT = Date.parse('2026-11-02T18:00:00Z');

export function hostsNow(): unknown {
	const shift = Date.now() - HOSTS_RECORDED_AT;
	return JSON.parse(JSON.stringify(fixture('hosts')), (_key, value: unknown) =>
		typeof value === 'string' && /^\d{4}-\d\d-\d\dT/.test(value)
			? new Date(Date.parse(value) + shift).toISOString()
			: value
	);
}

/**
 * Remplace le flux de signaux du serveur par un faux, à brancher avant la navigation ;
 * `emitWidgets` y fait passer un signal « du nouveau pour ce service ».
 */
export async function mockEvents(page: Page) {
	await page.addInitScript(() => {
		type Listener = (event: { data: string }) => void;
		class FakeEventSource {
			static CLOSED = 2;
			static sources: FakeEventSource[] = [];
			readyState = 1;
			listeners: Record<string, Listener[]> = {};
			constructor() {
				FakeEventSource.sources.push(this);
			}
			addEventListener(type: string, listener: Listener) {
				(this.listeners[type] ??= []).push(listener);
			}
			close() {
				this.readyState = FakeEventSource.CLOSED;
			}
		}
		Object.assign(window, {
			EventSource: FakeEventSource,
			__emitWidgets: (slug: string) => {
				for (const source of FakeEventSource.sources)
					for (const listener of source.listeners.widgets ?? []) listener({ data: slug });
			}
		});
	});
}

export const emitWidgets = (page: Page, slug: string) =>
	page.evaluate(
		(s) => (window as unknown as { __emitWidgets(slug: string): void }).__emitWidgets(s),
		slug
	);

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
		if (path === 'hosts') return route.fulfill({ json: hostsNow() });
		// Un 204 dit au navigateur de ne pas se reconnecter : pas de signal, comme sans nouveauté.
		if (path === 'events') return route.fulfill({ status: 204 });
		const name = routes[path];
		if (!name) return route.fulfill({ status: 404, json: { error: 'not found' } });
		return route.fulfill({ json: fixture(name) });
	});
}
