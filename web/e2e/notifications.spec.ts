import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { fixture, mockApi } from './api.ts';

type Status = Record<string, unknown> & {
	ntfy: Record<string, unknown>;
	smtp: Record<string, unknown>;
	alerts: { id: string; enabled: boolean; delaySeconds: number }[];
	quietHours: Record<string, unknown>;
};

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

// Le test de la veille, daté par rapport à maintenant pour que « hier » garde son sens.
function base(): Status {
	const st = structuredClone(fixture('notifications')) as Status;
	const yesterday = new Date();
	yesterday.setDate(yesterday.getDate() - 1);
	st.ntfy.last = { at: yesterday.toISOString(), ok: true, test: true, error: '' };
	return st;
}

async function open(page: Page, st: Status = base()) {
	await mockApi(page);
	await page.route('**/api/v1/notifications', (route) => route.fulfill({ json: st }));
	await page.goto('/settings#notifications');
	return page.locator('#notifications');
}

test('la section montre les canaux, les six alertes et les heures calmes', async ({
	page
}, testInfo) => {
	const panel = await open(page);
	await expect(panel.getByRole('heading', { name: 'Notifications', level: 2 })).toBeVisible();
	await expect(panel.getByText('Envoi testé hier')).toBeVisible();
	await expect(panel.getByText('https://ntfy.sh · sujet')).toBeVisible();
	await expect(panel.getByText('hub-•••••••')).toBeVisible();
	await expect(panel.getByText('Pas configuré')).toBeVisible();
	await expect(panel.getByRole('button', { name: 'Configurer' })).toBeVisible();

	const boxes = panel.getByRole('listitem').getByRole('checkbox');
	await expect(boxes).toHaveCount(6);
	const mobile = testInfo.project.name === 'mobile';
	await expect(
		panel.getByRole('checkbox', {
			name: mobile ? /Service arrêté ou injoignable/ : /Un service est arrêté ou injoignable/
		})
	).toBeChecked();
	await expect(
		panel.getByRole('checkbox', {
			name: mobile ? /Disque au-delà de 90 %/ : /Un disque dépasse 90 %/
		})
	).not.toBeChecked();
	await expect(
		panel.getByRole('checkbox', { name: mobile ? /Nouvelle connexion/ : /Une nouvelle connexion/ })
	).not.toBeChecked();
	await expect(panel.getByRole('checkbox', { name: /Heures calmes/ })).toBeChecked();
	if (mobile) {
		await expect(panel.getByText('23:00 à 08:00, résumé le matin')).toBeVisible();
		await expect(panel.getByText('Mini-PC, QNAP, Raspberry Pi.')).toBeHidden();
	} else {
		await expect(panel.getByText('Un hôte injoignable passe quand même.')).toBeVisible();
		await expect(panel.locator('.delay', { hasText: 'après 15 min' })).toBeVisible();
	}

	const scan = await new AxeBuilder({ page }).include('#notifications').withTags(TAGS).analyze();
	expect(scan.violations.map((v) => v.id)).toEqual([]);
	const [width, viewport] = await page.evaluate(() => [
		document.documentElement.scrollWidth,
		window.innerWidth
	]);
	expect(width).toBeLessThanOrEqual(viewport);
});

test('cocher une alerte enregistre toute la liste', async ({ page }) => {
	const st = base();
	const panel = await open(page, st);
	const sent: unknown[] = [];
	await page.route('**/api/v1/notifications/alerts', (route) => {
		const body = route.request().postDataJSON() as {
			alerts: Record<string, boolean>;
			quietHours: boolean;
		};
		sent.push(body);
		return route.fulfill({
			json: {
				...st,
				alerts: st.alerts.map((a) => ({ ...a, enabled: body.alerts[a.id] })),
				quietHours: { ...st.quietHours, enabled: body.quietHours }
			}
		});
	});
	await panel.getByRole('checkbox', { name: /disque/i }).check();
	await expect(panel.getByRole('checkbox', { name: /disque/i })).toBeChecked();
	await panel.getByRole('checkbox', { name: /Heures calmes/ }).uncheck();
	await expect(panel.getByRole('checkbox', { name: /Heures calmes/ })).not.toBeChecked();
	expect(sent).toEqual([
		{
			alerts: { service: true, host: true, widget: true, backup: true, disk: true, login: false },
			quietHours: true
		},
		{
			alerts: { service: true, host: true, widget: true, backup: true, disk: true, login: false },
			quietHours: false
		}
	]);
});

