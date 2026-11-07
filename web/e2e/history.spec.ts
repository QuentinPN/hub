import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

type Json = Record<string, unknown>;

const pad = (n: number) => String(n).padStart(2, '0');
const isoDay = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

/** 90 jours comme le serveur les donne : les 10 premiers sans mesure, une panne avant-hier. */
function days() {
	const out: Json[] = [];
	for (let i = 89; i >= 0; i--) {
		const d = new Date();
		d.setDate(d.getDate() - i);
		const measured = i < 80;
		out.push({
			date: isoDay(d),
			upMinutes: measured && i !== 40 ? (i === 2 ? 1420 : 1440) : 0,
			downMinutes: i === 2 ? 20 : 0,
			unknownMinutes: i === 40 ? 1440 : 0,
			latencyMs: measured && i !== 40 ? 30 + (i % 7) : null,
			outages: i === 2 ? 1 : 0
		});
	}
	return out;
}

const EVENTS: Json[] = [
	{ id: 9, at: minutesAgo(5), kind: 'outage', action: 'up', minutes: 20 },
	{
		id: 8,
		at: minutesAgo(5),
		kind: 'alert',
		action: 'recovery',
		title: 'Emby est revenu',
		via: ['ntfy', 'smtp']
	},
	{ id: 7, at: minutesAgo(20), kind: 'container', action: 'start', container: 'emby' },
	{ id: 6, at: minutesAgo(24), kind: 'container', action: 'oom', container: 'emby' },
	{
		id: 5,
		at: minutesAgo(25),
		kind: 'outage',
		action: 'down',
		reason: 'conteneurs absents',
		minutes: 20
	},
	{
		id: 4,
		at: minutesAgo(26 * 60),
		kind: 'version',
		action: 'changed',
		container: 'emby',
		from: '4.8.11.0',
		to: '4.9.1.80',
		image: 'lscr.io/linuxserver/emby:4.9.1.80'
	},
	{ id: 3, at: minutesAgo(27 * 60), kind: 'edit', action: 'service_url_set', by: 'quentin' }
];
const COUNTS = { outage: 4, container: 21, version: 3, edit: 9, alert: 26 };

/** Le serveur simulé filtre et pagine comme le vrai ; il note chaque requête. */
async function mockHistory(page: Page, opts: { empty?: boolean; failures?: number } = {}) {
	const asked: URLSearchParams[] = [];
	let failures = opts.failures ?? 0;
	await page.route('**/api/v1/services/emby', (route) =>
		route.fulfill({ json: { ...(fixture('services-emby') as Json), discovered: true } })
	);
	await page.route('**/api/v1/services/emby/history**', (route) => {
		const q = new URL(route.request().url()).searchParams;
		asked.push(q);
		if (failures > 0) {
			failures--;
			return route.fulfill({ status: 500, json: { error: 'internal error' } });
		}
		const since = minutesAgo(30);
		if (opts.empty) {
			const blank = days().map((d) => ({
				...d,
				upMinutes: 0,
				downMinutes: 0,
				unknownMinutes: 0,
				latencyMs: null,
				outages: 0
			}));
			return route.fulfill({
				json: {
					since,
					retentionDays: 90,
					days: blank,
					counts: {},
					total: 0,
					offset: 0,
					limit: 25,
					events: []
				}
			});
		}
		const kind = q.get('kinds');
		const offset = Number(q.get('offset') ?? 0);
		const total = kind ? (COUNTS[kind as keyof typeof COUNTS] ?? 0) : 63;
		const events = kind === 'version' ? [] : EVENTS.filter((e) => !kind || e.kind === kind);
		return route.fulfill({
			json: {
				since,
				retentionDays: 90,
				days: days(),
				counts: COUNTS,
				total: kind === 'version' ? 0 : total,
				offset,
				limit: 25,
				events: offset ? EVENTS.slice(0, 2) : events
			}
		});
	});
	return asked;
}

