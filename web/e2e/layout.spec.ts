import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

type Row = { id: string; visible: boolean; excluded: string[] };
type Layout = { rows: Row[]; choices: Record<string, unknown> };

/** Répond comme le serveur : la disposition reçue devient la disposition lue. */
async function trackLayout(page: Page) {
	await mockApi(page);
	const sent: Row[][] = [];
	let current = fixture('home-layout') as Layout;
	const initial = current;
	await page.route('**/api/v1/home-layout', async (route) => {
		const method = route.request().method();
		if (method === 'PUT') {
			const body = route.request().postDataJSON() as { rows: Row[] };
			sent.push(body.rows);
			current = { ...current, rows: body.rows };
		}
		if (method === 'DELETE') current = initial;
		await route.fulfill({ json: current });
	});
	return sent;
}

test('Personnaliser s’ouvre depuis l’accueil seulement', async ({ page }) => {
	await trackLayout(page);
	await page.goto('/');
	await page.getByRole('link', { name: 'Personnaliser' }).click();
	await expect(page).toHaveURL('/personnaliser');
	await expect(
		page.getByRole('heading', { level: 1, name: 'Personnaliser l’accueil' })
	).toBeVisible();
	await expect(page.getByRole('link', { name: 'Personnaliser', exact: true })).toHaveCount(0);
	await page.getByRole('link', { name: 'Terminé' }).click();
	await expect(page).toHaveURL('/');
});

test('monter, descendre et masquer une rangée s’enregistre aussitôt', async ({ page }) => {
	const sent = await trackLayout(page);
	await page.goto('/personnaliser');

	await page.getByRole('button', { name: 'Monter Infrastructure' }).click();
	await expect.poll(() => sent.length).toBe(1);
	expect(sent[0]?.map((r) => r.id)).toEqual([
		'playing',
		'favorites',
		'latest',
		'upcoming',
		'hosts',
		'downloads'
	]);
	await expect(page.getByRole('button', { name: 'Descendre Infrastructure' })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Descendre Téléchargements' })).toBeDisabled();
	await expect(page.getByText('Infrastructure déplacé en position 5.')).toBeAttached();

	await page.getByRole('checkbox', { name: 'Visible : Favoris' }).uncheck();
	await expect.poll(() => sent.length).toBe(2);
	expect(sent[1]?.find((r) => r.id === 'favorites')?.visible).toBe(false);
});

test('le bandeau reste en haut, mais peut se masquer', async ({ page }) => {
	await trackLayout(page);
	await page.goto('/personnaliser');
	await expect(page.getByText('Toujours en haut')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Monter En lecture' })).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Monter Favoris' })).toBeDisabled();
	await expect(page.getByRole('checkbox', { name: 'Visible : En lecture' })).toBeChecked();
});

test('retirer une source la laisse dans la liste, prête à revenir', async ({ page }) => {
	const sent = await trackLayout(page);
	await page.goto('/personnaliser');
	const chip = page.getByRole('button', { name: 'Prowlarr' });
	await expect(chip).toHaveAttribute('aria-pressed', 'true');
	await chip.click();
	await expect(chip).toHaveAttribute('aria-pressed', 'false');
	expect(sent[0]?.find((r) => r.id === 'downloads')?.excluded).toEqual(['prowlarr']);
	await chip.click();
	await expect(chip).toHaveAttribute('aria-pressed', 'true');
});

test('rétablir l’ordre revient au défaut', async ({ page }) => {
	await trackLayout(page);
	await page.goto('/personnaliser');
	await page.getByRole('button', { name: 'Monter Infrastructure' }).click();
	await expect(page.getByRole('listitem').nth(4)).toContainText('Infrastructure');
	await page.getByRole('button', { name: 'Rétablir l’ordre' }).click();
	await expect(page.getByRole('listitem').nth(5)).toContainText('Infrastructure');
});

test('un enregistrement refusé le dit et revient en arrière', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/home-layout', (route) =>
		route.request().method() === 'PUT'
			? route.fulfill({ status: 500, json: { error: 'internal error' } })
			: route.fulfill({ json: fixture('home-layout') })
	);
	await page.goto('/personnaliser');
	await page.getByRole('button', { name: 'Monter Infrastructure' }).click();
	await expect(page.getByRole('alert')).toContainText('Non enregistré');
	await expect(page.getByRole('listitem').nth(5)).toContainText('Infrastructure');
});

test('sans lecture, Emby reste sur l’accueil et dans Personnaliser', async ({ page }) => {
	await trackLayout(page);
	const home = fixture('home') as Record<string, unknown>;
	// Une série est datée par son dernier épisode ajouté, pas par le premier.
	const episode = {
		title: 'Une série',
		detail: 'S02E06',
		kind: 'episode',
		addedAt: new Date(Date.now() - 2 * 24 * 3600 * 1000).toISOString()
	};
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({
			json: {
				...home,
				sessions: [],
				sessionsSlug: 'emby',
				lastAdded: episode,
				library: { movies: 1240, series: 312, episodes: 8420 },
				latest: [
					episode,
					{
						title: 'Un film',
						detail: 'Film',
						kind: 'movie',
						addedAt: new Date(Date.now() - 3 * 24 * 3600 * 1000).toISOString()
					}
				],
				latestSlug: 'emby',
				rows: ['playing', 'favorites', 'latest', 'upcoming']
			}
		})
	);
	await page.goto('/');
	await expect(page.getByRole('heading', { level: 1, name: 'Rien en lecture' })).toBeVisible();
	await expect(page.getByText('Emby · aucune session')).toBeVisible();
	await expect(page.getByText('Dernier ajout : Une série, il y a 2 j')).toBeVisible();
	await expect(page.getByText(/1\s240 films · 312 séries/)).toBeVisible();
	await expect(page.getByRole('link', { name: 'Ouvrir la fiche Emby' })).toHaveAttribute(
		'href',
		'/services/emby'
	);
	await expect(page.getByRole('heading', { level: 2 })).toHaveText([
		'Favoris',
		'Ajoutés récemment · Emby',
		'À venir · Sonarr et Radarr'
	]);
	await expect(page.getByRole('region', { name: 'Ajoutés récemment · Emby' })).toContainText(
		'Une série'
	);

	await page.getByRole('link', { name: 'Personnaliser' }).click();
	const row = page.getByRole('listitem').filter({ hasText: 'Ajoutés récemment' });
	await expect(row.getByRole('button', { name: 'Emby' })).toHaveAttribute('aria-pressed', 'true');
	await expect(row.getByRole('checkbox', { name: 'Visible : Ajoutés récemment' })).toBeChecked();
});

