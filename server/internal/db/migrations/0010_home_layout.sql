-- Disposition de l'accueil : ordre des rangées, rangées masquées, sources écartées.
-- Une seule ligne : Hub n'a qu'un compte. Sans ligne, l'ordre par défaut.
CREATE TABLE home_layout (
	id         boolean PRIMARY KEY DEFAULT true CHECK (id),
	layout     jsonb NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
);
