// Signaux du serveur : le nom d'un service qui a du nouveau, à relire par l'API. Un seul flux
// pour toute la page, ouvert tant que quelqu'un écoute.

type Listener = (slug: string) => void;

const RETRY_MS = 30_000;

const listeners = new Set<Listener>();
let source: EventSource | null = null;
let retry: ReturnType<typeof setTimeout> | undefined;

function open() {
	source = new EventSource('/api/v1/events');
	source.addEventListener('widgets', (event) => {
		for (const listener of listeners) listener((event as MessageEvent<string>).data);
	});
	// Le navigateur se reconnecte seul après une coupure, mais renonce sur une réponse d'erreur,
	// par exemple pendant un redémarrage : la relecture régulière des pages prend alors le relais.
	source.addEventListener('error', () => {
		if (source?.readyState !== EventSource.CLOSED) return;
		source = null;
		retry = setTimeout(() => {
			retry = undefined;
			if (listeners.size > 0) open();
		}, RETRY_MS);
	});
}

/** Écoute les signaux ; la fonction rendue arrête l'écoute. */
export function onWidgetNews(listener: Listener): () => void {
	listeners.add(listener);
	if (!source && retry === undefined) open();
	return () => {
		listeners.delete(listener);
		if (listeners.size > 0) return;
		source?.close();
		source = null;
		clearTimeout(retry);
		retry = undefined;
	};
}
