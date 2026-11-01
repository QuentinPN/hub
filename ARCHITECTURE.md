# Architecture cible (proposition à confirmer)

## Stack
Validé le 2026-11-01 (voir DECISIONS.md).
- **Serveur API et agent : Go.** Bibliothèque standard `net/http`, SSE pour les événements unidirectionnels (WebSocket seulement si nécessaire), `pgx` pour PostgreSQL (requêtes SQL écrites à la main ; `sqlc` si elles se multiplient), migrations SQL numérotées embarquées dans le binaire et appliquées au démarrage (`server/internal/db`), SDK Docker officiel côté agent. Binaires statiques multi-arch (amd64 pour le mini-PC et le QNAP, arm64 pour le Pi). Lint : `golangci-lint`, format : `gofmt`.
- **UI : SvelteKit + TypeScript strict**, en `adapter-static` (SPA). Le build est embarqué dans le binaire Go (`embed`). Pas de SSR : le dashboard est privé et derrière authentification. Les tokens CSS viennent de la direction A. Biome si compatible avec Svelte, sinon ESLint + Prettier.
- **PostgreSQL** pour la configuration et l'inventaire.
- Le navigateur ne parle qu'à l'API Go, jamais aux agents ni aux applications intégrées.

## Topologie
Navigateur -> reverse proxy HTTPS (Caddy) -> serveur Dashboard/API -> PostgreSQL.
Serveur Dashboard <-> agents légers authentifiés par hôte (mini-PC Debian, QNAP, Raspberry Pi) -> Docker Engine API locale.
Serveur Dashboard -> connecteurs HTTP serveur-vers-serveur -> Emby, Sonarr, Radarr, Prowlarr, qBittorrent, etc.

## Découverte
1. Enregistrement explicite d’un hôte, bootstrap sûr d’agent et test de connexion.
2. Snapshot initial de tous les conteneurs, métadonnées Compose et labels, état et ports.
3. Écoute d’événements Docker + scans de réconciliation périodiques et au reconnect.
4. Normalisation en hôte / application / instance / conteneur / endpoint.
5. Déduplication stable par identifiants d’hôte et ressources, avec gestion des recréations de conteneurs.
6. Matching par catalogue de signatures; score de confiance; override manuel.
7. Inférence prudente des URLs : labels explicites ou saisie utilisateur; jamais supposer qu’un port publié est une URL utilisable.
8. Importer les services sans endpoint comme cartes « configuration requise ».
9. Marquer données périmées et hôte injoignable, sans assimiler absence de télémétrie à une panne.

## Stockage
Tables conceptuelles : hosts, agents, applications, containers, endpoints, catalog_signatures, widget_instances, widget_credentials, dashboard_layouts, users, audit_events, discovery_events, health_checks. Contraintes uniques, migrations et historique de changement.

## Résilience et performances
Pollings regroupés par API/hôte, cache TTL, backoff, circuit breakers, limites de concurrence, timeout, retries bornés; budgets réseau; aucun appel client direct aux API privées; logs structurés, métriques et traces optionnelles. Lazy loading des widgets, animations GPU-friendly.

## Déploiement
Docker Compose, images multi-architecture si agents sur Pi, healthchecks, volumes persistants, backups PostgreSQL et procédure de restauration. Caddy en frontal, accès admin LAN/VPN par défaut. Prévoir configuration par variables d’environnement et secrets Docker/fichiers.
