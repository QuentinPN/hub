import { expect, test } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

type Backup = Record<string, unknown>;

test('la section dit la dernière, la prochaine et l’historique', async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings#backup');
	const panel = page.locator('#backup');
	await expect(panel.getByRole('heading', { name: 'Sauvegarde', level: 2 })).toBeVisible();
	await expect(panel.getByText('réussie · 2,4 Mo · 41 s')).toBeVisible();
	await expect(panel.getByText('4 sauvegardes')).toBeVisible();
	await expect(panel.getByText('/sauvegardes', { exact: true })).toBeVisible();
	await expect(panel.getByText('Chiffrées avec ta phrase de passe')).toBeVisible();
	await expect(panel.getByRole('listitem')).toHaveCount(4);
	await expect(panel.getByText(/Échec · Dossier \/sauvegardes absent/)).toBeVisible();
	await expect(panel.getByRole('switch', { name: /Sauvegarde automatique/ })).toBeChecked();
});

test('couper la sauvegarde automatique l’enregistre et le dit', async ({ page }) => {
	await mockApi(page);
	const bodies: unknown[] = [];
	await page.route('**/api/v1/backup/schedule', (route) => {
		const body = route.request().postDataJSON() as Backup;
		bodies.push(body);
		return route.fulfill({ json: { ...(fixture('backup') as Backup), ...body, next: null } });
	});
	await page.goto('/settings#backup');
	const panel = page.locator('#backup');
	await panel.getByRole('switch', { name: /Sauvegarde automatique/ }).click();
	await expect(panel.getByRole('switch', { name: /Sauvegarde automatique/ })).not.toBeChecked();
	expect(bodies).toEqual([{ enabled: false, frequency: 'daily', retention: '7d4w' }]);
	await expect(panel.getByText('automatique désactivée')).toBeVisible();
	await expect(
		panel.getByText(/ne sauvegarde plus que sur « Sauvegarder maintenant »/)
	).toBeVisible();
});

test('sans phrase de passe, il faut la choisir avant tout', async ({ page }) => {
	await mockApi(page);
	const empty = {
		...(fixture('backup') as Backup),
		configured: false,
		keySetAt: null,
		next: null,
		kept: 0,
		keptBytes: 0,
		runs: []
	};
	await page.route('**/api/v1/backup', (route) => route.fulfill({ json: empty }));
	const sent: unknown[] = [];
	await page.route('**/api/v1/backup/passphrase', (route) => {
		sent.push(route.request().postDataJSON());
		return route.fulfill({ json: { ...(fixture('backup') as Backup), runs: [] } });
	});
	await page.goto('/settings#backup');
	const panel = page.locator('#backup');
	await expect(panel.getByRole('button', { name: 'Sauvegarder maintenant' })).toBeDisabled();
	await panel.getByLabel('Phrase de passe').fill('courte');
	await expect(panel.getByText('12 caractères au moins.')).toBeVisible();
	await panel.getByLabel('Phrase de passe').fill('une phrase assez longue');
	await panel.getByLabel('Encore une fois').fill('une phrase assez longue');
	await panel.getByRole('button', { name: 'Enregistrer la phrase' }).click();
	await expect(panel.getByText(/Hub ne la garde pas/)).toBeVisible();
	expect(sent).toEqual([{ passphrase: 'une phrase assez longue' }]);
	await expect(panel.getByRole('button', { name: 'Sauvegarder maintenant' })).toBeEnabled();
});

test('un dossier absent bloque la sauvegarde et dit quoi faire', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/backup', (route) =>
		route.fulfill({
			json: {
				...(fixture('backup') as Backup),
				dirError: 'Dossier /sauvegardes absent : à monter dans le compose'
			}
		})
	);
	await page.goto('/settings#backup');
	const panel = page.locator('#backup');
	await expect(panel.getByRole('alert')).toContainText('à monter dans le compose');
	await expect(panel.getByRole('button', { name: 'Sauvegarder maintenant' })).toBeDisabled();
});

test('désactivées sur le serveur, la section le dit et ne propose rien', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/backup', (route) =>
		route.fulfill({ json: { ...(fixture('backup') as Backup), off: true, runs: [] } })
	);
	await page.goto('/settings#backup');
	const panel = page.locator('#backup');
	await expect(panel.getByText('Sauvegardes désactivées sur ce serveur.')).toBeVisible();
	await expect(panel.getByRole('button')).toHaveCount(0);
	await expect(panel.getByRole('switch')).toHaveCount(0);
	await expect(panel.getByLabel('Phrase de passe')).toHaveCount(0);
});
