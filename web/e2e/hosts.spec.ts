import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { hostsNow, mockApi } from './api.ts';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

const hostRow = (page: Page, name: string) =>
	page.locator('#hosts li').filter({ has: page.getByText(name, { exact: true }) });

async function noViolations(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(TAGS).analyze();
	expect(violations.map((v) => v.id)).toEqual([]);
}

test.beforeEach(async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings');
	await expect(page.getByRole('heading', { level: 2, name: 'Hôtes et agents' })).toBeVisible();
});

test('chaque hôte montre l’état de son agent', async ({ page }) => {
	await expect(page.getByText('4 hôtes · 2 agents connectés')).toBeVisible();
	await expect(hostRow(page, 'QNAP')).toContainText(/Connecté · vu il y a \d+\ss/);
	await expect(hostRow(page, 'QNAP')).toContainText('renouvelé seul vers le');
	await expect(hostRow(page, 'Raspberry Pi')).toContainText('Docker injoignable');
	await expect(hostRow(page, 'Raspberry Pi')).toContainText(
		'Ses 2 conteneurs sont affichés « inconnu »'
	);
	await expect(hostRow(page, 'mini-PC Geekom')).toContainText('En attente d’enrôlement');
	await expect(hostRow(page, 'mini-PC Geekom')).toContainText(/expire dans 1[01] min/);
	await expect(hostRow(page, 'debian-mini')).toContainText('Aucun agent');
	await noViolations(page);
});

test('les conteneurs d’un hôte, puis la révocation de son agent', async ({ page }) => {
	const revoked: string[] = [];
	await page.route('**/api/v1/hosts/*/revoke', (route) => {
		revoked.push(new URL(route.request().url()).pathname);
		return route.fulfill({ json: { revoked: 1 } });
	});

	const toggle = page.getByRole('button', { name: 'Conteneurs de QNAP' });
	await toggle.click();
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	const list = page.locator('#containers-qnap');
	// Trois conteneurs ; sur mobile, l'en-tête du tableau disparaît au profit d'une carte par ligne.
	await expect(list.getByRole('row', { name: /gluetun|qbittorrent|watchtower/ })).toHaveCount(3);
	await expect(list.getByRole('row', { name: /gluetun/ })).toContainText('6 labels, 2 masqués');
	await expect(list.getByRole('row', { name: /watchtower/ })).toContainText('Exited (0)');
	await noViolations(page);

	await list.getByRole('button', { name: 'Révoquer l’agent' }).click();
	const dialog = page.getByRole('alertdialog', { name: 'Révoquer l’agent de QNAP ?' });
	await expect(dialog).toContainText('Les 5 conteneurs restent dans l’inventaire');
	await noViolations(page);
	await dialog.getByRole('button', { name: 'Annuler' }).click();
	await expect(dialog).toBeHidden();
	expect(revoked).toEqual([]);

	await list.getByRole('button', { name: 'Révoquer l’agent' }).click();
	await dialog.getByRole('button', { name: 'Révoquer l’agent' }).click();
	await expect(dialog).toBeHidden();
	await expect(page.getByRole('status').filter({ hasText: 'Agent de QNAP révoqué' })).toBeVisible();
	expect(revoked).toEqual(['/api/v1/hosts/qnap/revoke']);
});

