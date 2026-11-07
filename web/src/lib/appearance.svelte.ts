import { applyTo, parse, STORAGE_KEY, type Appearance } from './appearance.ts';

const LIGHT = '(prefers-color-scheme: light)';

function read(): Appearance {
	try {
		return parse(localStorage.getItem(STORAGE_KEY));
	} catch {
		// stockage bloqué (navigation privée stricte) : défauts
		return parse(null);
	}
}

function deviceMatches(query: string): boolean {
	return typeof matchMedia === 'function' && matchMedia(query).matches;
}

// État partagé par la section Apparence et le layout. Le script inline d'app.html a déjà posé
// les attributs : on ne fait que relire la même valeur.
export const appearance = $state<Appearance>(read());

export function setAppearance(patch: Partial<Appearance>) {
	Object.assign(appearance, patch);
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(appearance));
	} catch {
		// stockage plein ou bloqué : le choix vaut pour cette page seulement
	}
	applyTo(document.documentElement, appearance, deviceMatches(LIGHT));
}

// « Comme l'appareil » suit l'appareil en direct ; un autre onglet qui change l'apparence aussi.
export function followDevice(): () => void {
	const media = matchMedia(LIGHT);
	const reapply = () => applyTo(document.documentElement, appearance, media.matches);
	const onStorage = (event: StorageEvent) => {
		if (event.key !== STORAGE_KEY) return;
		Object.assign(appearance, parse(event.newValue));
		reapply();
	};
	media.addEventListener('change', reapply);
	window.addEventListener('storage', onStorage);
	return () => {
		media.removeEventListener('change', reapply);
		window.removeEventListener('storage', onStorage);
	};
}

// Pour le JavaScript qui anime (défilement des rangées) : réglage de Hub ou de l'appareil.
export function motionReduced(): boolean {
	return appearance.reduceMotion || deviceMatches('(prefers-reduced-motion: reduce)');
}

export function transparencyReducedByDevice(): boolean {
	return deviceMatches('(prefers-reduced-transparency: reduce)');
}

export function motionReducedByDevice(): boolean {
	return deviceMatches('(prefers-reduced-motion: reduce)');
}
