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
[x] Shell UI, navigation, responsive, accessibilité (jalon 3, validé le 2026-11-02 après revue en 1440 et 390 px : shell, axe et clavier, tests Playwright, menu des Réglages en mobile, changement et réinitialisation du mot de passe, déconnexion d'une, des autres ou de toutes les sessions)

## Phase 2 — Discovery multi-host
[ ] Agent minimal sûr, connexion d’hôtes, snapshot initial (étape 1 faite sur le poste de dev le 2026-11-02 : AC, enrôlement, mTLS, agent derrière le proxy, instantané et événements ; écran Réglages → Hôtes et agents fait le même jour : enrôlement, annulation, révocation, conteneurs par hôte ; mini-PC, Pi et QNAP connectés le 2026-11-06)
[ ] Events + rescan, réconciliation, matching, inventaire persistant (réconciliation et catalogue de signatures déployés sur le mini-PC le 2026-11-06)
[x] Catalogue et overrides manuels, URLs vérifiées (2026-11-06, maquette validée ; déployé sur le mini-PC le même jour)

## Phase 3 — Dashboard
[ ] Widgets génériques, grille personnalisable, recherche, favoris (recherche et favoris faits ; rangées personnalisables déployées et confirmées sur le vrai le 2026-11-07)
[ ] Pages Médias, Téléchargements et Infra (déployées et confirmées sur le vrai le 2026-11-06, maquette validée) ; états loading/empty/error/offline, live updates (widgets : signal immédiat vers le navigateur et écoute d'Emby déployés et confirmés sur le vrai Emby le 2026-11-06)

## Phase 4 — Integrations
[x] Emby, Sonarr, Radarr, qBittorrent (socle et Emby faits le 2026-11-06, maquette validée, testés contre un faux Emby, déployés sur le mini-PC et confirmés sur le vrai Emby le même jour ; Sonarr et Radarr codés le même jour d'après leurs specs officielles, avec réaction immédiate par SignalR, déployés, connectés aux vrais et flux SignalR confirmé dans les journaux le même jour ; qBittorrent déployé et confirmé sur le vrai le même jour)
[ ] Autres connecteurs priorisés après inventaire réel (Seerr et Prowlarr déployés et connectés aux vrais le 2026-11-06 ; Beszel codé le 2026-11-07, à essayer sur le vrai)

## Phase 5 — Hardening & release
[ ] Tests E2E, sécurité, accessibilité, performance et panne d’agents
[ ] Déploiement Compose/Caddy, sauvegardes/restauration, observabilité (fichiers et procédure prêts et testés sur le poste de dev le 2026-11-06, `deploy/server/` ; serveur déployé sur le mini-PC le 2026-11-06 ; reste l'observabilité)
[ ] Documentation d’exploitation et procédure de rollback

À chaque phase : critères d’acceptation, démonstration, tests et accord avant modifications risquées.
