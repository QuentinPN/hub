import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { mockApi } from './api.ts';

const manifest = {
	kind: 'emby',
	name: 'Emby',
	keyHelp: 'Dans Emby : tableau de bord du serveur, Clés d’API.',
	reads: ['GET /emby/Sessions', 'GET /emby/Items/Counts'],
	widgets: [
		{ id: 'sessions', label: 'Sessions en cours', where: 'accueil et fiche' },
		{ id: 'latest', label: 'Ajoutés récemment', where: 'fiche' },
		{ id: 'library', label: 'Bibliothèque', where: 'fiche' }
	]
};

const base = {
	slug: 'emby',
	name: 'Emby',
	monogram: 'E',
	icon: 'emby',
	color: '#2e7d4f',
	category: 'Médias',
	hostId: 'mini-pc',
	status: 'online',
	latencyMs: 42,
	note: 'en ligne',
	favorite: false,
	url: 'http://192.0.2.14:8096',
	version: '',
	tint: '#1f2a36',
	stats: [],
	upcoming: [],
	identification: [{ label: 'Reconnu par', value: 'image emby' }],
	access: [{ label: 'emby', value: 'en cours · principal' }],
	connection: [{ label: 'Widget', value: 'non configuré' }],
	discovered: true
};

const offered = { ...base, widget: { support: 'ready', manifest, refreshChoices: [30, 60, 300] } };

const connection = (extra: Record<string, unknown> = {}) => ({
	state: 'ok',
	keyHint: '7f2a',
	refreshSeconds: 30,
	enabled: ['sessions', 'latest', 'library'],
	options: { userId: 'u1' },
	dataAt: new Date(Date.now() - 12_000).toISOString(),
	checkedAt: new Date().toISOString(),
	failingSince: null,
	data: {
		sessions: [
			{
				title: 'Le film du soir',
				subtitle: '',
				device: 'Apple TV',
				progress: 0.58,
				durationLabel: '42 min restantes',
				transcoding: false
			}
		],
		latest: [{ title: 'Un film', detail: 'Film', kind: 'movie', addedAt: '2026-11-05T10:00:00Z' }],
		library: { movies: 120, series: 30, episodes: 900 }
	},
	...extra
});

const connected = (extra: Record<string, unknown> = {}) => ({
	...base,
	widget: { ...offered.widget, connection: connection(extra) }
});

async function openTab(page: Page, service: unknown) {
	await mockApi(page);
	await page.route('**/api/v1/services/emby', (route) => route.fulfill({ json: service }));
	await page.goto('/services/emby#widgets');
}

test('un signal du serveur met la fiche à jour sans attendre', async ({ page }) => {
	await openTab(page, connected());
	await expect(page.getByText('Le film du soir')).toBeVisible();
	const next = connected({
		data: {
			sessions: [
				{
					title: 'Un autre film',
					subtitle: '',
					device: 'iPad',
					progress: 0.1,
					durationLabel: '90 min restantes',
					transcoding: false
				}
			]
		}
	});
	await page.route('**/api/v1/services/emby', (route) => route.fulfill({ json: next }));
	await page.route('**/api/v1/events', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: 'event: widgets\ndata: autre\n\nevent: widgets\ndata: emby\n\n'
		})
	);
	await page.reload();
	await expect(page.getByText('Un autre film')).toBeVisible();
	await page.route('**/api/v1/services/emby', (route) =>
		route.fulfill({ json: connected({ data: { sessions: [] } }) })
	);
	// La réponse se termine : le navigateur se reconnecte, et le signal suivant recharge.
	await expect(page.getByText('Rien en lecture.')).toBeVisible({ timeout: 10_000 });
});

test('les onglets de la fiche passent d’Aperçu à Widgets', async ({ page }) => {
	await openTab(page, offered);
	const nav = page.getByRole('navigation', { name: 'Sections du service' });
	await expect(nav.locator('[aria-current="page"]')).toHaveText('Widgets');
	await nav.getByRole('link', { name: 'Aperçu' }).click();
	await expect(nav.locator('[aria-current="page"]')).toHaveText('Aperçu');
	await expect(page.getByRole('heading', { name: 'Identification' })).toBeVisible();
});

