-- Notifications : canaux, alertes choisies, incidents en cours, résumé des heures calmes et
-- appareils déjà vus. Le sujet ntfy et le mot de passe SMTP sont dans `secrets`, jamais ici.

-- Une seule ligne ; sans elle, aucun canal et les alertes par défaut.
CREATE TABLE notify_settings (
	id            boolean PRIMARY KEY DEFAULT true CHECK (id),
	ntfy_url      text NOT NULL DEFAULT '',
	-- Début du sujet, pour le reconnaître sans le montrer en entier.
	ntfy_hint     text NOT NULL DEFAULT '',
	smtp_host     text NOT NULL DEFAULT '',
	smtp_port     integer NOT NULL DEFAULT 587 CHECK (smtp_port BETWEEN 1 AND 65535),
	smtp_username text NOT NULL DEFAULT '',
	smtp_from     text NOT NULL DEFAULT '',
	smtp_to       text NOT NULL DEFAULT '',
	-- Alertes changées par rapport à leur valeur par défaut : {"disk": true, …}.
	alerts        jsonb NOT NULL DEFAULT '{}',
	quiet_hours   boolean NOT NULL DEFAULT true,
	-- Dernière session examinée pour « nouvel appareil » ; NULL tant que rien n'a été appris.
	last_session  bigint,
	updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Dernier envoi de chaque canal, test ou alerte.
CREATE TABLE notify_channel_state (
	channel text PRIMARY KEY CHECK (channel IN ('ntfy', 'smtp')),
	at      timestamptz NOT NULL,
	ok      boolean NOT NULL,
	test    boolean NOT NULL,
	error   text NOT NULL DEFAULT ''
);

-- Ce qui est en panne. notified_at est posé une fois l'alerte partie : au retour, un message
-- dit que c'est revenu ; sans alerte partie, la ligne disparaît en silence.
CREATE TABLE notify_incidents (
	key         text PRIMARY KEY,
	alert       text NOT NULL,
	subject     text NOT NULL,
	detail      text NOT NULL DEFAULT '',
	since       timestamptz NOT NULL,
	notified_at timestamptz
);

-- Messages gardés pendant les heures calmes, envoyés ensemble le matin.
CREATE TABLE notify_digest (
	id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	at    timestamptz NOT NULL,
	title text NOT NULL,
	body  text NOT NULL DEFAULT ''
);

-- Appareils déjà vus : navigateur et système, d'après l'en-tête User-Agent des sessions.
CREATE TABLE notify_devices (
	label      text PRIMARY KEY,
	first_seen timestamptz NOT NULL
);
