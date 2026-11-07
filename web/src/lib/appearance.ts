// Apparence de Hub, gardée par navigateur (localStorage) : rien ne passe par le serveur.
// Le script inline d'app.html applique la même chose avant le premier rendu ; appearance.test.ts
// vérifie que les deux donnent les mêmes attributs.

export type Theme = 'dark' | 'light' | 'system';
export type Density = 'comfortable' | 'compact';

export interface Appearance {
	theme: Theme;
	density: Density;
	// Faux = suivre l'appareil (prefers-reduced-transparency / prefers-reduced-motion).
	lessTransparency: boolean;
	reduceMotion: boolean;
}

export const STORAGE_KEY = 'hub.appearance';

export const DEFAULTS: Appearance = {
	theme: 'dark',
	density: 'comfortable',
	lessTransparency: false,
	reduceMotion: false
};

// Une valeur abîmée ou d'une ancienne version retombe sur le défaut, champ par champ.
export function parse(raw: string | null): Appearance {
	let v: Record<string, unknown> = {};
	try {
		const parsed: unknown = JSON.parse(raw ?? '{}');
		if (parsed && typeof parsed === 'object') v = parsed as Record<string, unknown>;
	} catch {
		// valeur illisible : défauts
	}
	return {
		theme: v.theme === 'light' || v.theme === 'system' ? v.theme : 'dark',
		density: v.density === 'compact' ? 'compact' : 'comfortable',
		lessTransparency: v.lessTransparency === true,
		reduceMotion: v.reduceMotion === true
	};
}

// Attributs data-* posés sur <html> ; null = attribut absent.
export function attributes(a: Appearance, deviceLight: boolean): Record<string, string | null> {
	return {
		'data-theme': a.theme === 'light' || (a.theme === 'system' && deviceLight) ? 'light' : 'dark',
		'data-density': a.density === 'compact' ? 'compact' : null,
		'data-transparency': a.lessTransparency ? 'reduce' : null,
		'data-motion': a.reduceMotion ? 'reduce' : null
	};
}

// Couleur de la barre du navigateur (mobile), celle de --bg.
export const THEME_COLORS = { dark: '#07080A', light: '#F2F2F5' } as const;

export function applyTo(doc: Document, a: Appearance, deviceLight: boolean) {
	const attrs = attributes(a, deviceLight);
	for (const [name, value] of Object.entries(attrs)) {
		if (value === null) doc.documentElement.removeAttribute(name);
		else doc.documentElement.setAttribute(name, value);
	}
	doc
		.querySelector('meta[name="theme-color"]')
		?.setAttribute('content', THEME_COLORS[attrs['data-theme'] === 'light' ? 'light' : 'dark']);
}
