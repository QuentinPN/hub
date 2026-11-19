import { expect, test } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

test.beforeEach(async ({ page }) => {
	await mockApi(page);
});

test('les onglets mènent aux trois pages', async ({ page }, testInfo) => {
	await page.goto('/');
	const nav = page.getByRole('navigation', {
		name: testInfo.project.name === 'desktop' ? 'Sections' : 'Navigation principale'
	});
	for (const [label, url, heading] of [
		[testInfo.project.name === 'desktop' ? 'Médias' : 'Médias', '/medias', 'Médias'],
		[
			testInfo.project.name === 'desktop' ? 'Téléchargements' : 'Téléch.',
			'/telechargements',
			'Téléchargements'
		],
		['Infra', '/infra', 'Infra']
	] as const) {
		await nav.getByRole('link', { name: label }).click();
		await expect(page).toHaveURL(url);
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
		await expect(nav.locator('[aria-current="page"]')).toContainText(label);
	}
});

test('Médias rassemble lectures, demandes et bibliothèque', async ({ page }) => {
	await page.goto('/medias');
	await expect(page.getByText('Le film du soir')).toBeVisible();
	await expect(
		page.getByText('1 lecture en cours · 1 à venir · 4 demandes à approuver')
	).toBeVisible();
	await expect(page.getByRole('heading', { name: /Demandes/ })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Ouvrir Request' })).toHaveAttribute(
		'href',
		'http://192.0.2.14:8085'
	);
	await expect(page.getByText('412')).toBeVisible();
});

test('Téléchargements montre vitesses, noms et files', async ({ page }, testInfo) => {
	await page.goto('/telechargements');
	await expect(page.getByText('12,4 Mo/s')).toBeVisible();
	await expect(page.getByText('Une série S02E06', { exact: true })).toBeVisible();
	await expect(page.getByText('4 min')).toBeVisible();
	await expect(page.getByText('En attente de sources')).toBeVisible();
	await expect(page.getByRole('link', { name: /12 indexeurs/ })).toHaveAttribute(
		'href',
		'/services/prowlarr#widgets'
	);
	const queue = page.getByRole('heading', { name: /File d’attente/ });
	if (testInfo.project.name === 'desktop') {
		await expect(queue).toBeVisible();
		await expect(page.getByText('À vérifier')).toBeVisible();
	} else {
		await expect(queue).toBeHidden();
	}
});

test('une page sans connexion propose de connecter ses services', async ({ page }) => {
	await page.route('**/api/v1/sections/downloads', (route) =>
		route.fulfill({
			json: {
				connected: 0,
				notices: [],
				toConnect: [
					{
						slug: 'sonarr',
						name: 'Sonarr',
						monogram: 'S',
						icon: 'sonarr',
						color: '#1f6fb2',
						hostId: 'mini-pc',
						hasUrl: true,
						support: 'ready',
						state: '',
						dataAt: null,
						failingSince: null
					}
				],
				transfer: null,
				torrents: [],
				queue: [],
				cards: []
			}
		})
	);
	await page.goto('/telechargements');
	await expect(page.getByText('Rien de connecté pour l’instant.')).toBeVisible();
	await expect(page.getByRole('link', { name: 'Connecter Sonarr' })).toHaveAttribute(
		'href',
		'/services/sonarr#widgets'
	);
});

test('une connexion en panne le dit sans masquer le reste', async ({ page }) => {
	const media = fixture('sections-media') as Record<string, unknown>;
	await page.route('**/api/v1/sections/media', (route) =>
		route.fulfill({
			json: {
				...media,
				notices: [{ slug: 'request', name: 'Request', state: 'auth', dataAt: null }]
			}
		})
	);
	await page.goto('/medias');
	await expect(page.getByRole('link', { name: /Request : clé refusée/ })).toHaveAttribute(
		'href',
		'/services/request#widgets'
	);
	await expect(page.getByText('Le film du soir')).toBeVisible();
});