test('ajouter un hôte : code, instructions, puis premier contact', async ({ page }) => {
	const bodies: unknown[] = [];
	await page.route('**/api/v1/hosts/enrollments', (route) => {
		bodies.push(route.request().postDataJSON());
		return route.fulfill({
			status: 201,
			json: {
				hostId: 'nas-du-salon',
				code: 'Zr4w9Qm1c0de-one-time-only_xyz',
				expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
				pin: 'sha256:9f2c',
				serverUrl: 'https://192.0.2.14:8443'
			}
		});
	});

	await page.getByRole('button', { name: 'Ajouter un hôte' }).click();
	const dialog = page.getByRole('dialog', { name: 'Ajouter un hôte' });
	await dialog.getByLabel('Nom affiché').fill('NAS du salon');
	await expect(dialog.getByLabel('Identifiant')).toHaveValue('nas-du-salon');
	await noViolations(page);

	// Vérifié avant l'envoi : rien ne part.
	await dialog.getByLabel('Identifiant').fill('NAS!');
	await dialog.getByRole('button', { name: 'Créer un code' }).click();
	await expect(dialog.getByRole('alert')).toContainText('lettres minuscules');
	expect(bodies).toEqual([]);

	await dialog.getByLabel('Identifiant').fill('nas-du-salon');
	await dialog.getByRole('button', { name: 'Créer un code' }).click();
	expect(bodies).toEqual([{ hostId: 'nas-du-salon', name: 'NAS du salon' }]);

	const code = dialog.getByRole('status', { name: 'Code d’enrôlement' });
	await expect(code).not.toContainText('Zr4w9');
	await expect(dialog).toContainText(/expire dans 1[45]:\d\d/);
	await dialog.getByRole('button', { name: 'Afficher' }).click();
	await expect(code).toHaveText('Zr4w9Qm1c0de-one-time-only_xyz');
	await expect(dialog.locator('pre').first()).toContainText(
		'HUB_SERVER_URL=https://192.0.2.14:8443'
	);
	await expect(dialog.locator('pre').first()).toContainText('HUB_CA_PIN=sha256:9f2c');
	await expect(dialog.getByRole('status').filter({ hasText: 'En attente' })).toBeVisible();
	await noViolations(page);

	// L'agent se présente : la liste relue le montre, la fenêtre l'annonce.
	const withNew = hostsNow() as { hosts: object[] };
	withNew.hosts.push({
		id: 'nas-du-salon',
		name: 'NAS du salon',
		os: '',
		arch: '',
		state: 'connected',
		agentVersion: '0.1.0',
		dockerVersion: '27.1.2',
		lastSeenAt: new Date().toISOString(),
		certExpiresAt: new Date(Date.now() + 30 * 86400_000).toISOString(),
		enrollmentExpiresAt: null,
		containers: 0
	});
	await page.route('**/api/v1/hosts', (route) => route.fulfill({ json: withNew }));
	await expect(dialog.getByText('Agent enrôlé.')).toBeVisible({ timeout: 8_000 });
	await expect(dialog.getByText('Code utilisé')).toBeVisible();
	await expect(dialog.getByRole('button', { name: 'Annuler le code' })).toHaveCount(0);
	await dialog.getByRole('button', { name: 'Fermer' }).last().click();
	await expect(dialog).toBeHidden();
	await expect(hostRow(page, 'NAS du salon')).toContainText('Connecté');
});

test('annuler le code d’un hôte en attente', async ({ page }) => {
	const cancelled: string[] = [];
	await page.route('**/api/v1/hosts/*/enrollment', (route) => {
		cancelled.push(`${route.request().method()} ${new URL(route.request().url()).pathname}`);
		return route.fulfill({ status: 204 });
	});
	await hostRow(page, 'mini-PC Geekom').getByRole('button', { name: 'Annuler le code' }).click();
	await expect(
		page.getByRole('status').filter({ hasText: 'Code de mini-PC Geekom annulé' })
	).toBeVisible();
	expect(cancelled).toEqual(['DELETE /api/v1/hosts/hub-homeserver/enrollment']);
});

test('sans port agents, les hôtes restent listés mais rien ne s’enrôle', async ({ page }) => {
	await page.route('**/api/v1/hosts', (route) =>
		route.fulfill({ json: { ...(hostsNow() as object), agentsEnabled: false } })
	);
	await page.reload();
	await expect(page.getByRole('note').filter({ hasText: 'Port des agents éteint' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Ajouter un hôte' })).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Enrôler debian-mini' })).toHaveCount(0);
});
