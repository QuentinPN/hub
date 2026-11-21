import { defineConfig, devices } from '@playwright/test';

// Tests de l'UI seule : l'API est simulée (e2e/api.ts) à partir de réponses enregistrées du
// serveur Go. Les parcours de bout en bout avec la base et les agents viendront en phase 5.
// PLAYWRIGHT_CHANNEL=chrome utilise le Chrome installé au lieu du Chromium de Playwright.
const channel = process.env.PLAYWRIGHT_CHANNEL || undefined;
// PLAYWRIGHT_PORT sépare les worktrees qui testent en même temps (reuseExistingServer).
const port = Number(process.env.PLAYWRIGHT_PORT) || 4173;

export default defineConfig({
	testDir: 'e2e',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	// Jamais de nouvelle tentative : elle transforme un test instable en test vert. Un échec se corrige à sa cause.
	retries: 0,
	reporter: process.env.CI ? 'github' : 'list',
	use: {
		baseURL: `http://localhost:${port}`,
		trace: 'retain-on-failure'
	},
	// Les deux tailles des maquettes de référence (DESIGN.md).
	projects: [
		{
			name: 'desktop',
			use: { ...devices['Desktop Chrome'], channel, viewport: { width: 1440, height: 900 } }
		},
		{
			name: 'mobile',
			use: {
				...devices['Desktop Chrome'],
				channel,
				viewport: { width: 390, height: 844 },
				hasTouch: true
			}
		}
	],
	webServer: {
		command: `npm run build && npm run preview -- --port ${port} --strictPort`,
		port,
		reuseExistingServer: !process.env.CI,
		timeout: 120_000
	}
});
