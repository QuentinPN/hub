# Roadmap avec portes de validation

## Phase 0 — Discovery & Design (STOP obligatoire)
[x] Examiner les skills et la documentation
[ ] Confirmer hypothèses sans modifier l’infrastructure
[x] Produire 3 propositions design desktop/mobile et comparatif
[x] Montrer exemple de home, catalogue, détail service, widgets et paramètres (maquettes validées, pages codées sur données fictives)
[x] Recueillir validation explicite de l’utilisateur (2026-11-01 : A + fiche service, backend Go)

## Phase 1 — Foundation
[x] Initialisation stack, lint, format, tests, CI, design system validé (jalon 1, 2026-11-01 : données fictives)
[x] Schéma PostgreSQL, migrations, authentification et secrets (jalon 2, 2026-11-01 : PostgreSQL et migrations, compte + mot de passe + sessions, passkeys, chiffrement des secrets ; la saisie des secrets dans l'UI arrivera avec les connecteurs)
[ ] Shell UI, navigation, responsive, accessibilité (jalon 3 en cours : shell, corrections axe et clavier, tests Playwright faits ; reste la validation)

## Phase 2 — Discovery multi-host
[ ] Agent minimal sûr, connexion d’hôtes, snapshot initial
[ ] Events + rescan, réconciliation, matching, inventaire persistant
[ ] Catalogue et overrides manuels, URLs vérifiées

## Phase 3 — Dashboard
[ ] Widgets génériques, grille personnalisable, recherche, favoris
[ ] États loading/empty/error/offline, live updates

## Phase 4 — Integrations
[ ] Emby, Sonarr, Radarr, qBittorrent
[ ] Autres connecteurs priorisés après inventaire réel

## Phase 5 — Hardening & release
[ ] Tests E2E, sécurité, accessibilité, performance et panne d’agents
[ ] Déploiement Compose/Caddy, sauvegardes/restauration, observabilité
[ ] Documentation d’exploitation et procédure de rollback

À chaque phase : critères d’acceptation, démonstration, tests et accord avant modifications risquées.
