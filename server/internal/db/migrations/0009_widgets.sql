-- Connexion d'un service à son API, un widget à la fois opt-in. La clé vit dans « secrets »,
-- chiffrée ; ici seulement ses derniers caractères, pour la reconnaître.
CREATE TABLE widget_connections (
	service_slug    text PRIMARY KEY REFERENCES services (slug) ON DELETE CASCADE,
	kind            text NOT NULL,
	-- L'adresse testée avec cette clé : si celle du service change, la clé n'y part pas.
	url             text NOT NULL,
	key_hint        text NOT NULL DEFAULT '',
	options         jsonb NOT NULL DEFAULT '{}',
	enabled         text[] NOT NULL,
	refresh_seconds integer NOT NULL CHECK (refresh_seconds BETWEEN 30 AND 3600),
	state           text NOT NULL CHECK (state IN ('ok', 'auth', 'cert', 'unreachable', 'unexpected')),
	-- Échecs de suite : ils espacent les essais.
	failures        integer NOT NULL DEFAULT 0,
	-- Dernière lecture réussie : déjà filtrée, sans secret.
	data            jsonb NOT NULL DEFAULT '{}',
	data_at         timestamptz,
	checked_at      timestamptz NOT NULL DEFAULT now(),
	failing_since   timestamptz,
	-- Change à chaque enregistrement : le résultat d'une lecture lancée avant est ignoré.
	generation      bigint NOT NULL DEFAULT 1,
	created_at      timestamptz NOT NULL DEFAULT now(),
	updated_at      timestamptz NOT NULL DEFAULT now()
);
