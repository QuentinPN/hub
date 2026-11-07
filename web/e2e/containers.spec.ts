import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];
const ROUTE = '**/api/v1/services/emby/containers';

async function noViolations(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
	expect(violations.map((v) => v.id)).toEqual([]);
}

/** Les conteneurs enregistrés, vus à l'instant, avec ce que le test y change. */
function containers(change: Record<string, unknown> = {}) {
	const base = fixture('services-emby-containers') as Record<string, unknown>;
	return { ...base, lastSeenAt: new Date(Date.now() - 20_000).toISOString(), ...change };
}

const card = (page: Page, id: string) =>
	page.locator('article').filter({ has: page.locator(`#${id}`) });

test.beforeEach(async ({ page }) => {
	await mockApi(page);
});

test('l’onglet montre les conteneurs du service et de ses projets reliés', async ({ page }) => {
	await page.route(ROUTE, (route) => route.fulfill({ json: containers() }));
	await page.goto('/services/emby');
	await page.getByRole('link', { name: 'Conteneurs' }).click();
	await expect(page).toHaveURL(/#conteneurs$/);
	await expect(page.getByRole('link', { name: 'Conteneurs' })).toHaveAttribute(
		'aria-current',
		'page'
	);

	await expect(page.locator('.summary')).toContainText(
		/3 conteneurs sur mini-PC Geekom · 2 en marche · agent connecté, dernier contact il y a \d+\ss/
	);
	await expect(page.getByRole('heading', { level: 2, name: 'media', exact: true })).toBeVisible();
	await expect(page.getByText('projet du service · 2 conteneurs')).toBeVisible();
	await expect(page.getByText('projet relié · 1 conteneur')).toBeVisible();

	const emby = card(page, 'ctr-3f9c1a7be204');
	for (const text of [
		'Principal',
		'emby/embyserver',
		'4.9.0.42',
		'Up 3 days (healthy)',
		'Sain',
		'8096 → 8096/tcp',
		'media · emby',
		'18, dont 2 masqués',
		'12 oct. 2026',
		'mini-PC Geekom'
	])
		await expect(emby).toContainText(text);
	await expect(card(page, 'ctr-a07e55c19d3b')).toContainText('Démarrage');
	await expect(card(page, 'ctr-9e83b2c6f017')).toContainText('Pas de healthcheck');
	await expect(card(page, 'ctr-9e83b2c6f017')).toContainText('Appoint');
	await expect(card(page, 'ctr-9e83b2c6f017')).toContainText('9, dont 1 masqué');

	await expect(page.getByRole('heading', { level: 2, name: 'Disparus' })).toBeVisible();
	await expect(card(page, 'gone-51d0c2e9aa47')).toContainText('disparu le 2 nov. 2026');
	await expect(card(page, 'gone-51d0c2e9aa47')).toContainText('4.8.10.0');
	// Aucune action : l'onglet ne fait que lire.
	await expect(page.locator('article button')).toHaveCount(0);
	await noViolations(page);
});

test('un service non découvert n’a pas de conteneurs', async ({ page }) => {
	await page.goto('/services/emby#conteneurs');
	await expect(page.getByRole('heading', { name: 'Aucun conteneur' })).toBeVisible();
	await expect(page.getByText('pas été découvert par un agent Docker')).toBeVisible();
	await noViolations(page);
});

test('une erreur se réessaie', async ({ page }) => {
	let fail = true;
	await page.route(ROUTE, (route) =>
		fail
			? route.fulfill({ status: 500, json: { error: 'internal error' } })
			: route.fulfill({ json: containers() })
	);
	await page.goto('/services/emby#conteneurs');
	const alert = page.getByRole('alert').filter({ hasText: 'Impossible de charger les conteneurs' });
	await expect(alert).toContainText('internal error');
	await noViolations(page);
	fail = false;
	await alert.getByRole('button', { name: 'Réessayer' }).click();
	await expect(page.getByRole('heading', { level: 2, name: 'media', exact: true })).toBeVisible();
});

test('hôte injoignable : les états datent du dernier contact', async ({ page }) => {
	await page.route(ROUTE, (route) =>
		route.fulfill({
			json: containers({
				hostState: 'offline',
				lastSeenAt: new Date(Date.now() - 12 * 60_000).toISOString()
			})
		})
	);
	await page.goto('/services/emby#conteneurs');
	const banner = page.getByRole('status').filter({ hasText: 'Agent de mini-PC Geekom' });
	await expect(banner).toContainText('injoignable');
	await expect(banner).toContainText(/Dernier contact il y a 12\smin/);
	const emby = card(page, 'ctr-3f9c1a7be204');
	await expect(emby).toContainText('au dernier contact');
	await expect(emby.getByRole('img', { name: 'en cours' })).toHaveCSS(
		'background-color',
		'rgb(142, 142, 147)'
	);
	await noViolations(page);
});

test('le chargement montre des squelettes', async ({ page }) => {
	let release = () => {};
	const held = new Promise<void>((resolve) => (release = resolve));
	await page.route(ROUTE, async (route) => {
		await held;
		await route.fulfill({ json: containers() });
	});
	await page.goto('/services/emby#conteneurs');
	await expect(page.getByLabel('Chargement des conteneurs')).toBeVisible();
	release();
	await expect(page.getByLabel('Chargement des conteneurs')).toBeHidden();
	await expect(page.getByRole('heading', { level: 2, name: 'media', exact: true })).toBeVisible();
});
