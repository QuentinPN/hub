import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { mockApi } from './api.ts';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

test.beforeEach(async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings');
	await expect(page.getByRole('heading', { level: 1, name: 'Réglages' })).toBeVisible();
});

test('le menu des sections reste accessible et mène à la section', async ({ page }, testInfo) => {
	const nav = page.getByRole('navigation', { name: 'Sections des réglages' });
	await expect(nav).toBeVisible();
	// En mobile, seules les sections qui ont du contenu figurent dans la rangée de pilules.
	const links = nav.getByRole('link');
	await expect(links).toHaveText(['Hôtes et agents', 'Découverte', 'Sécurité et accès']);
	if (testInfo.project.name === 'mobile') {
		await expect(nav.getByText('Général')).toBeHidden();
	}
	await links.filter({ hasText: 'Sécurité et accès' }).click();
	await expect(page).toHaveURL(/#security$/);
	await expect(page.getByRole('heading', { level: 2, name: 'Sécurité et accès' })).toBeInViewport();
});

test('changer le mot de passe : erreur, puis succès et sessions rechargées', async ({ page }) => {
	let sessionLoads = 0;
	page.on('request', (req) => {
		if (req.url().endsWith('/api/v1/auth/sessions')) sessionLoads++;
	});
	const bodies: unknown[] = [];
	await page.route('**/api/v1/auth/password', (route) => {
		bodies.push(route.request().postDataJSON());
		return bodies.length === 1
			? route.fulfill({ status: 403, json: { error: 'wrong password' } })
			: route.fulfill({ status: 204 });
	});

	await page.getByRole('button', { name: 'Changer le mot de passe' }).click();
	const current = page.getByLabel('Mot de passe actuel');
	const next = page.getByLabel(/^Nouveau mot de passe/);
	const confirm = page.getByLabel('Confirmation du nouveau mot de passe');

	// Vérifié avant l'envoi : rien ne part.
	await current.fill('ancien mot de passe');
	await next.fill('trop court');
	await confirm.fill('trop court');
	await page.getByRole('button', { name: 'Changer le mot de passe' }).click();
	await expect(page.getByRole('alert')).toHaveText(/au moins 12 caractères/);
	expect(bodies).toHaveLength(0);

	const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
	expect(violations.map((v) => v.id)).toEqual([]);

	await current.fill('mauvais mot de passe');
	await next.fill('un tout nouveau mot de passe');
	await confirm.fill('un tout nouveau mot de passe');
	await page.getByRole('button', { name: 'Changer le mot de passe' }).click();
	await expect(page.getByRole('alert')).toHaveText('Mot de passe actuel incorrect.');

	const before = sessionLoads;
	await current.fill('bon mot de passe');
	await next.fill('un tout nouveau mot de passe');
	await confirm.fill('un tout nouveau mot de passe');
	await page.getByRole('button', { name: 'Changer le mot de passe' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Mot de passe changé' })).toBeVisible();
	await expect(current).toBeHidden();
	expect(bodies[1]).toEqual({ current: 'bon mot de passe', new: 'un tout nouveau mot de passe' });
	await expect.poll(() => sessionLoads).toBeGreaterThan(before);
});
