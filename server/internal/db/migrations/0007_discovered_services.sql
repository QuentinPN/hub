-- Réconciliation : un service découvert est lié à son projet Compose sur son hôte, qui
-- survit aux recréations de conteneurs. NULL : service de l'inventaire d'exemple.
-- L'état n'est pas stocké : il se calcule à la lecture depuis les conteneurs.
ALTER TABLE services
	ADD COLUMN discovery_key text,
	ADD COLUMN recognized_by text NOT NULL DEFAULT '',
	ADD COLUMN absent_since  timestamptz,
	ADD COLUMN archived_at   timestamptz;

CREATE UNIQUE INDEX services_discovery_key_idx ON services (host_id, discovery_key)
	WHERE discovery_key IS NOT NULL;
