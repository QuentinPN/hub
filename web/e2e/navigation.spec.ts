import { expect, test } from '@playwright/test';
import { mockApi } from './api.ts';

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

test('le titre de l’onglet suit la page', async ({ page }) => {
	await page.goto('/services/emby');
	await expect(page).toHaveTitle('Emby · Hub');
	await page.goto('/services');
	await expect(page).toHaveTitle('Tous les services · Hub');
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
