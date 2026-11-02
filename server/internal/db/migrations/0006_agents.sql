-- Agents Docker (AGENT-STUDY.md) : enrôlement, certificats et conteneurs observés.

-- Ce que l'agent a remonté de son hôte ; vide tant qu'aucun agent ne s'est connecté.
ALTER TABLE hosts
	ADD COLUMN agent_version  text NOT NULL DEFAULT '',
	ADD COLUMN docker_version text NOT NULL DEFAULT '',
	ADD COLUMN docker_ok      boolean NOT NULL DEFAULT false,
	ADD COLUMN last_seen_at   timestamptz;

-- Codes d'enrôlement à usage unique. Seul le SHA-256 du code est gardé.
CREATE TABLE agent_enrollments (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	host_id    text NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
	code_hash  bytea NOT NULL UNIQUE CHECK (length(code_hash) = 32),
	created_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz NOT NULL,
	used_at    timestamptz
);

CREATE INDEX agent_enrollments_host_id_idx ON agent_enrollments (host_id);

-- Certificats émis par l'AC des agents. Le serveur est le seul à les vérifier : une
-- révocation est une date ici, sans liste à publier.
CREATE TABLE agent_certs (
	serial      text PRIMARY KEY CHECK (serial ~ '^[0-9a-f]{1,40}$'),
	host_id     text NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
	fingerprint bytea NOT NULL UNIQUE CHECK (length(fingerprint) = 32),
	not_before  timestamptz NOT NULL,
	not_after   timestamptz NOT NULL,
	created_at  timestamptz NOT NULL DEFAULT now(),
	revoked_at  timestamptz
);

CREATE INDEX agent_certs_host_id_idx ON agent_certs (host_id);

-- Conteneurs tels que l'agent les voit. Jamais de variable d'environnement ni de ligne
-- de commande ; les labels hors liste autorisée n'ont que leur clé. Un conteneur disparu
-- reste, marqué absent (removed_at).
CREATE TABLE containers (
	host_id         text NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
	id              text NOT NULL CHECK (id ~ '^[0-9a-f]{64}$'),
	name            text NOT NULL,
	image           text NOT NULL,
	state           text NOT NULL,
	status          text NOT NULL,
	created         timestamptz,
	compose_project text NOT NULL DEFAULT '',
	compose_service text NOT NULL DEFAULT '',
	labels          jsonb NOT NULL DEFAULT '{}',
	redacted_labels text[] NOT NULL DEFAULT '{}',
	ports           jsonb NOT NULL DEFAULT '[]',
	first_seen_at   timestamptz NOT NULL DEFAULT now(),
	seen_at         timestamptz NOT NULL DEFAULT now(),
	removed_at      timestamptz,
	PRIMARY KEY (host_id, id)
);

CREATE INDEX containers_compose_idx ON containers (host_id, compose_project);
