-- Icône d'un service : référence selfh.st (https://selfh.st/icons), servie par
-- GET /api/v1/icons/{ref}. NULL ou icône introuvable : le monogramme reste affiché.
ALTER TABLE services ADD COLUMN icon text CHECK (icon ~ '^[a-z0-9][a-z0-9-]{0,62}$');

-- Inventaire d'exemple déjà en base : chaque service reçoit l'icône de même nom, sauf
-- Pi-hole (« pi-hole » chez selfh.st) et Portainer (variante claire, lisible sur fond noir).
UPDATE services
SET icon = CASE slug WHEN 'pihole' THEN 'pi-hole' WHEN 'portainer' THEN 'portainer-light' ELSE slug END
WHERE icon IS NULL AND slug IN (
	'emby', 'sonarr', 'radarr', 'qbittorrent', 'jellyseerr', 'pihole', 'bazarr', 'prowlarr',
	'lidarr', 'readarr', 'tdarr', 'sabnzbd', 'flaresolverr', 'unpackerr', 'gluetun',
	'wireguard', 'unbound', 'caddy', 'tailscale', 'portainer', 'dockhand', 'postgresql',
	'redis', 'diun', 'vaultwarden', 'nextcloud', 'immich', 'syncthing', 'uptime-kuma',
	'grafana', 'prometheus', 'loki', 'cadvisor', 'gotify'
);

-- Icônes téléchargées depuis selfh.st, vérifiées avant d'arriver ici (internal/icons).
-- content_type et body NULL : selfh.st n'a pas cette icône (nouvel essai après 7 jours).
CREATE TABLE icon_cache (
	ref          text PRIMARY KEY CHECK (ref ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
	content_type text CHECK (content_type IN ('image/svg+xml', 'image/webp')),
	body         bytea CHECK (octet_length(body) <= 204800),
	fetched_at   timestamptz NOT NULL,
	CHECK ((content_type IS NULL) = (body IS NULL))
);
