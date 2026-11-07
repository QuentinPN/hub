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
	// Seule action : mettre à part un service Compose du projet du service, jamais d'un projet relié.
	await expect(emby.getByRole('button', { name: 'En faire un service à part…' })).toBeVisible();
	await expect(card(page, 'ctr-9e83b2c6f017').getByRole('button')).toHaveCount(0);
	await expect(card(page, 'gone-51d0c2e9aa47').getByRole('button')).toHaveCount(0);
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

test('un service Compose se met à part, puis se rattache à son projet', async ({ page }) => {
	const emby = fixture('services-emby') as Record<string, unknown>;
	const cache = { ...emby, slug: 'emby-cache', name: 'Emby Cache', url: null };
	const base: Record<string, unknown> = containers();
	const own = (base.containers as Record<string, unknown>[]).find((c) => c.id === 'a07e55c19d3b');
	let sent: unknown = null;
	await page.route(ROUTE, (route) => route.fulfill({ json: base }));
	await page.route('**/api/v1/services/emby/split', async (route) => {
		sent = route.request().postDataJSON();
		await route.fulfill({ json: cache });
	});
	await page.route('**/api/v1/services/emby-cache', (route) => route.fulfill({ json: cache }));
	await page.route('**/api/v1/services/emby-cache/containers', (route) =>
		route.fulfill({
			json: containers({
				containers: [{ ...own, project: 'media/cache', role: 'main' }],
				removed: [],
				removedTotal: 0,
				split: {
					project: 'media',
					service: 'cache',
					since: '2026-11-07T15:00:00+01:00',
					parentSlug: 'emby',
					parentName: 'Emby'
				}
			})
		})
	);
	await page.route('**/api/v1/services/emby-cache/join', (route) => route.fulfill({ json: emby }));

	await page.goto('/services/emby#conteneurs');
	await card(page, 'ctr-a07e55c19d3b')
		.getByRole('button', { name: 'En faire un service à part…' })
		.click();
	const dialog = page.getByRole('dialog', { name: 'Faire de cache un service à part' });
	await expect(dialog).toContainText('Emby garde emby. Rien ne change dans la stack Docker.');
	await expect(dialog.getByLabel('Nom')).toHaveValue('Emby Cache');
	await expect(dialog.getByLabel('Catégorie')).toHaveValue('Médias');
	await expect(dialog).toContainText('Aucun port publié');
	await noViolations(page);

	await dialog.getByRole('button', { name: 'Créer le service' }).click();
	await expect(page).toHaveURL(/\/services\/emby-cache#conteneurs$/);
	expect(sent).toEqual({ service: 'cache', name: 'Emby Cache', category: 'Médias' });
	await expect(page.getByRole('heading', { level: 2, name: 'media', exact: true })).toBeVisible();
	await expect(page.getByText('séparé du projet le 7 nov. 2026')).toBeVisible();
	await expect(page.getByRole('link', { name: 'Emby', exact: true })).toHaveAttribute(
		'href',
		'/services/emby#conteneurs'
	);
	await expect(page.getByRole('button', { name: 'En faire un service à part…' })).toHaveCount(0);
	await noViolations(page);

	await page.getByRole('button', { name: 'Rattacher à Emby…' }).click();
	const confirm = page.getByRole('alertdialog', { name: 'Rattacher cache à Emby ?' });
	await expect(confirm).toContainText('Son historique est gardé.');
	await confirm.getByRole('button', { name: 'Rattacher' }).click();
	await expect(page).toHaveURL(/\/services\/emby#conteneurs$/);
});

test('une séparation refusée le dit dans le dialogue', async ({ page }) => {
	await page.route(ROUTE, (route) => route.fulfill({ json: containers() }));
	await page.route('**/api/v1/services/emby/split', (route) =>
		route.fulfill({
			status: 400,
			json: { error: 'ce conteneur ne peut pas devenir un service à part' }
		})
	);
	await page.goto('/services/emby#conteneurs');
	await card(page, 'ctr-3f9c1a7be204')
		.getByRole('button', { name: 'En faire un service à part…' })
		.click();
	const dialog = page.getByRole('dialog', { name: 'Faire de emby un service à part' });
	await dialog.getByLabel('Nom').fill('Serveur');
	await dialog.getByRole('button', { name: 'Créer le service' }).click();
	await expect(dialog.getByRole('alert')).toContainText('ne peut pas devenir un service à part');
	await expect(page).toHaveURL(/\/services\/emby#conteneurs$/);
});
