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

export function applyTo(root: HTMLElement, a: Appearance, deviceLight: boolean) {
	for (const [name, value] of Object.entries(attributes(a, deviceLight))) {
		if (value === null) root.removeAttribute(name);
		else root.setAttribute(name, value);
	}
}
