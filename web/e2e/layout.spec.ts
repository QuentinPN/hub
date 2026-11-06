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
		'upcoming',
		'hosts',
		'downloads'
	]);
	await expect(page.getByRole('button', { name: 'Descendre Infrastructure' })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Descendre Téléchargements' })).toBeDisabled();
	await expect(page.getByText('Infrastructure déplacé en position 4.')).toBeAttached();

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
	await expect(page.getByRole('listitem').nth(3)).toContainText('Infrastructure');
	await page.getByRole('button', { name: 'Rétablir l’ordre' }).click();
	await expect(page.getByRole('listitem').nth(4)).toContainText('Infrastructure');
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
	await expect(page.getByRole('listitem').nth(4)).toContainText('Infrastructure');
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
