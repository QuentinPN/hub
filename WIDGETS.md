# Système de widgets

## Contrat
Widget manifest : id, version, nom, service compatible, permissions requises, options, dimensions min/max, TTL, schéma des données, état de chargement, erreurs, mode dégradé. Les connecteurs tournent exclusivement côté serveur; le navigateur reçoit un DTO filtré.

## Widgets prioritaires
- Emby : sessions actives, lectures en cours, nouveaux médias, statistiques disponibles selon API/permissions.
- Sonarr : calendrier des épisodes, prochains épisodes, file d’attente, activité, statut des séries.
- Radarr : sorties à venir, téléchargements, bibliothèque, activité.
- Prowlarr : état des indexeurs et statistiques autorisées.
- qBittorrent : téléchargements actifs, vitesse, file, stockage (sans divulguer les noms sensibles en mode partagé).
- Request (remplace Seerr) : demandes en attente, à approuver, récentes, signalements ouverts, statistiques ; approuver, refuser, chercher et demander un titre depuis Hub.
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

## Request
- Clé : Réglages › API › Créer une clé, avec le compte propriétaire (une clé a les droits de qui la crée ; clés à droits limités pas encore disponibles). En-tête `X-Api-Key`, jamais `X-Request`.
- Widgets : `requests` (compteurs), `pending` (à approuver, page 1), `recent` (dernières demandes, page 1, plus récentes d'abord), `issues` (signalements ouverts), `stats`. Relecture toutes les 60 s par défaut.
- Actions (session + même origine) : `POST /api/v1/services/{slug}/requests/{id}/approve`, `…/decline` `{ "reason" }`, `POST /api/v1/services/{slug}/requests` `{ "mediaType", "tmdbId" }`, `GET /api/v1/services/{slug}/search?q=`. Un refus 403 d'Request devient 403 « La clé Request n'a pas le droit de faire cette action. », une clé refusée (401) devient 502, les autres 4xx gardent leur message, 5xx et réseau donnent 502. Chaque écriture réussie demande une relecture immédiate (ignorée en « Économe » et juste après l'enregistrement de la connexion, avant le premier tour du moteur : la relecture suivante rattrape).
- Webhook (signal de changement) : dans Hub, générer le secret du webhook de la connexion (`POST /api/v1/services/{slug}/connection/webhook-secret` ; adresse et secret montrés une seule fois, régénérer remplace l'ancien ; la connexion n'indique ensuite que `webhook.configured`). L'adresse part de `HUB_ORIGIN`, sinon de celle de la requête. Dans Request, Réglages › Webhook sortant : coller l'adresse (`https://<Hub>/api/v1/hooks/{slug}`) et, dans l'en-tête `Authorization`, le secret tel quel (sans « Bearer »), puis cocher les évènements voulus et « Tester ». Le corps n'est qu'un signal : Hub relit la connexion (5 s au plus souvent) et garde la relecture toutes les 60 s pour ce qu'il manquerait. En « Économe », le signal est ignoré.
