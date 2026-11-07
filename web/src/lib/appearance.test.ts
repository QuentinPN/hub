import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { attributes, DEFAULTS, parse, STORAGE_KEY, THEME_COLORS } from './appearance.ts';

describe('parse', () => {
	it('retombe sur les défauts sans valeur ou avec une valeur abîmée', () => {
		expect(parse(null)).toEqual(DEFAULTS);
		expect(parse('pas du json')).toEqual(DEFAULTS);
		expect(parse('"texte"')).toEqual(DEFAULTS);
		expect(parse('{"theme":"rose","density":7,"reduceMotion":"oui"}')).toEqual(DEFAULTS);
	});

	it('garde les valeurs connues', () => {
		const a = { theme: 'system', density: 'compact', lessTransparency: true, reduceMotion: true };
		expect(parse(JSON.stringify(a))).toEqual(a);
	});
});

describe('attributes', () => {
	it('sans réglage : sombre, rien d’autre', () => {
		expect(attributes(DEFAULTS, true)).toEqual({
			'data-theme': 'dark',
			'data-density': null,
			'data-transparency': null,
			'data-motion': null
		});
	});

	it('« Comme l’appareil » suit l’appareil', () => {
		const a = { ...DEFAULTS, theme: 'system' as const };
		expect(attributes(a, true)['data-theme']).toBe('light');
		expect(attributes(a, false)['data-theme']).toBe('dark');
	});
});

// Le script inline d'app.html doit poser exactement les mêmes attributs que attributes().
describe('script inline d’app.html', () => {
	const html = readFileSync(new URL('../app.html', import.meta.url), 'utf8');
	const body = /<script>([\s\S]*?)<\/script>/.exec(html)?.[1] ?? '';

	function run(stored: string | null, deviceLight: boolean) {
		const attrs: Record<string, string> = {};
		const meta = { setAttribute: (_k: string, v: string) => (attrs['theme-color'] = v) };
		const document = {
			documentElement: { setAttribute: (k: string, v: string) => (attrs[k] = v) },
			querySelector: () => meta
		};
		const localStorage = { getItem: (k: string) => (k === STORAGE_KEY ? stored : null) };
		const matchMedia = (q: string) => ({ matches: q.includes('light') && deviceLight });
		new Function('document', 'localStorage', 'matchMedia', body)(
			document,
			localStorage,
			matchMedia
		);
		return attrs;
	}

	const cases: [string | null, boolean][] = [
		[null, false],
		['{', true],
		['{"theme":"light"}', false],
		['{"theme":"system"}', true],
		['{"theme":"system"}', false],
		['{"density":"compact","lessTransparency":true,"reduceMotion":true}', false]
	];
	for (const [stored, light] of cases) {
		it(`${stored ?? 'rien'}, appareil ${light ? 'clair' : 'sombre'}`, () => {
			const attrs = attributes(parse(stored), light);
			const expected = {
				...Object.fromEntries(Object.entries(attrs).filter(([, v]) => v !== null)),
				'theme-color': THEME_COLORS[attrs['data-theme'] === 'light' ? 'light' : 'dark']
			};
			expect(run(stored, light)).toEqual(expected);
		});
	}

	it('survit à un stockage bloqué', () => {
		const document = { documentElement: { setAttribute: () => {} }, querySelector: () => null };
		const localStorage = {
			getItem: () => {
				throw new Error('SecurityError');
			}
		};
		expect(() =>
			new Function('document', 'localStorage', 'matchMedia', body)(document, localStorage, () => ({
				matches: false
			}))
		).not.toThrow();
	});
});
