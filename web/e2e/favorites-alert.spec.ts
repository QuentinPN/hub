import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

type Favorite = { slug: string; name: string; status: string };
type Home = { favorites: Favorite[] } & Record<string, unknown>;

/** L'accueil avec chaque favori au statut donné ; les autres restent en ligne. */
async function homeWith(page: Page, statuses: () => Record<string, string>) {
	await mockApi(page);
	const home = fixture('home') as Home;
	await page.route('**/api/v1/home', (route) => {
		const set = statuses();
		const favorites = home.favorites.map((f) => ({ ...f, status: set[f.slug] ?? 'online' }));
		return route.fulfill({ json: { ...home, favorites } });
	});
}

test('un favori en panne se voit sans faire défiler la rangée', async ({ page }) => {
	await homeWith(page, () => ({ pihole: 'down', jellyseerr: 'degraded' }));
	await page.goto('/');
	const alert = page.getByRole('status').filter({ hasText: 'favoris ne sont pas en ligne' });
	await expect(alert).toBeInViewport();
	await expect(alert.getByRole('heading')).toHaveCount(0);
	// L'arrêt passe avant le dégradé, et chaque état est écrit, pas seulement coloré.
	const links = alert.getByRole('link');
	await expect(links).toHaveText([/Pi-hole\s*Arrêté/, /Jellyseerr\s*Dégradé/]);
	await links.first().click();
	await expect(page).toHaveURL('/services/pihole');
});

test('rien ne s’affiche quand tous les favoris sont en ligne', async ({ page }) => {
	await homeWith(page, () => ({}));
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Favoris' })).toBeVisible();
	await expect(page.getByText('pas en ligne')).toHaveCount(0);
});

test('l’avertissement suit la relecture régulière de l’accueil', async ({ page }) => {
	await page.clock.install();
	let statuses: Record<string, string> = {};
	await homeWith(page, () => statuses);
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Favoris' })).toBeVisible();
	await expect(page.getByText('pas en ligne')).toHaveCount(0);

	statuses = { sonarr: 'down' };
	await page.clock.runFor(30_000);
	const alert = page.getByRole('status').filter({ hasText: '1 favori n’est pas en ligne' });
	await expect(alert.getByRole('link', { name: /Sonarr/ })).toBeVisible();

	statuses = {};
	await page.clock.runFor(30_000);
	await expect(page.getByText('pas en ligne')).toHaveCount(0);
});
