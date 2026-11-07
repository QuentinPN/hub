-- Historique des services : ce qui leur est arrivé, et leur disponibilité heure par heure.
-- Gardé history_days jours (Réglages > Général), effacé ensuite par le serveur.

-- Un événement : panne ou retour, action Docker sur un de ses conteneurs, nouvelle version,
-- modification faite dans Hub, alerte envoyée. detail ne garde que des noms, des nombres et
-- des versions : jamais une adresse, une clé ni un libellé de conteneur hors de Hub.
CREATE TABLE service_events (
	id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	service_slug text NOT NULL REFERENCES services (slug) ON DELETE CASCADE,
	at           timestamptz NOT NULL,
	kind         text NOT NULL CHECK (kind IN ('outage', 'container', 'version', 'edit', 'alert')),
	action       text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 40),
	detail       jsonb NOT NULL DEFAULT '{}',
	user_id      bigint REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX service_events_service_idx ON service_events (service_slug, at DESC, id DESC);
CREATE INDEX service_events_at_idx ON service_events (at);

-- Minutes relevées par heure : en ligne, en panne, inconnues (hôte injoignable). La latence
-- est la somme des mesures de l'adresse prises en ligne, et leur nombre.
CREATE TABLE service_checks (
	service_slug    text NOT NULL REFERENCES services (slug) ON DELETE CASCADE,
	hour            timestamptz NOT NULL,
	up_minutes      integer NOT NULL DEFAULT 0 CHECK (up_minutes BETWEEN 0 AND 60),
	down_minutes    integer NOT NULL DEFAULT 0 CHECK (down_minutes BETWEEN 0 AND 60),
	unknown_minutes integer NOT NULL DEFAULT 0 CHECK (unknown_minutes BETWEEN 0 AND 60),
	latency_sum_ms  bigint NOT NULL DEFAULT 0 CHECK (latency_sum_ms >= 0),
	latency_count   integer NOT NULL DEFAULT 0 CHECK (latency_count BETWEEN 0 AND 60),
	-- Dernière minute comptée : un second relevé dans la même minute ne compte pas deux fois.
	last_minute     timestamptz NOT NULL,
	PRIMARY KEY (service_slug, hour)
);

CREATE INDEX service_checks_hour_idx ON service_checks (hour);

-- Dernier état connu, pour voir commencer et finir une panne à travers un redémarrage.
-- « Inconnu » ne le change pas : un hôte injoignable n'ouvre ni ne ferme rien.
CREATE TABLE service_health (
	service_slug text PRIMARY KEY REFERENCES services (slug) ON DELETE CASCADE,
	down         boolean NOT NULL,
	since        timestamptz NOT NULL,
	-- L'événement de panne en cours, pour lui donner sa durée au retour.
	event_id     bigint REFERENCES service_events (id) ON DELETE SET NULL
);

-- Début de l'historique : avant, Hub n'a rien gardé.
CREATE TABLE history_meta (
	id         boolean PRIMARY KEY DEFAULT true CHECK (id),
	started_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO history_meta DEFAULT VALUES;

ALTER TABLE general_settings
	ADD COLUMN history_days integer NOT NULL DEFAULT 90 CHECK (history_days IN (30, 90, 180, 365));
