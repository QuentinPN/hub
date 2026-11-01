-- Inventaire persistant : hôtes, services logiques et réglages de découverte.
-- L'état vivant (en ligne, latence, dernier contact) ne vit pas ici : il vient des agents.
-- Conteneurs, endpoints et widgets auront leurs propres tables avec la découverte réelle.

CREATE TABLE hosts (
	id         text PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
	name       text NOT NULL,
	os         text NOT NULL DEFAULT '',
	arch       text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE services (
	slug       text PRIMARY KEY CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
	name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
	monogram   text NOT NULL CHECK (length(monogram) BETWEEN 1 AND 3),
	color      text NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
	category   text NOT NULL,
	host_id    text NOT NULL REFERENCES hosts (id),
	favorite   boolean NOT NULL DEFAULT false,
	position   integer NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX services_host_id_idx ON services (host_id);

-- Une seule ligne, garantie par la clé booléenne toujours vraie.
CREATE TABLE discovery_settings (
	id                    boolean PRIMARY KEY DEFAULT true CHECK (id),
	follow_events         boolean NOT NULL DEFAULT true,
	rescan_minutes        integer NOT NULL DEFAULT 5 CHECK (rescan_minutes BETWEEN 1 AND 1440),
	on_removal            text NOT NULL DEFAULT 'absent_then_archive'
		CHECK (on_removal IN ('absent_then_archive', 'absent_only', 'archive_now')),
	archive_after_days    integer NOT NULL DEFAULT 7 CHECK (archive_after_days BETWEEN 1 AND 365),
	confirm_below_percent integer NOT NULL DEFAULT 80 CHECK (confirm_below_percent BETWEEN 50 AND 100),
	updated_at            timestamptz NOT NULL DEFAULT now()
);

INSERT INTO discovery_settings DEFAULT VALUES;
