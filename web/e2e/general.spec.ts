import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

type Json = Record<string, unknown>;

/** Le serveur simulé garde ce qu'on lui envoie, comme le vrai. */
async function mockGeneral(page: Page) {
	const sent: Json[] = [];
	let current = fixture('settings-general') as Json;
	await page.route('**/api/v1/settings/general', async (route) => {
		if (route.request().method() === 'PUT') {
			const body = route.request().postDataJSON() as Json;
			sent.push(body);
			current = { ...current, ...body };
		}
		return route.fulfill({ json: current });
	});
	return sent;
}

test('la section montre les réglages et ce que dit le .env', async ({ page }) => {
	await mockApi(page);
	await mockGeneral(page);
	await page.goto('/settings#general');
	const panel = page.locator('#general');
	await expect(panel.getByRole('heading', { name: 'Général', level: 2 })).toBeVisible();
	await expect(panel.getByLabel('Nom affiché')).toHaveValue('Hub');
	await expect(panel.getByLabel('Page d’ouverture')).toHaveValue('home');
	await expect(panel.getByLabel('Fuseau horaire')).toHaveValue('');
	await expect(panel.getByLabel('Fuseau horaire').locator('option').first()).toHaveText(
		'Celui du serveur (Europe/Paris)'
	);
	await expect(panel.getByLabel('Rafraîchissement des widgets')).toHaveValue('normal');

	const about = page.locator('#about');
	await expect(about.getByRole('heading', { name: 'À propos' })).toBeVisible();
	await expect(about.getByText('0.4.0')).toBeVisible();
	await expect(about.getByText('https://home.example.test:8444')).toBeVisible();
	await expect(about.getByText('https://192.0.2.14:8443')).toBeVisible();
	// Ce que le serveur ne sait pas, il le dit, sans inventer.
	await expect(about.getByText('Non renseigné (HUB_HOST_NAME)')).toBeVisible();
	await expect(about.getByText(/viennent du fichier .env/)).toBeVisible();
	await expect(page.getByRole('link', { name: 'Général' })).toHaveAttribute('href', '#general');

	// À 390 px comme à 1440 px : rien ne déborde de la page.
	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth - document.documentElement.clientWidth
	);
	expect(overflow).toBeLessThanOrEqual(0);
});

test('chaque choix s’enregistre, le nom change l’onglet', async ({ page }) => {
	await mockApi(page);
	const sent = await mockGeneral(page);
	await page.goto('/settings#general');
	const panel = page.locator('#general');

	await panel.getByLabel('Page d’ouverture').selectOption('infra');
	await expect(panel.getByRole('status')).toHaveText('Enregistré.');
	await panel.getByLabel('Rafraîchissement des widgets').selectOption('economy');
	await panel.getByLabel('Fuseau horaire').selectOption('UTC');

	const name = panel.getByLabel('Nom affiché');
	await name.fill('   ');
	await name.press('Enter');
	await expect(panel.getByText('Le nom ne peut pas être vide.')).toBeVisible();
	await expect(name).toHaveAttribute('aria-invalid', 'true');
	await name.fill(' Hub de Quentin ');
	await name.press('Enter');
	await expect(page).toHaveTitle('Réglages · Hub de Quentin');
	await panel.getByLabel('Historique des services').selectOption('30');
	await expect(panel.getByRole('status')).toHaveText('Enregistré.');

	expect(sent.at(-1)).toMatchObject({ historyDays: 30 });
	expect(sent.slice(0, -1)).toEqual([
		{
			displayName: 'Hub',
			startPage: 'infra',
			timeZone: '',
			widgetRefresh: 'normal',
			historyDays: 90
		},
		{
			displayName: 'Hub',
			startPage: 'infra',
			timeZone: '',
			widgetRefresh: 'economy',
			historyDays: 90
		},
		{
			displayName: 'Hub',
			startPage: 'infra',
			timeZone: 'UTC',
			widgetRefresh: 'economy',
			historyDays: 90
		},
		{
			displayName: 'Hub de Quentin',
			startPage: 'infra',
			timeZone: 'UTC',
			widgetRefresh: 'economy',
			historyDays: 90
		}
	]);
});

test('un refus du serveur est dit, la valeur reste celle enregistrée', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/settings/general', (route) =>
		route.request().method() === 'PUT'
			? route.fulfill({ status: 400, json: { error: 'fuseau horaire inconnu' } })
			: route.fulfill({ json: fixture('settings-general') })
	);
	await page.goto('/settings#general');
	const panel = page.locator('#general');
	await panel.getByLabel('Fuseau horaire').selectOption('UTC');
	await expect(panel.getByRole('status')).toHaveText(
		"Échec de l'enregistrement : fuseau horaire inconnu"
	);
});

test('Hub s’ouvre sur la page choisie, « Accueil » reste l’accueil', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/auth/state', (route) =>
		route.fulfill({ json: { ...(fixture('auth-state') as Json), startPage: 'medias' } })
	);
	await page.goto('/');
	await expect(page).toHaveURL('/medias');
	await expect(page.getByRole('heading', { level: 1, name: 'Médias' })).toBeVisible();

	await page.getByRole('link', { name: 'Accueil' }).first().click();
	await expect(page).toHaveURL('/');
	await expect(page).toHaveTitle('Accueil · Hub');
});

test('une adresse précise s’ouvre telle quelle', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/v1/auth/state', (route) =>
		route.fulfill({ json: { ...(fixture('auth-state') as Json), startPage: 'infra' } })
	);
	await page.goto('/services');
	await expect(page).toHaveURL('/services');
});

test('l’écran de connexion porte le nom affiché', async ({ page }) => {
	await mockApi(page, { authenticated: false });
	await page.route('**/api/v1/auth/state', (route) =>
		route.fulfill({
			json: {
				setupRequired: false,
				authenticated: false,
				passkeys: false,
				displayName: 'Hub de Quentin'
			}
		})
	);
	await page.goto('/');
	await expect(page).toHaveURL('/login');
	await expect(page.getByText('Hub de Quentin', { exact: true })).toBeVisible();
	await expect(page).toHaveTitle('Connexion · Hub de Quentin');
});
