import { expect, test } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

test('quand tout va bien, la section le dit sans compteur', async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings');
	const panel = page.locator('#status');
	await expect(panel.getByRole('heading', { name: 'État de Hub' })).toBeVisible();
	await expect(panel.getByText('Tout fonctionne')).toBeVisible();
	await expect(panel.getByText('906bb56')).toBeVisible();
	await expect(panel.getByText('migration 10 sur 10')).toBeVisible();
	await expect(panel.getByRole('listitem')).toHaveCount(6);
	await expect(panel.getByRole('link', { name: 'Hôtes' })).toHaveCount(0);
	const nav = page.getByRole('navigation', { name: 'Sections des réglages' });
	await expect(nav.getByRole('link', { name: 'État de Hub' })).toHaveText('État de Hub');
});

test('un problème a son lien, et le menu en montre le nombre', async ({ page }) => {
	await mockApi(page);
	const status = fixture('status') as { checks: Record<string, unknown>[] };
	await page.route('**/api/v1/status', (route) =>
		route.fulfill({
			json: {
				...status,
				problems: 2,
				checks: status.checks.map((c) =>
					c.id === 'agents'
						? { ...c, level: 'warn', detail: '2 sur 3 connectés · QNAP injoignable' }
						: c.id === 'secrets'
							? { ...c, level: 'bad', detail: '1 secret illisible avec le fichier de clés' }
							: c
				)
			}
		})
	);
	await page.goto('/settings');
	const panel = page.locator('#status');
	await expect(panel.getByText('2 points à regarder')).toBeVisible();
	await expect(panel.getByText('À corriger :')).toBeAttached();
	await expect(panel.getByRole('link', { name: 'Hôtes' })).toHaveAttribute('href', '#hosts');
	const nav = page.getByRole('navigation', { name: 'Sections des réglages' });
	await expect(nav.getByRole('link', { name: /État de Hub/ })).toContainText('2');
});

test('si le serveur ne répond plus, la section le dit', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/status', (route) =>
		route.fulfill({ status: 502, body: 'Bad Gateway' })
	);
	await page.goto('/settings');
	await expect(page.locator('#status').getByRole('alert')).toContainText('ne répond pas');
});