test('Infra signale ce qui ne va pas, et un hôte muet sans le dire arrêté', async ({
	page
}, testInfo) => {
	await page.goto('/infra');
	await expect(
		page.getByText('1 hôte connecté · 40 conteneurs · 0 services sur 1 sans problème')
	).toBeVisible();
	await expect(page.getByRole('heading', { name: 'À surveiller' })).toBeVisible();
	await expect(page.getByRole('link', { name: /TZ Masters/ }).first()).toHaveAttribute(
		'href',
		'/services/tzmasters'
	);
	await expect(page.getByRole('article', { name: 'Raspberry Pi' })).toContainText('Injoignable');
	const groups = page.getByRole('heading', { name: 'Services par hôte' });
	if (testInfo.project.name === 'desktop') await expect(groups).toBeVisible();
	else await expect(groups).toBeHidden();
});

test('sans problème, Infra n’affiche pas la carte À surveiller', async ({ page }) => {
	const infra = fixture('sections-infra') as Record<string, unknown>;
	await page.route('**/api/v1/sections/infra', (route) =>
		route.fulfill({ json: { ...infra, watch: [] } })
	);
	await page.goto('/infra');
	await expect(page.getByRole('heading', { name: 'Hôtes' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'À surveiller' })).toHaveCount(0);
});

test('les affiches passent par Hub, et l’aplat reste si le service n’en a pas', async ({
	page
}) => {
	const media = fixture('sections-media') as Record<string, unknown>;
	const png = Buffer.from(
		'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
		'base64'
	);
	await page.route('**/api/v1/sections/media', (route) =>
		route.fulfill({
			json: {
				...media,
				latestSlug: 'emby',
				upcoming: [
					{
						title: 'Avec affiche',
						detail: 'Sonarr',
						when: '',
						kind: 'episode',
						tone: '',
						at: '2030-01-08T20:00:00Z',
						poster: '12',
						source: 'sonarr'
					},
					{
						title: 'Sans affiche',
						detail: 'Radarr',
						when: '',
						kind: 'movie',
						tone: '',
						at: '2030-01-09T20:00:00Z',
						poster: '99',
						source: 'radarr'
					}
				]
			}
		})
	);
	await page.route('**/api/v1/posters/sonarr/12', (route) =>
		route.fulfill({ contentType: 'image/png', body: png })
	);
	await page.route('**/api/v1/posters/radarr/99', (route) =>
		route.fulfill({ status: 404, json: { error: 'not found' } })
	);
	await page.goto('/medias');
	const withPoster = page.locator('article.poster', { hasText: 'Avec affiche' });
	await expect(withPoster.locator('img')).toHaveAttribute('src', '/api/v1/posters/sonarr/12');
	await expect
		.poll(() => withPoster.locator('img').evaluate((el: HTMLImageElement) => el.naturalWidth))
		.toBeGreaterThan(0);
	const without = page.locator('article.poster', { hasText: 'Sans affiche' });
	await expect(without).toBeVisible();
	await expect(without.locator('img')).toHaveCount(0);
});

test('Infra affiche les mesures d’un hôte quand Beszel les donne', async ({ page }) => {
	const infra = fixture('sections-infra') as { hosts: Record<string, unknown>[] };
	await page.route('**/api/v1/sections/infra', (route) =>
		route.fulfill({
			json: {
				...infra,
				hasMetrics: true,
				hosts: [
					{
						...infra.hosts[0],
						metrics: [
							{ label: 'CPU', percent: 12 },
							{ label: 'Mémoire', percent: 41 },
							{ label: 'Disque', percent: 78 }
						]
					},
					infra.hosts[1]
				]
			}
		})
	);
	await page.goto('/infra');
	await expect(page.getByRole('meter', { name: 'Disque de mini-PC' })).toHaveAttribute(
		'aria-valuenow',
		'78'
	);
	await expect(page.getByText('78 %')).toBeVisible();
	await expect(page.getByText('arriveront avec le connecteur Beszel')).toHaveCount(0);
});
