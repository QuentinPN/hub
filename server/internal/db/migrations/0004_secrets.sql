-- Secrets des connecteurs (clés d'API, jetons), chiffrés par le serveur avant d'arriver ici.
-- La clé vit dans un fichier hors de la base (HUB_SECRET_KEY_FILE) : un dump seul ne
-- révèle rien.
CREATE TABLE secrets (
	-- Par exemple « emby:api_key ».
	name       text PRIMARY KEY CHECK (name ~ '^[a-z0-9][a-z0-9._:-]{0,127}$'),
	-- Clé du fichier qui a scellé la valeur ; une rotation rechiffre les autres.
	key_id     text NOT NULL CHECK (key_id ~ '^[A-Za-z0-9._-]{1,32}$'),
	-- AES-256-GCM : nonce de 12 octets, au moins 1 octet de données, étiquette de 16 octets ;
	-- 8 Kio au plus en clair.
	ciphertext bytea NOT NULL CHECK (length(ciphertext) BETWEEN 29 AND 8220),
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX secrets_key_id_idx ON secrets (key_id);
