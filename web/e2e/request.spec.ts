import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

const service = fixture('services-request') as {
	widget: { connection: Record<string, unknown> };
};

async function openTab(page: Page, detail: unknown = service) {
	await mockApi(page);
	await page.route('**/api/v1/services/request', (route) => route.fulfill({ json: detail }));
	await page.goto('/services/request#widgets');
}

test('la fiche montre compteurs, demandes à approuver, suivi, signalements et statistiques', async ({
	page
}) => {
	await openTab(page);

	const counts = page.getByRole('region', { name: 'Demandes', exact: true });
	await expect(counts.getByText('À approuver')).toBeVisible();
	await expect(counts.getByText('Mes demandes en cours')).toBeVisible();
	await expect(counts.getByText('Signalements ouverts')).toBeVisible();
	await expect(page.getByText('lecture et actions par Hub')).toBeVisible();

	const shelf = page.getByRole('region', { name: 'À approuver', exact: true });
	await expect(shelf.getByText('Matrix')).toBeVisible();
	await expect(shelf.getByText('Breaking Bad')).toBeVisible();
	await expect(shelf.getByText('Série · S1, S2')).toBeVisible();
	await expect(shelf.getByText('Bob · 4K')).toBeVisible();
	await expect(page.getByText('1 autre demande attend dans Request')).toHaveCount(0);

	const recent = page.getByRole('region', { name: 'Suivi des demandes' });
	await expect(recent.getByText('Disponible', { exact: true })).toBeVisible();
	await expect(recent.getByText('Échec', { exact: true })).toBeVisible();
	await expect(recent.getByText('Sonarr injoignable')).toBeVisible();
	await expect(recent.getByText('Refusée', { exact: true })).toBeVisible();
	await expect(recent.getByText('En attente', { exact: true })).toBeVisible();

	const issues = page.getByRole('region', { name: 'Signalements', exact: true });
	await expect(issues.getByText('Sous-titres · S1E3 · Bob')).toBeVisible();

	const stats = page.getByRole('region', { name: 'Statistiques' });
	await expect(stats.getByText('120 demandes')).toBeVisible();
	await expect(
		stats.getByRole('img', { name: /Demandes par mois.*120 demandes sur 12 mois/ })
	).toBeVisible();
	await expect(stats.getByText('1 j 2 h')).toBeVisible();
	await expect(stats.getByText('—')).toBeVisible();
	await expect(stats.getByText('8 %')).toBeVisible();
	await expect(stats.getByText('Alice')).toBeVisible();

	const axe = await new AxeBuilder({ page }).include('main').analyze();
	expect(axe.violations).toEqual([]);
});

test('approuver appelle la route et retire la demande', async ({ page }) => {
	await openTab(page);
	const calls: string[] = [];
	await page.route('**/api/v1/services/request/requests/12/approve', (route) => {
		calls.push(route.request().method());
		return route.fulfill({ status: 204 });
	});
	// Sans signal du serveur, c'est la relecture de la connexion qui confirme.
	let reads = 0;
	await page.route('**/api/v1/services/request', (route) => {
		reads++;
		return route.fulfill({ json: service });
	});
	await page.getByRole('button', { name: 'Approuver Matrix' }).click();
	await expect(page.getByRole('button', { name: 'Approuver Matrix' })).toHaveCount(0);
	await expect.poll(() => reads).toBeGreaterThan(0);
	await expect(
		page.getByRole('status').filter({ hasText: 'Matrix : demande approuvée.' })
	).toHaveCount(1);
	expect(calls).toEqual(['POST']);
	await expect(page.getByRole('button', { name: 'Approuver Breaking Bad' })).toBeVisible();
});

test('refuser demande un motif avant d’envoyer', async ({ page }) => {
	await openTab(page);
	const bodies: unknown[] = [];
	await page.route('**/api/v1/services/request/requests/13/decline', (route) => {
		bodies.push(route.request().postDataJSON());
		return route.fulfill({ status: 204 });
	});
	await page.getByRole('button', { name: 'Refuser Breaking Bad' }).click();
	const confirm = page.getByRole('button', { name: 'Confirmer le refus' });
	const reason = page.getByLabel('Motif du refus de Breaking Bad');
	await expect(reason).toBeFocused();
	await expect(confirm).toBeDisabled();
	await reason.fill('   ');
	await expect(confirm).toBeDisabled();
	await expect(reason).toHaveAttribute('maxlength', '500');
	await reason.fill('  Déjà dans la bibliothèque  ');
	await confirm.click();
	await expect(page.getByRole('button', { name: 'Refuser Breaking Bad' })).toHaveCount(0);
	expect(bodies).toEqual([{ reason: 'Déjà dans la bibliothèque' }]);
});

