-- Passkeys (WebAuthn) du compte local, en plus du mot de passe de secours.

-- Identifiant WebAuthn du compte (user handle) : aléatoire, jamais le nom ni l'id SQL.
-- Le défaut volatile donne une valeur distincte à chaque ligne existante.
ALTER TABLE users
	ADD COLUMN webauthn_handle bytea NOT NULL UNIQUE DEFAULT uuid_send(gen_random_uuid());

CREATE TABLE passkeys (
	id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id       bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	-- Identifiant choisi par l'authentificateur, cherché à chaque connexion.
	credential_id bytea NOT NULL UNIQUE CHECK (length(credential_id) BETWEEN 16 AND 1023),
	name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
	-- Enregistrement complet de go-webauthn (clé publique, compteur, drapeaux). Rien de secret.
	credential    jsonb NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now(),
	last_used_at  timestamptz
);

CREATE INDEX passkeys_user_id_idx ON passkeys (user_id);