test('configurer l’e-mail envoie le formulaire, puis le test dit l’échec', async ({ page }) => {
	const st = base();
	const panel = await open(page, st);
	const saved: unknown[] = [];
	const configured = {
		...st,
		smtp: {
			configured: true,
			host: 'smtp.exemple.fr',
			port: 465,
			username: 'hub@exemple.fr',
			passwordSet: true,
			from: 'hub@exemple.fr',
			to: 'moi@exemple.fr',
			last: null
		}
	};
	await page.route('**/api/v1/notifications/smtp', (route) => {
		saved.push(route.request().postDataJSON());
		return route.fulfill({ json: configured });
	});
	await page.route('**/api/v1/notifications/smtp/test', (route) =>
		route.fulfill({
			json: {
				...configured,
				smtp: {
					...configured.smtp,
					last: {
						at: new Date().toISOString(),
						ok: false,
						test: true,
						error: 'identifiants refusés (535)'
					}
				}
			}
		})
	);
	await panel.getByRole('button', { name: 'Configurer' }).click();
	await panel.getByLabel('Serveur SMTP').fill('smtp.exemple.fr');
	await panel.getByLabel('Port').fill('465');
	await panel.getByLabel('Compte').fill('hub@exemple.fr');
	await panel.getByLabel('Mot de passe').fill('mot-de-passe-de-test');
	await panel.getByLabel('Expéditeur').fill('hub@exemple.fr');
	await panel.getByLabel('Destinataire').fill('moi@exemple.fr');
	await panel.getByRole('button', { name: 'Enregistrer' }).click();
	await expect(panel.getByText('smtp.exemple.fr:465 · vers')).toBeVisible();
	expect(saved).toEqual([
		{
			host: 'smtp.exemple.fr',
			port: 465,
			username: 'hub@exemple.fr',
			password: 'mot-de-passe-de-test',
			from: 'hub@exemple.fr',
			to: 'moi@exemple.fr'
		}
	]);
	// Le mot de passe n'est jamais réaffiché : il ne reste nulle part dans la page.
	await expect(page.locator('input[type=password]')).toHaveCount(0);

	await panel.getByRole('button', { name: 'Envoyer un test' }).nth(1).click();
	await expect(panel.getByRole('status')).toHaveText(
		/Test non parti : identifiants refusés \(535\)/
	);
	await expect(panel.getByText(/^Échec à l’instant$/)).toBeVisible();
});

test('modifier ntfy sans ressaisir le sujet le garde, le refus du serveur s’affiche', async ({
	page
}) => {
	const panel = await open(page);
	const sent: unknown[] = [];
	await page.route('**/api/v1/notifications/ntfy', (route) => {
		sent.push(route.request().postDataJSON());
		return route.fulfill({
			status: 400,
			json: { error: "saisir le sujet : il n'est envoyé qu'au serveur pour lequel il a été saisi" }
		});
	});
	await panel.getByRole('button', { name: 'Modifier' }).click();
	await expect(panel.getByLabel('Sujet')).toHaveValue('');
	await expect(panel.getByLabel('Sujet')).toHaveAttribute('placeholder', 'Inchangé si vide');
	await panel.getByLabel('Adresse du serveur').fill('https://ntfy.exemple.fr');
	await panel.getByRole('button', { name: 'Enregistrer' }).click();
	await expect(panel.getByRole('alert')).toContainText('saisir le sujet');
	expect(sent).toEqual([{ url: 'https://ntfy.exemple.fr', topic: '' }]);
});

test('un échec d’envoi se voit sur le canal', async ({ page }) => {
	const st = base();
	st.ntfy.last = {
		at: new Date().toISOString(),
		ok: false,
		test: false,
		error: 'serveur injoignable'
	};
	const panel = await open(page, st);
	await expect(panel.getByText('Échec à l’instant')).toBeVisible();
	await expect(panel.getByText('serveur injoignable')).toBeVisible();
});