test('une clé se teste, puis s’enregistre sans jamais revenir', async ({ page }) => {
	await openTab(page, offered);
	const tests: unknown[] = [];
	const saves: unknown[] = [];
	await page.route('**/api/v1/services/emby/connection/test', async (route) => {
		tests.push(route.request().postDataJSON());
		await route.fulfill({
			json: {
				result: 'ok',
				latencyMs: 58,
				note: '1 lecture en cours',
				options: [
					{
						id: 'userId',
						label: 'Compte pour les nouveautés',
						choices: [{ value: 'u1', label: 'admin' }]
					}
				]
			}
		});
	});
	await page.route('**/api/v1/services/emby/connection', async (route) => {
		saves.push(route.request().postDataJSON());
		await route.fulfill({ json: { saved: true, result: 'ok', service: connected() } });
	});

	await page.getByRole('button', { name: 'Connecter Emby' }).click();
	const dialog = page.getByRole('dialog', { name: 'Connecter Emby' });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByRole('button', { name: 'Enregistrer' })).toBeDisabled();
	await expect(dialog.getByText('GET /emby/Sessions')).toBeVisible();
	const axe = await new AxeBuilder({ page }).include('dialog').analyze();
	expect(axe.violations).toEqual([]);

	const field = dialog.getByLabel('Clé d’API');
	await expect(field).toHaveAttribute('type', 'password');
	await field.fill('une-cle-de-test');
	await dialog.getByRole('button', { name: 'Tester la clé' }).click();
	await expect(dialog.getByText('Clé acceptée')).toBeVisible();
	await expect(dialog.getByText('1 lecture en cours, réponse en 58 ms.')).toBeVisible();
	expect(tests).toEqual([{ apiKey: 'une-cle-de-test' }]);

	await dialog.getByLabel(/Ajoutés récemment/).uncheck();
	await dialog.getByRole('button', { name: 'Enregistrer' }).click();
	await expect(dialog).toHaveCount(0);
	expect(saves).toEqual([
		{
			apiKey: 'une-cle-de-test',
			widgets: ['sessions', 'library'],
			refreshSeconds: 30,
			options: { userId: 'u1' }
		}
	]);

	await expect(page.getByRole('heading', { name: 'Sessions en cours' })).toBeVisible();
	await expect(page.getByText('Le film du soir')).toBeVisible();
	await expect(page.getByText('•••• 7f2a')).toBeVisible();
	await expect(page.getByText('une-cle-de-test')).toHaveCount(0);
});

