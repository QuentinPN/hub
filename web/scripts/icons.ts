// Régénère les images de repli du logo à partir des SVG sources :
//   static/favicon.svg        → static/favicon.ico (16, 32 et 48 px)
//   icons/apple-touch-icon.svg → static/apple-touch-icon.png (180 px)
// Rendu par le Chromium de Playwright, le même moteur que les navigateurs. Lancer depuis web/ :
//   node scripts/icons.ts   (PLAYWRIGHT_CHANNEL=chrome pour le Chrome installé)
import { chromium } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';

const browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || undefined });
const page = await browser.newPage();

async function render(svgPath: string, size: number): Promise<Buffer> {
	const svg = await readFile(svgPath, 'utf8');
	await page.setViewportSize({ width: size, height: size });
	await page.setContent(
		`<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`
	);
	return page.locator('svg').screenshot({ omitBackground: true });
}

// Un .ico peut contenir des PNG tels quels (Windows Vista et suivants, tous les navigateurs).
function ico(pngs: { size: number; data: Buffer }[]): Buffer {
	const header = Buffer.alloc(6);
	header.writeUInt16LE(1, 2); // type : icône
	header.writeUInt16LE(pngs.length, 4);
	let offset = 6 + 16 * pngs.length;
	const entries = pngs.map(({ size, data }) => {
		const entry = Buffer.alloc(16);
		entry.writeUInt8(size % 256, 0);
		entry.writeUInt8(size % 256, 1);
		entry.writeUInt16LE(1, 4); // plans
		entry.writeUInt16LE(32, 6); // bits par pixel
		entry.writeUInt32LE(data.length, 8);
		entry.writeUInt32LE(offset, 12);
		offset += data.length;
		return entry;
	});
	return Buffer.concat([header, ...entries, ...pngs.map((p) => p.data)]);
}

const sizes = [16, 32, 48];
const pngs = [];
for (const size of sizes) {
	pngs.push({ size, data: await render('static/favicon.svg', size) });
}
await writeFile('static/favicon.ico', ico(pngs));
await writeFile('static/apple-touch-icon.png', await render('icons/apple-touch-icon.svg', 180));
await browser.close();
