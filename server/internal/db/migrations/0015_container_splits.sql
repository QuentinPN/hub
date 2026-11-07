-- Un service Compose d'un projet mis à part : ses conteneurs forment leur propre service,
-- de clé « projet/service ». Survit aux recréations de conteneurs comme aux rescans.
CREATE TABLE container_splits (
	host_id    text NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
	project    text NOT NULL CHECK (project <> '' AND position('/' IN project) = 0),
	service    text NOT NULL CHECK (service <> ''),
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (host_id, project, service)
);
