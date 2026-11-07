import { expect, test, type Page } from '@playwright/test';
import { mockApi } from './api.ts';

const html = (page: Page, name: string) =>
	page.evaluate((n) => document.documentElement.getAttribute(n), name);

// Playwright n'émule pas prefers-reduced-transparency ; sans cela, le test suivrait le réglage
// du poste qui le lance (Windows sans effets de transparence le demande).
async function deviceTransparency(page: Page, value: 'reduce' | 'no-preference') {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('Emulation.setEmulatedMedia', {
		features: [{ name: 'prefers-reduced-transparency', value }]
	});
}

test.beforeEach(async ({ page }) => {
	await deviceTransparency(page, 'no-preference');
	await mockApi(page);
	await page.goto('/settings#appearance');
	await expect(page.getByRole('heading', { level: 2, name: 'Apparence' })).toBeVisible();
});

test('l’entrée Apparence du menu mène à la section', async ({ page }) => {
	await page.goto('/settings');
	await page
		.getByRole('navigation', { name: 'Sections des réglages' })
		.getByRole('link', { name: 'Apparence' })
		.click();
	await expect(page).toHaveURL(/#appearance$/);
	await expect(page.getByRole('heading', { level: 2, name: 'Apparence' })).toBeInViewport();
});

test('densité compacte : plus de tuiles par rangée, gardée après rechargement', async ({
	page
}) => {
	const density = page.getByRole('radiogroup', { name: 'Densité' });
	await expect(density.getByRole('radio', { name: 'Confortable' })).toBeChecked();

	await page.goto('/services');
	const card = page.locator('.app').first();
	const wide = await card.evaluate((el) => el.getBoundingClientRect().width);

	await page.goto('/settings#appearance');
	await density.getByRole('radio', { name: 'Compacte' }).check();
	expect(await html(page, 'data-density')).toBe('compact');

	await page.reload();
	await expect(density.getByRole('radio', { name: 'Compacte' })).toBeChecked();
	await page.goto('/services');
	expect(await html(page, 'data-density')).toBe('compact');
	expect(await card.evaluate((el) => el.getBoundingClientRect().width)).toBeLessThan(wide);

	await page.goto('/settings#appearance');
	await density.getByRole('radio', { name: 'Confortable' }).check();
	await page.reload();
	expect(await html(page, 'data-density')).toBeNull();
});

test('moins de transparence : plus de flou, gardé après rechargement', async ({ page }) => {
	const box = page.getByRole('checkbox', { name: /Moins de transparence/ });
	await expect(box).not.toBeChecked();
	const blur = () =>
		page
			.getByRole('navigation', { name: 'Sections' })
			.first()
			.evaluate((el) => getComputedStyle(el).backdropFilter);

	await box.check();
	expect(await html(page, 'data-transparency')).toBe('reduce');
	await page.reload();
	await expect(box).toBeChecked();
	expect(await html(page, 'data-transparency')).toBe('reduce');
	if (test.info().project.name === 'desktop') expect(await blur()).toBe('none');

	await box.uncheck();
	await page.reload();
	await expect(box).not.toBeChecked();
	expect(await html(page, 'data-transparency')).toBeNull();
});

test('réduire les animations : le focus n’agrandit plus les cartes, même après rechargement', async ({
	page
}) => {
	const box = page.getByRole('checkbox', { name: /Réduire les animations/ });
	await box.check();
	await page.reload();
	await expect(box).toBeChecked();

	await page.goto('/services');
	expect(await html(page, 'data-motion')).toBe('reduce');
	const card = page.locator('.app', { hasText: 'Emby' });
	await card.getByRole('link', { name: /Emby/ }).focus();
	const style = await card.evaluate((el) => {
		const s = getComputedStyle(el);
		return { transform: s.transform, duration: s.transitionDuration };
	});
	expect(['none', 'matrix(1, 0, 0, 1, 0, 0)']).toContain(style.transform);
	expect(style.duration.split(',').every((d) => parseFloat(d) === 0)).toBe(true);
});

test('un appareil qui demande moins de transparence coche la case d’office', async ({ page }) => {
	await deviceTransparency(page, 'reduce');
	await page.reload();
	const box = page.getByRole('checkbox', { name: /Moins de transparence/ });
	await expect(box).toBeChecked();
	await expect(box).toBeDisabled();
	if (test.info().project.name === 'desktop') {
		const blur = await page
			.getByRole('navigation', { name: 'Sections' })
			.first()
			.evaluate((el) => getComputedStyle(el).backdropFilter);
		expect(blur).toBe('none');
	}
});

test.describe('appareil qui demande déjà moins de mouvement', () => {
	test.use({ reducedMotion: 'reduce' });

	test('la case est cochée d’office et ne se décoche pas', async ({ page }) => {
		const box = page.getByRole('checkbox', { name: /Réduire les animations/ });
		await expect(box).toBeChecked();
		await expect(box).toBeDisabled();
	});
});

test('thème : clair, comme l’appareil, sombre, gardés après rechargement', async ({ page }) => {
	const theme = page.getByRole('group', { name: 'Thème' });
	const bg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
	const meta = () =>
		page.evaluate(() =>
			document.querySelector('meta[name="theme-color"]')?.getAttribute('content')
		);
	await expect(theme.getByRole('radio', { name: 'Sombre' })).toBeChecked();
	expect(await bg()).toBe('rgb(7, 8, 10)');

	await theme.getByRole('radio', { name: 'Clair' }).check();
	expect(await html(page, 'data-theme')).toBe('light');
	expect(await bg()).toBe('rgb(242, 242, 245)');
	await page.reload();
	await expect(theme.getByRole('radio', { name: 'Clair' })).toBeChecked();
	expect(await html(page, 'data-theme')).toBe('light');
	expect(await meta()).toBe('#F2F2F5');

	// « Comme l'appareil » suit l'appareil, y compris quand il change en cours de route.
	await page.emulateMedia({ colorScheme: 'dark' });
	await theme.getByRole('radio', { name: 'Comme l’appareil' }).check();
	expect(await html(page, 'data-theme')).toBe('dark');
	await page.emulateMedia({ colorScheme: 'light' });
	await expect.poll(() => html(page, 'data-theme')).toBe('light');
	await page.reload();
	await expect(theme.getByRole('radio', { name: 'Comme l’appareil' })).toBeChecked();
	expect(await html(page, 'data-theme')).toBe('light');

	await theme.getByRole('radio', { name: 'Sombre' }).check();
	await page.reload();
	expect(await html(page, 'data-theme')).toBe('dark');
	expect(await bg()).toBe('rgb(7, 8, 10)');
});

test('le thème est posé avant le premier rendu, sans passer par le sombre', async ({ page }) => {
	await page.evaluate(() => localStorage.setItem('hub.appearance', '{"theme":"light"}'));
	// Lu au moment où le document est analysé, avant tout script de l'application.
	await page.addInitScript(() => {
		document.addEventListener('DOMContentLoaded', () => {
			(window as unknown as { early: string | null }).early =
				document.documentElement.getAttribute('data-theme');
		});
	});
	await page.reload();
	expect(await page.evaluate(() => (window as unknown as { early: string | null }).early)).toBe(
		'light'
	);
});

test('un stockage illisible retombe sur les défauts', async ({ page }) => {
	await page.evaluate(() => localStorage.setItem('hub.appearance', '{abîmé'));
	await page.reload();
	await expect(page.getByRole('radio', { name: 'Confortable' })).toBeChecked();
	expect(await html(page, 'data-theme')).toBe('dark');
});
