-- Réglages « Général » : une seule ligne, sans ligne les valeurs par défaut.
-- time_zone vide : celui du serveur (TZ). Le nom est vérifié par le serveur, qui seul sait
-- quels fuseaux il connaît.
CREATE TABLE general_settings (
	id             boolean PRIMARY KEY DEFAULT true CHECK (id),
	display_name   text NOT NULL DEFAULT 'Hub' CHECK (char_length(display_name) BETWEEN 1 AND 40),
	start_page     text NOT NULL DEFAULT 'home' CHECK (start_page IN ('home', 'medias', 'downloads', 'infra')),
	time_zone      text NOT NULL DEFAULT '' CHECK (char_length(time_zone) <= 64),
	widget_refresh text NOT NULL DEFAULT 'normal' CHECK (widget_refresh IN ('normal', 'economy')),
	updated_at     timestamptz NOT NULL DEFAULT now()
);
