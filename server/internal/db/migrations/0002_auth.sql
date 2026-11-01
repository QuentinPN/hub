-- Compte local unique, sessions côté serveur et journal d'audit (SECURITY.md).

CREATE TABLE users (
	id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	username      text NOT NULL UNIQUE CHECK (length(username) BETWEEN 1 AND 64),
	-- Chaîne Argon2id au format PHC ($argon2id$v=19$m=…,t=…,p=…$sel$hash).
	password_hash text NOT NULL CHECK (password_hash LIKE '$argon2id$%'),
	created_at    timestamptz NOT NULL DEFAULT now(),
	updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Un seul compte tant que le partage avec rôles n'existe pas.
CREATE UNIQUE INDEX users_single_account ON users ((true));

-- Le navigateur garde le jeton ; la base n'en garde que l'empreinte SHA-256.
CREATE TABLE sessions (
	id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	token_hash   bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
	user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	user_agent   text NOT NULL DEFAULT '',
	ip           text NOT NULL DEFAULT '',
	created_at   timestamptz NOT NULL DEFAULT now(),
	last_seen_at timestamptz NOT NULL DEFAULT now(),
	expires_at   timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE audit_events (
	id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	at      timestamptz NOT NULL DEFAULT now(),
	kind    text NOT NULL,
	user_id bigint REFERENCES users (id) ON DELETE SET NULL,
	ip      text NOT NULL DEFAULT '',
	detail  text NOT NULL DEFAULT ''
);

CREATE INDEX audit_events_at_idx ON audit_events (at);