test('une clé refusée ne laisse rien enregistrer', async ({ page }) => {
	await openTab(page, offered);
	await page.route('**/api/v1/services/emby/connection/test', (route) =>
		route.fulfill({ json: { result: 'auth', latencyMs: 12 } })
	);
	await page.getByRole('button', { name: 'Connecter Emby' }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Clé d’API').fill('mauvaise');
	await dialog.getByRole('button', { name: 'Tester la clé' }).click();
	await expect(dialog.getByText('Clé refusée')).toBeVisible();
	await expect(dialog.getByRole('button', { name: 'Enregistrer' })).toBeDisabled();
	await expect(dialog.getByLabel(/Sessions en cours/)).toHaveCount(0);
});

test('sans adresse, la fiche la demande avant la clé', async ({ page }) => {
	await openTab(page, { ...offered, url: null });
	await expect(page.getByText('Donne d’abord l’adresse du service')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Connecter Emby' })).toHaveCount(0);
});

test('un service injoignable garde ses données, datées', async ({ page }) => {
	await openTab(page, connected({ state: 'unreachable', failingSince: new Date().toISOString() }));
	await expect(page.getByLabel('État du widget')).toContainText('Injoignable');
	await expect(page.getByLabel('État du widget')).toContainText('Données il y a');
	await expect(page.getByText('Le film du soir')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Corriger la connexion' })).toHaveCount(0);
});

test('une clé refusée n’affiche plus rien et propose de corriger', async ({ page }) => {
	await openTab(page, connected({ state: 'auth', data: {} }));
	await expect(page.getByLabel('État du widget')).toContainText('Clé refusée');
	await expect(page.getByText('Le film du soir')).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Corriger la connexion' })).toBeVisible();
});

test('une connexion existante reprend sa clé et peut se retirer', async ({ page }) => {
	await openTab(page, connected());
	const tests: unknown[] = [];
	let removed = 0;
	await page.route('**/api/v1/services/emby/connection/test', async (route) => {
		tests.push(route.request().postDataJSON());
		await route.fulfill({
			json: { result: 'ok', latencyMs: 20, note: 'Aucune lecture en cours', options: [] }
		});
	});
	await page.route('**/api/v1/services/emby/connection', async (route) => {
		removed++;
		await route.fulfill({ json: offered });
	});
	await page.getByRole('button', { name: 'Gérer la connexion' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText('Clé acceptée')).toBeVisible();
	expect(tests).toEqual([{ apiKey: '' }]);
	await expect(dialog.getByLabel('Clé d’API')).toHaveValue('');

	await dialog.getByRole('button', { name: 'Retirer la connexion' }).click();
	expect(removed).toBe(0);
	await dialog.getByRole('button', { name: 'Confirmer : effacer la clé' }).click();
	await expect(dialog).toHaveCount(0);
	expect(removed).toBe(1);
	await expect(page.getByRole('button', { name: 'Connecter Emby' })).toBeVisible();
});

test('un service sans connecteur le dit clairement', async ({ page }) => {
	await openTab(page, { ...base, widget: { support: 'link' } });
	await expect(page.getByRole('heading', { name: 'Lien seulement' })).toBeVisible();
	await page.route('**/api/v1/services/emby', (route) =>
		route.fulfill({ json: { ...base, widget: { support: 'planned' } } })
	);
	await page.reload();
	await expect(page.getByRole('heading', { name: 'Widget à venir' })).toBeVisible();
});

test('les réglages listent les connexions et mènent à la fiche', async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings');
	const panel = page.locator('#widgets');
	await expect(panel.getByRole('heading', { name: 'Widgets et connexions' })).toBeVisible();
	await expect(panel.getByText('Widget à venir')).toBeVisible();
	await expect(panel.getByText('Lien seulement : pas d’API confirmée')).toBeVisible();
	await expect(panel.getByRole('link')).toHaveCount(1);
	await expect(panel.getByRole('link', { name: 'Gérer Emby' })).toHaveAttribute(
		'href',
		'/services/emby#widgets'
	);
});

test('l’accueil mène à la fiche du service qui lit les sessions', async ({ page }) => {
	await mockApi(page);
	const session = {
		title: 'Le film du soir',
		subtitle: '',
		device: 'Apple TV',
		progress: 0.5,
		durationLabel: '42 min restantes',
		transcoding: false
	};
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({
			json: {
				summary: { online: 1, total: 1 },
				sessions: [session],
				sessionsSlug: 'emby-qnap',
				favorites: [],
				services: [],
				upcoming: [],
				hosts: []
			}
		})
	);
	await page.goto('/');
	await expect(page.getByRole('heading', { level: 1, name: 'Le film du soir' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Ouvrir la fiche Emby' })).toHaveAttribute(
		'href',
		'/services/emby-qnap'
	);
});

test('l’accueil rassemble ce qui arrive et ce qui se télécharge', async ({ page }) => {
	await mockApi(page);
	const tomorrow = new Date(Date.now() + 86_400_000).toISOString();
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({
			json: {
				summary: { online: 2, total: 2 },
				sessions: [],
				favorites: [],
				services: [],
				upcoming: [
					{
						title: 'Une série',
						detail: 'S02E06 · Sonarr',
						when: '',
						kind: 'episode',
						tone: '',
						at: tomorrow
					}
				],
				downloads: [
					{
						slug: 'sonarr',
						label: 'Sonarr · file d’attente',
						headline: '2 épisodes',
						note: '1 en cours · 1 en attente',
						state: 'ok',
						dataAt: new Date().toISOString()
					},
					{
						slug: 'radarr',
						label: 'Radarr · file d’attente',
						headline: 'File vide',
						note: 'Rien à récupérer',
						state: 'unreachable',
						dataAt: new Date(Date.now() - 480_000).toISOString()
					}
				],
				hosts: []
			}
		})
	);
	await page.goto('/');
	await expect(page.getByText('Demain')).toBeVisible();
	await expect(page.getByText('S02E06 · Sonarr')).toBeVisible();
	const section = page.getByRole('region', { name: 'Téléchargements' });
	await expect(section.getByRole('link', { name: /2 épisodes/ })).toHaveAttribute(
		'href',
		'/services/sonarr#widgets'
	);
	await expect(section.getByText('1 en cours · 1 en attente')).toBeVisible();
	const stale = section.getByRole('link', { name: /Radarr/ });
	await expect(stale).toContainText('Injoignable, données il y a 8 min');
	await expect(stale).not.toContainText('Rien à récupérer');
	const axe = await new AxeBuilder({ page }).include('main').analyze();
	expect(axe.violations).toEqual([]);
});

test('la fiche de Sonarr montre son calendrier et sa file', async ({ page }) => {
	const service = {
		...base,
		slug: 'sonarr',
		name: 'Sonarr',
		widget: {
			...offered.widget,
			connection: connection({
				enabled: ['calendar', 'queue'],
				data: {
					calendar: [
						{
							title: 'Une série',
							detail: 'S02E06 · Sonarr',
							when: '',
							kind: 'episode',
							tone: '',
							at: new Date(Date.now() + 86_400_000).toISOString()
						}
					],
					queue: { total: 2, downloading: 1, problems: 0 }
				}
			})
		}
	};
	await mockApi(page);
	await page.route('**/api/v1/services/sonarr', (route) => route.fulfill({ json: service }));
	await page.goto('/services/sonarr#widgets');
	await expect(page.getByRole('heading', { name: 'À venir' })).toBeVisible();
	await expect(page.getByText('Demain')).toBeVisible();
	await expect(page.getByRole('heading', { name: 'File d’attente' })).toBeVisible();
	await expect(page.getByText('En cours')).toBeVisible();
});

test('la fiche de qBittorrent montre ses transferts, sans aucun nom', async ({ page }) => {
	const service = {
		...base,
		slug: 'qbittorrent',
		name: 'qBittorrent',
		widget: {
			...offered.widget,
			connection: connection({
				enabled: ['transfer'],
				data: {
					transfer: {
						downSpeed: 12_400_000,
						upSpeed: 850_000,
						downloading: 3,
						connection: 'firewalled'
					}
				}
			})
		}
	};
	await mockApi(page);
	await page.route('**/api/v1/services/qbittorrent', (route) => route.fulfill({ json: service }));
	await page.goto('/services/qbittorrent#widgets');
	await expect(page.getByRole('heading', { name: 'Transferts' })).toBeVisible();
	await expect(page.getByText('12,4 Mo/s')).toBeVisible();
	await expect(page.getByText('850 Ko/s')).toBeVisible();
	await expect(page.getByText('derrière un pare-feu')).toBeVisible();
});