test('annuler un refus rend la main sans rien envoyer', async ({ page }) => {
	await openTab(page);
	await page.getByRole('button', { name: 'Refuser Matrix' }).click();
	await page.getByRole('button', { name: 'Annuler' }).click();
	await expect(page.getByLabel('Motif du refus de Matrix')).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Refuser Matrix' })).toBeFocused();
});

test('un 403 du serveur s’affiche tel quel et garde la demande', async ({ page }) => {
	await openTab(page);
	await page.route('**/api/v1/services/request/requests/12/approve', (route) =>
		route.fulfill({ status: 403, json: { error: 'La clé d’Request ne peut pas approuver.' } })
	);
	await page.getByRole('button', { name: 'Approuver Matrix' }).click();
	await expect(
		page
			.getByRole('alert')
			.filter({ hasText: 'Matrix : La clé d’Request ne peut pas approuver.' })
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Approuver Matrix' })).toBeEnabled();
});

test('la recherche propose de demander un titre', async ({ page }) => {
	await mockApi(page);
	const rows = fixture('widgets') as Record<string, unknown>[];
	await page.route('**/api/v1/widgets', (route) =>
		route.fulfill({
			json: [
				...rows,
				{
					slug: 'request',
					kind: 'request',
					name: 'Request',
					monogram: 'U',
					icon: null,
					color: '#7a5cff',
					hostId: 'mini-pc',
					hasUrl: true,
					support: 'ready',
					state: 'ok',
					dataAt: '2026-11-06T18:00:00Z',
					failingSince: null
				}
			]
		})
	);
	const searches: string[] = [];
	await page.route(/\/api\/v1\/services\/request\/search\?/, (route) => {
		searches.push(new URL(route.request().url()).searchParams.get('q') ?? '');
		return route.fulfill({
			json: {
				items: [
					{ mediaType: 'movie', tmdbId: 603, title: 'Matrix', year: 1999, status: null },
					{
						mediaType: 'movie',
						tmdbId: 604,
						title: 'Matrix Reloaded',
						year: 2003,
						status: 'available'
					},
					{ mediaType: 'tv', tmdbId: 605, title: 'Matrix, la série', status: 'requested' }
				]
			}
		});
	});
	const requests: unknown[] = [];
	await page.route('**/api/v1/services/request/requests', (route) => {
		requests.push(route.request().postDataJSON());
		return route.fulfill({ status: 201, json: {} });
	});

	await page.goto('/services');
	const search = page.locator('#catalogue-search');
	await search.fill('m');
	await page.waitForTimeout(500);
	expect(searches).toEqual([]);
	await expect(page.getByRole('heading', { name: 'Demander dans Request' })).toHaveCount(0);

	await search.fill('matrix');
	const shelf = page.getByRole('region', { name: 'Demander dans Request' });
	await expect(shelf.getByText('Matrix', { exact: true })).toBeVisible();
	expect(searches).toEqual(['matrix']);
	await expect(shelf.getByText('Disponible', { exact: true })).toBeVisible();
	await expect(shelf.getByText('Demandé', { exact: true })).toBeVisible();
	await expect(shelf.getByRole('button', { name: /Demander/ })).toHaveCount(1);

	await shelf.getByRole('button', { name: 'Demander Matrix' }).click();
	await expect(shelf.getByRole('button', { name: /Demander/ })).toHaveCount(0);
	await expect(shelf.getByText('Demandé', { exact: true })).toHaveCount(2);
	expect(requests).toEqual([{ mediaType: 'movie', tmdbId: 603 }]);
});

test('sans connexion Request, la recherche reste celle du catalogue', async ({ page }) => {
	await mockApi(page);
	await page.goto('/services');
	await page.locator('#catalogue-search').fill('matrix');
	await page.waitForTimeout(500);
	await expect(page.getByRole('heading', { name: 'Demander dans Request' })).toHaveCount(0);
});

test('le secret du webhook n’apparaît qu’une fois', async ({ page }) => {
	await openTab(page);
	let configured = false;
	await page.route('**/api/v1/services/request', (route) =>
		route.fulfill({
			json: {
				...service,
				widget: {
					...service.widget,
					connection: { ...service.widget.connection, webhook: { configured } }
				}
			}
		})
	);
	await page.route('**/api/v1/services/request/connection/webhook-secret', (route) => {
		configured = true;
		return route.fulfill({
			json: {
				url: 'https://hub.example/api/v1/hooks/request',
				secret: 'secret-de-test-0123456789'
			}
		});
	});
	await page.getByRole('button', { name: 'Gérer la connexion' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText('Webhook non configuré')).toBeVisible();
	await dialog.getByRole('button', { name: 'Générer le secret' }).click();
	await expect(dialog.getByLabel('Secret')).toHaveValue('secret-de-test-0123456789');
	await expect(dialog.getByLabel('URL')).toHaveValue(
		'https://hub.example/api/v1/hooks/request'
	);
	await expect(
		dialog.getByText(
			'Réglages › Webhook sortant, colle l’URL et mets le secret dans Authorization.'
		)
	).toBeVisible();
	await expect(dialog.getByText('Webhook configuré')).toBeVisible();
	await expect(dialog.getByRole('button', { name: 'Régénérer le secret' })).toBeVisible();
	const axe = await new AxeBuilder({ page }).include('dialog').analyze();
	expect(axe.violations).toEqual([]);

	await dialog.getByRole('button', { name: 'Fermer' }).click();
	await page.getByRole('button', { name: 'Gérer la connexion' }).click();
	await expect(page.getByRole('dialog').getByText('Webhook configuré')).toBeVisible();
	await expect(page.getByText('secret-de-test-0123456789')).toHaveCount(0);
	await expect(page.getByLabel('Secret')).toHaveCount(0);
});

test('la fiche dit ce que Hub fait avec Request, et propose 60 s par défaut', async ({
	page
}) => {
	await openTab(page, {
		...service,
		widget: { ...service.widget, connection: undefined, refreshChoices: [30, 60, 300] }
	});
	await page.getByRole('button', { name: 'Connecter Request' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText('Lire, approuver, demander')).toBeVisible();
	await expect(dialog.getByText('Il ne touche jamais aux réglages.')).toBeVisible();
	await expect(dialog.getByText('Lire, et rien d’autre')).toHaveCount(0);
	await page.route('**/api/v1/services/request/connection/test', (route) =>
		route.fulfill({ json: { result: 'ok', latencyMs: 20, note: '3 demandes à approuver' } })
	);
	await dialog.getByLabel('Clé d’API').fill('ur_test');
	await dialog.getByRole('button', { name: 'Tester la clé' }).click();
	await expect(dialog.getByLabel('Rafraîchir')).toHaveValue('60');
});

test('une connexion Seerr gardée ne se lit plus et ne peut que se retirer', async ({ page }) => {
	const seerr = {
		...service,
		slug: 'seerr',
		name: 'Seerr',
		widget: {
			support: '',
			connection: {
				state: 'unsupported',
				keyHint: '1a2b',
				refreshSeconds: 60,
				enabled: ['requests'],
				options: {},
				dataAt: null,
				checkedAt: null,
				failingSince: null,
				data: {}
			}
		}
	};
	await mockApi(page);
	await page.route('**/api/v1/services/seerr', (route) => route.fulfill({ json: seerr }));
	await page.goto('/services/seerr#widgets');
	await expect(page.getByRole('heading', { name: 'Plus pris en charge' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Gérer la connexion' })).toHaveCount(0);

	let deleted = 0;
	await page.route('**/api/v1/services/seerr/connection', (route) => {
		deleted++;
		return route.fulfill({ json: { ...seerr, widget: { support: '' } } });
	});
	await page.getByRole('button', { name: 'Retirer la connexion' }).click();
	expect(deleted).toBe(0);
	await page.getByRole('button', { name: 'Confirmer : retirer la connexion' }).click();
	await expect(page.getByRole('heading', { name: 'Plus pris en charge' })).toHaveCount(0);
	expect(deleted).toBe(1);
});