test('l’onglet est cliquable et montre la disponibilité', async ({ page }) => {
	await mockApi(page);
	await mockHistory(page);
	await page.goto('/services/emby');
	await page
		.getByRole('navigation', { name: 'Sections du service' })
		.getByRole('link', { name: 'Historique' })
		.click();
	await expect(page).toHaveURL(/#historique$/);

	const avail = page.getByRole('region', { name: 'Disponibilité' });
	await expect(avail.getByText('99,98 %')).toBeVisible();
	await expect(avail.getByRole('definition').filter({ hasText: '20 min' })).toBeVisible();
	const bar = avail.getByRole('slider', { name: 'Disponibilité par jour' });
	await expect(bar.locator('.cell')).toHaveCount(90);
	await expect(bar.locator('.cell.down')).toHaveCount(0);
	await expect(bar.locator('.cell.warn')).toHaveCount(1);
	await expect(bar.locator('.cell.unknown')).toHaveCount(1);
	await expect(bar.locator('.cell.none')).toHaveCount(10);

	// Au clavier, chaque jour se dit ; deux jours en arrière, la panne.
	await bar.focus();
	await page.keyboard.press('ArrowLeft');
	await page.keyboard.press('ArrowLeft');
	await expect(bar).toHaveAttribute('aria-valuetext', /1 panne · 20 min en panne/);

	await avail.getByRole('button', { name: '30 jours' }).click();
	await expect(bar.locator('.cell')).toHaveCount(30);
	await expect(avail.getByRole('button', { name: '30 jours' })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(avail.getByRole('img', { name: /Latence moyenne par jour/ })).toBeVisible();
});

test('la chronologie se filtre et se pagine', async ({ page }) => {
	await mockApi(page);
	const asked = await mockHistory(page);
	await page.goto('/services/emby#historique');
	const events = page.getByRole('region', { name: 'Événements' });

	await expect(events.getByText('Aujourd’hui')).toBeVisible();
	await expect(events.getByText('Mémoire épuisée (OOM)')).toBeVisible();
	await expect(events.getByText('Après 20 min de panne')).toBeVisible();
	await expect(events.getByText('« Emby est revenu » · ntfy et e-mail')).toBeVisible();
	await expect(events.getByText('4.8.11.0 → 4.9.1.80', { exact: false })).toBeVisible();
	await expect(events.getByText('Par quentin')).toBeVisible();
	await expect(events.getByText('1 à 25 sur 63 événements')).toBeVisible();
	await expect(events.getByRole('button', { name: '‹ Plus récents' })).toBeDisabled();

	await events.getByRole('button', { name: 'Plus anciens ›' }).click();
	await expect(events.getByText('26 à 50 sur 63 événements')).toBeVisible();
	expect(asked.at(-1)?.get('offset')).toBe('25');

	await events.getByRole('button', { name: /^Pannes/ }).click();
	await expect(events.getByRole('button', { name: /^Pannes/ })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(events.getByText('En panne')).toBeVisible();
	await expect(events.getByText('Mémoire épuisée (OOM)')).toHaveCount(0);
	expect(asked.at(-1)?.get('kinds')).toBe('outage');
	expect(asked.at(-1)?.get('offset')).toBeNull();

	await events.getByRole('button', { name: /^Versions/ }).click();
	await expect(events.getByText('Aucun changement de version')).toBeVisible();
	await events.getByRole('button', { name: 'Tout afficher' }).click();
	await expect(events.getByRole('button', { name: /^Tout/ })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(events.getByRole('link', { name: 'Réglages › Général' })).toHaveAttribute(
		'href',
		'/settings#general'
	);
});

test('sans rien d’enregistré, l’historique dit quand il commence', async ({ page }) => {
	await mockApi(page);
	await mockHistory(page, { empty: true });
	await page.goto('/services/emby#historique');
	await expect(
		page.getByText(/^L’historique commence au \d+ \S+ \d{4} à \d\d:\d\d$/)
	).toBeVisible();
	await expect(page.getByRole('slider').locator('.cell.none')).toHaveCount(90);
});

test('une erreur se dit et se réessaie', async ({ page }) => {
	await mockApi(page);
	await mockHistory(page, { failures: 1 });
	await page.goto('/services/emby#historique');
	const alert = page.getByRole('alert').filter({ hasText: 'Impossible de charger l’historique' });
	await expect(alert).toBeVisible();
	await alert.getByRole('button', { name: 'Réessayer' }).click();
	await expect(page.getByRole('region', { name: 'Événements' })).toBeVisible();
	await expect(alert).toHaveCount(0);
});

for (const theme of ['dark', 'light'] as const)
	test(`aucune violation axe ni débordement · ${theme}`, async ({ page }) => {
		await page.addInitScript(
			(t) => localStorage.setItem('hub.appearance', JSON.stringify({ theme: t })),
			theme
		);
		await mockApi(page);
		await mockHistory(page);
		await page.goto('/services/emby#historique');
		await expect(page.getByText('Mémoire épuisée (OOM)')).toBeVisible();
		const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
		expect(violations.map((v) => `${v.id} : ${v.nodes.map((n) => n.target).join(', ')}`)).toEqual(
			[]
		);
		const overflow = await page.evaluate(
			() => document.documentElement.scrollWidth - document.documentElement.clientWidth
		);
		expect(overflow).toBeLessThanOrEqual(0);
	});
