import { expect, test } from '@playwright/test';
import { mockApi } from './api.ts';

test.beforeEach(async ({ page }) => {
	await mockApi(page);
});

test('le logo est servi en favicon, avec ses replis', async ({ page, request }) => {
	await page.goto('/');
	const links = [
		{ selector: 'link[rel="icon"][type="image/svg+xml"]', type: 'image/svg+xml' },
		{ selector: 'link[rel="icon"][sizes="32x32"]', type: 'image/' },
		{ selector: 'link[rel="apple-touch-icon"]', type: 'image/png' }
	];
	for (const { selector, type } of links) {
		const href = await page.locator(selector).getAttribute('href');
		expect(href, selector).toBeTruthy();
		const res = await request.get(new URL(href ?? '', page.url()).href);
		expect(res.status(), selector).toBe(200);
		expect(res.headers()['content-type'], selector).toContain(type);
	}
	await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute('content', '#07080A');
});
