import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { mockApi } from './api.ts';

const summary = {
	slug: 'tzmasters',
	name: 'TZ Masters',
	monogram: 'T',
	icon: null,
	color: '#a23b4f',
	category: 'Mes applications',
	hostId: 'mini-pc',
	status: 'config_required',
	latencyMs: null,
	note: 'port 8096',
	favorite: false,
	url: null
};

const detail = {
	...summary,
	version: '1.6.0',
	tint: '#1f2a36',
	stats: [],
	upcoming: [],
	identification: [{ label: 'Reconnu par', value: 'projet Compose tzmasters-prod' }],
	access: [{ label: 'tzmasters', value: 'en cours · principal' }],
	connection: [{ label: 'Widget', value: 'non configuré' }],
	discovered: true,
	ports: [8096, 8920],
	project: { key: 'tzmasters-prod', containers: 5 },
	links: [],
	linkable: [{ key: 'tzmasters-db-prod', containers: 1 }]
};

const online = {
	...detail,
	status: 'online',
	note: 'en ligne',
	url: 'http://192.0.2.14:8096',
	urlCheck: {
		result: 'ok',
		status: 200,
		latencyMs: 42,
		checkedAt: new Date().toISOString(),
		certPinned: false
	}
};

/** Fiche découverte, avec une réponse au choix pour la vérification d'adresse. */
async function openFiche(page: Page, check: Record<string, unknown>) {
	await mockApi(page);
	let current: unknown = detail;
	const sent: unknown[] = [];
	await page.route('**/api/v1/services/tzmasters', async (route) => {
		if (route.request().method() === 'PATCH') {
			sent.push(route.request().postDataJSON());
			current = { ...detail, name: 'TZ', links: [{ key: 'tzmasters-db-prod', containers: 1 }] };
		}
		await route.fulfill({ json: current });
	});
	await page.route('**/api/v1/services/tzmasters/url', async (route) => {
		const body = route.request().postDataJSON() as Record<string, unknown>;
		sent.push(body);
		const saved = check.saved === true || body.force === true || Boolean(body.acceptCert);
		if (saved) current = online;
		await route.fulfill({ json: { ...check, saved } });
	});
	await page.route('**/api/v1/services/tzmasters/favorite', async (route) => {
		current = { ...detail, favorite: true };
		await route.fulfill({ json: current });
	});
	await page.goto('/services/tzmasters');
	return sent;
}

const ok = { saved: true, result: 'ok', status: 200, latencyMs: 42 };

test('une adresse qui répond est enregistrée et ouvre le service', async ({ page }) => {
	const sent = await openFiche(page, ok);
	await expect(page.getByRole('link', { name: /Ouvrir TZ Masters/ })).toHaveCount(0);

	await page.getByLabel('Adresse', { exact: true }).fill('http://192.0.2.14');
	await page.getByRole('button', { name: 'Utiliser le port 8096' }).click();
	await expect(page.getByLabel('Adresse', { exact: true })).toHaveValue('http://192.0.2.14:8096');
	await page.getByRole('button', { name: 'Vérifier et enregistrer' }).click();

	expect(sent).toEqual([{ url: 'http://192.0.2.14:8096' }]);
	const open = page.getByRole('link', { name: /Ouvrir TZ Masters/ });
	await expect(open).toHaveAttribute('href', 'http://192.0.2.14:8096');
	await expect(open).toHaveAttribute('target', '_blank');
	await expect(open).toHaveAttribute('rel', 'noopener noreferrer');
	await expect(page.getByText('200 en 42 ms')).toBeVisible();
});

test('un port seul prépare le champ pour taper l’hôte', async ({ page }) => {
	await openFiche(page, ok);
	await page.getByRole('button', { name: 'Utiliser le port 8920' }).click();
	const field = page.getByLabel('Adresse', { exact: true });
	await expect(field).toHaveValue('http://:8920');
	await expect(field).toBeFocused();
});

test('une adresse injoignable ne s’enregistre que sur demande', async ({ page }) => {
	const sent = await openFiche(page, {
		saved: false,
		result: 'unreachable',
		status: 0,
		latencyMs: 0
	});
	await page.getByLabel('Adresse', { exact: true }).fill('http://10.0.0.9');
	await page.getByRole('button', { name: 'Vérifier et enregistrer' }).click();
	await expect(page.getByText('Injoignable depuis le serveur')).toBeVisible();
	await expect(page.getByRole('link', { name: /Ouvrir TZ Masters/ })).toHaveCount(0);

	await page.getByRole('button', { name: 'Enregistrer quand même' }).click();
	await expect(page.getByRole('link', { name: /Ouvrir TZ Masters/ })).toBeVisible();
	expect(sent[1]).toEqual({ url: 'http://10.0.0.9', force: true });
});

