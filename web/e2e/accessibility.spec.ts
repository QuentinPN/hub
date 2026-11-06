import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { mockApi } from './api.ts';

// WCAG 2.2 niveau AA, plus les bonnes pratiques d'axe (landmarks, ordre des titres…).
const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

const pages = [
	{ path: '/', heading: 'Dune : Deuxième partie' },
	{ path: '/services', heading: 'Tous les services' },
	{ path: '/services/emby', heading: 'Emby' },
	{ path: '/services/prowlarr', heading: 'Prowlarr' },
	{ path: '/personnaliser', heading: 'Personnaliser l’accueil' },
	{ path: '/medias', heading: 'Médias' },
	{ path: '/telechargements', heading: 'Téléchargements' },
	{ path: '/infra', heading: 'Infra' },
	{ path: '/settings', heading: 'Réglages' }
];

for (const { path, heading } of pages) {
	test.describe(path, () => {
		test.beforeEach(async ({ page }) => {
			await mockApi(page);
			await page.goto(path);
			await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
		});

		test('aucune violation axe', async ({ page }) => {
			const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
			expect(violations.map((v) => `${v.id} : ${v.nodes.map((n) => n.target).join(', ')}`)).toEqual(
				[]
			);
		});

		test('pas de défilement horizontal de la page', async ({ page }) => {
			const { scroll, client } = await page.evaluate(() => ({
				scroll: document.documentElement.scrollWidth,
				client: document.documentElement.clientWidth
			}));
			expect(scroll).toBeLessThanOrEqual(client);
		});

		test('chaque élément atteint au clavier a un focus visible', async ({ page }) => {
			const seen = new Set<string>();
			for (let i = 0; i < 40; i++) {
				await page.keyboard.press('Tab');
				const focus = await page.evaluate(() => {
					const el = document.activeElement as HTMLElement | null;
					if (!el || el === document.body) return null;
					// Un champ sans outline propre peut porter l'anneau sur son libellé (recherche).
					const ring = getComputedStyle(el).outlineStyle === 'none' ? el.closest('label') : el;
					const style = getComputedStyle(ring ?? el);
					return {
						key: el.outerHTML.slice(0, 120),
						visible: style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) >= 2
					};
				});
				if (!focus || seen.has(focus.key)) break;
				seen.add(focus.key);
				expect(focus.visible, focus.key).toBe(true);
			}
			expect(seen.size).toBeGreaterThan(3);
		});
	});
}

test('la page de connexion n’a aucune violation axe', async ({ page }) => {
	await mockApi(page, { authenticated: false });
	await page.goto('/login');
	await expect(page.getByRole('heading', { level: 1, name: 'Connexion' })).toBeVisible();
	const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
	expect(violations.map((v) => v.id)).toEqual([]);
});

test.describe('mouvement réduit', () => {
	test.use({ reducedMotion: 'reduce' });

	test('le focus d’une carte ne l’agrandit pas et ne l’anime pas', async ({ page }) => {
		await mockApi(page);
		await page.goto('/services');
		const card = page.getByRole('link', { name: /Emby/ });
		await card.focus();
		const style = await card.evaluate((el) => {
			const s = getComputedStyle(el);
			return { transform: s.transform, duration: s.transitionDuration };
		});
		expect(['none', 'matrix(1, 0, 0, 1, 0, 0)']).toContain(style.transform);
		expect(style.duration.split(',').every((d) => parseFloat(d) === 0)).toBe(true);
	});
});

test('sans préférence, le focus d’une carte l’agrandit', async ({ page }) => {
	await mockApi(page);
	await page.goto('/services');
	const card = page.locator('.app', { hasText: 'Emby' });
	await card.getByRole('link', { name: /Emby/ }).focus();
	await expect
		.poll(() => card.evaluate((el) => getComputedStyle(el).transform))
		.toBe('matrix(1.06, 0, 0, 1.06, 0, 0)');
});
