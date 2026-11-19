-- Ce que la clé d'une connexion peut faire, relevé à l'enregistrement (la portée d'une clé ne
-- change pas) : faux pour une clé en lecture seule. Nul : inconnu, la clé est supposée pouvoir agir.
ALTER TABLE widget_connections ADD COLUMN can_act boolean;
-- Une connexion enregistrée avant cette colonne gardait la valeur dans ses données lues.
UPDATE widget_connections SET can_act = (data -> 'requests' ->> 'canAct')::boolean
WHERE kind = 'request' AND jsonb_typeof(data -> 'requests' -> 'canAct') = 'boolean';