test('ce qui joue s’affiche avec son fond et son affiche, servis par Hub', async ({ page }) => {
	await trackLayout(page);
	const home = fixture('home') as { sessions: Record<string, unknown>[] };
	const png = Buffer.from(
		'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
		'base64'
	);
	const asked: string[] = [];
	await page.route('**/api/v1/posters/emby/**', (route) => {
		asked.push(new URL(route.request().url()).pathname);
		return route.fulfill({ contentType: 'image/png', body: png });
	});
	let playing = true;
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({
			json: {
				...home,
				sessions: playing
					? [{ ...home.sessions[0], poster: 'serie', backdrop: 'serie.backdrop' }]
					: [],
				sessionsSlug: 'emby',
				rows: ['playing', 'favorites']
			}
		})
	);
	await page.goto('/');
	await expect(page.getByText('En lecture sur Emby')).toBeVisible();
	const backdrop = page.locator('.hero .backdrop');
	await expect(backdrop).toHaveCSS('opacity', '1');
	await expect(backdrop.locator('img')).toHaveAttribute(
		'src',
		'/api/v1/posters/emby/serie.backdrop'
	);
	const cover = page.locator('.hero img.cover');
	if ((page.viewportSize()?.width ?? 0) > 640) await expect(cover).toBeVisible();
	await expect(cover).toHaveAttribute('src', '/api/v1/posters/emby/serie');
	expect(asked).toContain('/api/v1/posters/emby/serie.backdrop');

	// La lecture s'arrête : le fond s'en va avec elle.
	playing = false;
	await page.reload();
	await expect(page.getByRole('heading', { level: 1, name: 'Rien en lecture' })).toBeVisible();
	await expect(page.locator('.hero .backdrop')).toHaveCount(0);
});

test('sans image chez Emby, le bandeau garde ses couleurs', async ({ page }) => {
	await trackLayout(page);
	const home = fixture('home') as { sessions: Record<string, unknown>[] };
	await page.route('**/api/v1/posters/emby/**', (route) =>
		route.fulfill({ status: 404, json: { error: 'not found' } })
	);
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({
			json: {
				...home,
				sessions: [{ ...home.sessions[0], poster: 'film', backdrop: 'film.backdrop' }],
				sessionsSlug: 'emby',
				rows: ['playing']
			}
		})
	);
	await page.goto('/');
	await expect(page.getByText('En lecture sur Emby')).toBeVisible();
	await expect(page.locator('.hero .backdrop')).toHaveCount(0);
	await expect(page.locator('.hero img.cover')).toHaveCount(0);
});

test('l’accueil suit l’ordre et masque le bandeau', async ({ page }) => {
	await mockApi(page);
	const home = fixture('home') as Record<string, unknown>;
	await page.route('**/api/v1/home', (route) =>
		route.fulfill({ json: { ...home, rows: ['hosts', 'favorites'] } })
	);
	await page.goto('/');
	const titles = page.getByRole('heading', { level: 2 });
	await expect(titles).toHaveText(['Infrastructure', 'Favoris']);
	await expect(page.getByRole('heading', { level: 1, name: 'Accueil' })).toBeAttached();
	await expect(page.getByRole('link', { name: 'Ouvrir la fiche Emby' })).toHaveCount(0);
});

test('les flèches font défiler une rangée trop longue', async ({ page, hasTouch }) => {
	test.skip(hasTouch, 'au doigt, la rangée se glisse ; les flèches sont masquées');
	await mockApi(page);
	const home = fixture('home') as { favorites: { slug: string; name: string }[] };
	const favorites = Array.from({ length: 20 }, (_, i) => ({
		...home.favorites[i % home.favorites.length],
		slug: `app-${i}`,
		name: `App ${i}`
	}));
	await page.route('**/api/v1/home', (route) => route.fulfill({ json: { ...home, favorites } }));
	await page.goto('/');
	const shelf = page.getByRole('region', { name: 'Favoris' });
	const previous = shelf.getByRole('button', { name: 'Précédents' });
	const next = shelf.getByRole('button', { name: 'Suivants' });
	await expect(previous).toBeDisabled();
	await next.click();
	await expect(previous).toBeEnabled();
	await expect(shelf.getByRole('link', { name: /App 19/ })).not.toBeInViewport();
	for (let i = 0; i < 6 && (await next.isEnabled()); i++) await next.click();
	await expect(next).toBeDisabled();
	await expect(shelf.getByRole('link', { name: /App 19/ })).toBeInViewport();
});
