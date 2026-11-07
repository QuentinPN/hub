-- Sauvegardes : réglages (une seule ligne, sans ligne les valeurs par défaut) et historique.
-- La phrase de passe n'est gardée nulle part : seulement la clé publique, et la clé privée
-- scellée par elle, que chaque sauvegarde emporte.
CREATE TABLE backup_settings (
	id         boolean PRIMARY KEY DEFAULT true CHECK (id),
	enabled    boolean NOT NULL DEFAULT true,
	frequency  text NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly')),
	retention  text NOT NULL DEFAULT '7d4w' CHECK (retention IN ('7d4w', '30d', 'last3')),
	recipient  text,
	sealed_key bytea,
	key_set_at timestamptz,
	updated_at timestamptz NOT NULL DEFAULT now(),
	CHECK ((recipient IS NULL) = (sealed_key IS NULL))
);

CREATE TABLE backup_runs (
	id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	started_at  timestamptz NOT NULL DEFAULT now(),
	finished_at timestamptz,
	manual      boolean NOT NULL,
	ok          boolean NOT NULL DEFAULT false,
	file        text NOT NULL DEFAULT '',
	size_bytes  bigint NOT NULL DEFAULT 0,
	error       text NOT NULL DEFAULT ''
);

CREATE INDEX backup_runs_started_at_idx ON backup_runs (started_at);
