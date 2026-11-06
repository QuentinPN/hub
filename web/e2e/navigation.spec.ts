import { expect, test } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

test.beforeEach(async ({ page }) => {
	await mockApi(page);
});

test('le lien d’évitement est le premier arrêt et mène au contenu', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
	await page.keyboard.press('Tab');
	const skip = page.getByRole('link', { name: 'Aller au contenu' });
	await expect(skip).toBeFocused();
	await expect(skip).toBeInViewport();
	await page.keyboard.press('Enter');
	await expect(page.locator('main#main')).toBeFocused();
});

test('une seule bannière, un seul main, la page courante est signalée', async ({
	page
}, testInfo) => {
	await page.goto('/services');
	await expect(page.getByRole('banner')).toHaveCount(1);
	await expect(page.getByRole('main')).toHaveCount(1);

	// Desktop : onglets en pilule en haut ; mobile : barre d'onglets en bas.
	const desktop = testInfo.project.name === 'desktop';
	const nav = page.getByRole('navigation', {
		name: desktop ? 'Sections' : 'Navigation principale'
	});
	const hidden = page.getByRole('navigation', {
		name: desktop ? 'Navigation principale' : 'Sections'
	});
	await expect(nav).toBeVisible();
	await expect(hidden).toBeHidden();
	await expect(nav.locator('[aria-current="page"]')).toHaveText(
		desktop ? 'Tous les services' : 'Services'
	);

	await nav.getByRole('link', { name: 'Accueil' }).click();
	await expect(page).toHaveURL('/');
	await expect(nav.locator('[aria-current="page"]')).toHaveText('Accueil');

	await page.getByRole('link', { name: 'Réglages' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Réglages' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Réglages' })).toHaveAttribute(
		'aria-current',
		'page'
	);
});

test('Ctrl+K ouvre la recherche du catalogue depuis une autre page', async ({ page }) => {
	await page.goto('/settings');
	await expect(page.getByRole('heading', { name: 'Réglages' })).toBeVisible();
	await page.keyboard.press('Control+k');
	await expect(page).toHaveURL('/services');
	const search = page.getByRole('searchbox', { name: 'Rechercher un service' });
	await expect(search).toBeFocused();
	await search.pressSequentially('sonarr');
	await expect(page.getByRole('link', { name: /Sonarr/ })).toHaveCount(1);
	await expect(page.getByText('1 service affiché')).toBeAttached();
});

test('Ctrl+K sur le catalogue resélectionne la recherche', async ({ page }) => {
	await page.goto('/services');
	const search = page.getByRole('searchbox', { name: 'Rechercher un service' });
	await search.fill('radarr');
	await page.getByRole('button', { name: /^En ligne/ }).focus();
	await page.keyboard.press('Control+k');
	await expect(search).toBeFocused();
	const selected = await search.evaluate((el: HTMLInputElement) =>
		el.value.slice(el.selectionStart ?? 0, el.selectionEnd ?? 0)
	);
	expect(selected).toBe('radarr');
});

test('le clavier ouvre une fiche et le retour ramène au catalogue', async ({ page }) => {
	await page.goto('/services');
	const card = page.getByRole('link', { name: /Emby/ });
	await card.focus();
	await page.keyboard.press('Enter');
	await expect(page).toHaveURL('/services/emby');
	await expect(page.getByRole('heading', { level: 1, name: 'Emby' })).toBeVisible();
	await page.getByRole('link', { name: '‹ Tous les services' }).click();
	await expect(page).toHaveURL('/services');
});

