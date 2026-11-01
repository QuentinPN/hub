# Système de widgets

## Contrat
Widget manifest : id, version, nom, service compatible, permissions requises, options, dimensions min/max, TTL, schéma des données, état de chargement, erreurs, mode dégradé. Les connecteurs tournent exclusivement côté serveur; le navigateur reçoit un DTO filtré.

## Widgets prioritaires
- Emby : sessions actives, lectures en cours, nouveaux médias, statistiques disponibles selon API/permissions.
- Sonarr : calendrier des épisodes, prochains épisodes, file d’attente, activité, statut des séries.
- Radarr : sorties à venir, téléchargements, bibliothèque, activité.
- Prowlarr : état des indexeurs et statistiques autorisées.
- qBittorrent : téléchargements actifs, vitesse, file, stockage (sans divulguer les noms sensibles en mode partagé).
- Jellyseerr/Overseerr : demandes et états.
- Bazarr : sous-titres manquants.
- Portainer / Dockhand : liens et métriques uniquement si API confirmée.
- Pi-hole : requêtes, blocages, état DNS.
- Grafana / Prometheus : liens vers panels ou requêtes serveur validées.
- Uptime Kuma : état des moniteurs si API utilisable.
- PostgreSQL : métriques non sensibles et statut via collecteur dédié.
- Docker host : CPU, RAM, stockage, charge, conteneurs, uptime via agent avec permissions adaptées.
- Générique : santé, lien, description, disponibilité, métriques standard.

## Configuration
Détection du type d’application, configuration guidée des endpoints, authentification serveur (clé API ou token), test de connexion, rotation des secrets, widget opt-in. Ne jamais inventer un endpoint ou stocker un secret en clair.

## Isolation
Timeout et rate limit par connecteur; une panne Emby ne bloque ni Sonarr ni l’accueil. Tests contractuels avec mocks; versions d’API prises en compte.