test('un certificat inconnu s’accepte explicitement, par son empreinte', async ({ page }) => {
	const certSha256 = 'ab'.repeat(32);
	const sent = await openFiche(page, {
		saved: false,
		result: 'cert',
		status: 0,
		latencyMs: 0,
		certSha256,
		certName: 'dockhand.lan'
	});
	await page.getByLabel('Adresse', { exact: true }).fill('https://dockhand.lan');
	await page.getByRole('button', { name: 'Vérifier et enregistrer' }).click();
	await expect(page.getByText('Certificat pour dockhand.lan', { exact: false })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Enregistrer quand même' })).toHaveCount(0);

	await page.getByRole('button', { name: 'Accepter ce certificat' }).click();
	await expect(page.getByRole('link', { name: /Ouvrir TZ Masters/ })).toBeVisible();
	expect(sent[1]).toEqual({ url: 'https://dockhand.lan', acceptCert: certSha256 });
});

test('le favori se pose depuis la fiche', async ({ page }) => {
	await openFiche(page, ok);
	const star = page.getByRole('button', { name: 'Favori' });
	await expect(star).toHaveAttribute('aria-pressed', 'false');
	await star.click();
	await expect(star).toHaveAttribute('aria-pressed', 'true');
});

test('le panneau Modifier corrige le service et relie un projet', async ({ page }) => {
	const sent = await openFiche(page, ok);
	await page.getByRole('button', { name: 'Modifier' }).click();
	const dialog = page.getByRole('dialog', { name: 'Modifier TZ Masters' });
	await expect(dialog).toBeVisible();
	const axe = await new AxeBuilder({ page }).include('dialog').analyze();
	expect(axe.violations).toEqual([]);

	await dialog.getByLabel('Nom').fill('TZ');
	await dialog.getByLabel(/Relier un autre projet/).selectOption('tzmasters-db-prod');
	await expect(dialog.getByRole('button', { name: 'Retirer tzmasters-db-prod' })).toBeVisible();
	await expect(dialog.getByLabel(/Relier un autre projet/)).toHaveCount(0);
	await dialog.getByRole('button', { name: 'Enregistrer' }).click();

	await expect(dialog).toHaveCount(0);
	await expect(page.getByRole('heading', { level: 1, name: 'TZ' })).toBeVisible();
	expect(sent).toEqual([
		{
			name: 'TZ',
			category: 'Mes applications',
			icon: '',
			hidden: false,
			links: ['tzmasters-db-prod']
		}
	]);
});

test('reprendre le détecté demande une confirmation', async ({ page }) => {
	await openFiche(page, ok);
	let resets = 0;
	await page.route('**/api/v1/services/tzmasters/reset', async (route) => {
		resets++;
		await route.fulfill({ json: detail });
	});
	await page.getByRole('button', { name: 'Modifier' }).click();
	await page.getByRole('button', { name: 'Reprendre le détecté' }).click();
	expect(resets).toBe(0);
	await page.getByRole('button', { name: 'Confirmer : effacer mes corrections' }).click();
	await expect(page.getByRole('dialog')).toHaveCount(0);
	expect(resets).toBe(1);
});

test('le catalogue pose un favori et réaffiche un masqué', async ({ page }) => {
	await mockApi(page);
	const list = [summary, { ...summary, slug: 'emby', name: 'Emby', url: 'http://x.fr:8096' }];
	await page.route('**/api/v1/services', (route) => route.fulfill({ json: list }));
	await page.route('**/api/v1/services/tzmasters/favorite', (route) =>
		route.fulfill({ json: { ...detail, favorite: true } })
	);
	const shown: unknown[] = [];
	await page.route('**/api/v1/hidden', (route) =>
		route.fulfill({
			json: shown.length ? [] : [{ hostId: 'qnap', key: 'hawser', reason: 'agent', containers: 1 }]
		})
	);
	await page.route('**/api/v1/hidden/show', async (route) => {
		shown.push(route.request().postDataJSON());
		await route.fulfill({ json: detail });
	});
	await page.goto('/services');

	const star = page.getByRole('button', { name: 'Favori : TZ Masters' });
	await star.click();
	await expect(star).toHaveAttribute('aria-pressed', 'true');
	await page.getByRole('button', { name: /^Favoris/ }).click();
	await expect(page.getByRole('link', { name: 'TZ Masters' })).toBeVisible();
	await expect(page.getByRole('link', { name: /^Emby/ })).toHaveCount(0);
	await page.getByRole('button', { name: /^Tous/ }).click();

	const open = page.getByRole('link', { name: 'Ouvrir Emby dans un nouvel onglet' });
	const narrow = (page.viewportSize()?.width ?? 1440) <= 720;
	if (narrow) await expect(open).toBeHidden();
	else await expect(open).toHaveAttribute('href', 'http://x.fr:8096');

	await page.getByRole('button', { name: /^Masqués/ }).click();
	await expect(page.getByText('qnap · agent')).toBeVisible();
	await page.getByRole('button', { name: 'Réafficher hawser sur qnap' }).click();
	expect(shown).toEqual([{ hostId: 'qnap', key: 'hawser' }]);
	await expect(page.getByRole('button', { name: /^Masqués/ })).toHaveCount(0);
});