test('une fiche ouverte depuis l’accueil ramène à l’accueil', async ({ page }) => {
	await page.goto('/');
	await page.getByRole('link', { name: 'Ouvrir la fiche Emby' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Emby' })).toBeVisible();
	await page.getByRole('link', { name: '‹ Accueil' }).click();
	await expect(page).toHaveURL('/');
});

test('une fiche chargée directement ramène à l’accueil, sans erreur', async ({ page }) => {
	const errors: string[] = [];
	page.on('pageerror', (err) => errors.push(err.message));
	await page.goto('/services/emby');
	await expect(page.getByRole('heading', { level: 1, name: 'Emby' })).toBeVisible();
	await expect(page.getByRole('link', { name: '‹ Accueil' })).toBeVisible();
	expect(errors).toEqual([]);
});

test('le titre de l’onglet suit la page', async ({ page }) => {
	await page.goto('/services/emby');
	await expect(page).toHaveTitle('Emby · Hub');
	await page.goto('/services');
	await expect(page).toHaveTitle('Tous les services · Hub');
});

test('chaque service affiche son icône selfh.st embarquée', async ({ page }) => {
	await page.goto('/services');
	const icons = page.locator('.app img');
	await expect(icons).toHaveCount(34);
	await expect(page.locator('.app', { hasText: 'Emby' }).locator('img')).toHaveAttribute(
		'src',
		'/api/v1/icons/emby'
	);
	// Toutes chargées depuis notre origine : rien ne part vers un CDN.
	const broken = await icons.evaluateAll((imgs) =>
		(imgs as HTMLImageElement[])
			.filter(
				(img) => !img.complete || img.naturalWidth === 0 || !img.src.startsWith(location.origin)
			)
			.map((img) => img.src)
	);
	expect(broken).toEqual([]);
});

test('sans icône connue, le monogramme reste affiché', async ({ page }) => {
	const list = fixture('services') as { slug: string; icon: string | null }[];
	for (const s of list) if (s.slug === 'emby') s.icon = null;
	for (const s of list) if (s.slug === 'sonarr') s.icon = 'inconnue-de-l-ui';
	await page.route('**/api/v1/services', (route) => route.fulfill({ json: list }));
	await page.goto('/services');
	await expect(page.locator('.app', { hasText: 'Emby' }).locator('img')).toHaveCount(0);
	await expect(page.locator('.app', { hasText: 'Emby' }).locator('.mono-tile')).toHaveText('E');
	await expect(page.locator('.app', { hasText: 'Sonarr' }).locator('.mono-tile')).toHaveText('S');
});

test.describe('sans session', () => {
	test.beforeEach(async ({ page }) => {
		await page.unrouteAll();
		await mockApi(page, { authenticated: false });
	});

	test('renvoie à la connexion, sans barre de navigation ni Ctrl+K', async ({ page }) => {
		await page.goto('/services');
		await expect(page).toHaveURL('/login');
		await expect(page.getByRole('heading', { level: 1, name: 'Connexion' })).toBeVisible();
		await expect(page.getByRole('banner')).toHaveCount(0);
		await page.keyboard.press('Control+k');
		await expect(page).toHaveURL('/login');
	});
});

test('sans favori, l’accueil liste les services découverts', async ({ page }) => {
	await mockApi(page);
	const service = {
		slug: 'radarr',
		name: 'Radarr',
		monogram: 'R',
		icon: null,
		color: '#b8860b',
		category: 'Téléchargements',
		hostId: 'mini-pc',
		status: 'config_required',
		latencyMs: null,
		note: 'port 7878',
		favorite: false
	};
	const host = {
		id: 'mini-pc',
		name: 'mini-PC',
		os: '',
		containers: 36,
		reachable: true,
		lastSeenLabel: 'il y a 3 s',
		metrics: []
	};
	const home = {
		summary: { online: 1, total: 1 },
		sessions: [],
		favorites: [],
		services: [service],
		upcoming: [],
		hosts: [host]
	};
	await page.route('**/api/v1/home', (route) => route.fulfill({ json: home }));

	await page.goto('/');
	await expect(page.getByRole('heading', { level: 2, name: 'Favoris' })).toHaveCount(0);
	await expect(page.getByRole('heading', { level: 2, name: 'Services' })).toBeVisible();
	// En mobile, la carte et la barre du haut masquent ces détails.
	const card = page.getByRole('link', { name: /Radarr/ });
	await expect(card).toBeVisible();
	await expect(card).toContainText('mini-pc · port 7878');
	await expect(page.getByText('36 conteneurs', { exact: true })).toBeVisible();
	await expect(page.getByText('1 sur 1 en ligne')).toHaveCount(1);
});

test.describe('sans aucun service', () => {
	test.beforeEach(async ({ page }) => {
		await mockApi(page);
		const home = {
			summary: { online: 0, total: 0 },
			sessions: [],
			favorites: [],
			services: [],
			upcoming: [],
			hosts: []
		};
		await page.route('**/api/v1/home', (route) => route.fulfill({ json: home }));
		await page.route('**/api/v1/services', (route) => route.fulfill({ json: [] }));
	});

	test('l’accueil invite à connecter un hôte, sans rangée vide', async ({ page }) => {
		await page.goto('/');
		await expect(page.getByRole('heading', { level: 1, name: 'Bienvenue sur Hub' })).toBeVisible();
		await expect(page.getByRole('heading', { level: 2 })).toHaveCount(0);
		await expect(page.getByText('sur 0 en ligne')).toHaveCount(0);
		await page.getByRole('link', { name: 'Connecter un hôte' }).click();
		await expect(page).toHaveURL(/\/settings#hosts$/);
	});

	test('le catalogue explique qu’il est vide', async ({ page }) => {
		await page.goto('/services');
		await expect(page.getByText('Aucun service pour l’instant.')).toBeVisible();
		await expect(page.getByText('Aucun service ne correspond')).toHaveCount(0);
		await expect(page.getByRole('link', { name: 'Connecter un hôte' })).toBeVisible();
	});
});
