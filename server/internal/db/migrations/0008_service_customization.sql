-- Adresse vérifiée, masquage choisi et projets reliés à un service.
-- url_cert_sha256 : le certificat accepté par l'utilisateur, seul à passer sans chaîne reconnue.
ALTER TABLE services
	ADD COLUMN url             text,
	ADD COLUMN url_cert_sha256 text NOT NULL DEFAULT '',
	ADD COLUMN url_result      text NOT NULL DEFAULT ''
		CHECK (url_result IN ('', 'ok', 'auth', 'cert', 'unreachable', 'error')),
	ADD COLUMN url_status      integer NOT NULL DEFAULT 0,
	ADD COLUMN url_latency_ms  integer NOT NULL DEFAULT 0,
	ADD COLUMN url_checked_at  timestamptz,
	ADD COLUMN hidden          boolean NOT NULL DEFAULT false,
	-- Réaffiché à la main alors que la découverte l'aurait masqué (agent, base seule, Hub).
	ADD COLUMN force_visible   boolean NOT NULL DEFAULT false;

-- Un projet du même hôte compté comme appoint du service. Il n'appartient qu'à un service.
CREATE TABLE service_links (
	service_slug  text NOT NULL REFERENCES services (slug) ON DELETE CASCADE,
	host_id       text NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
	discovery_key text NOT NULL,
	PRIMARY KEY (host_id, discovery_key)
);

CREATE INDEX service_links_slug_idx ON service_links (service_slug);
