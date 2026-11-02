# Étude de sécurité : l'agent Docker (phase 2)

Statut : **option 1.b validée le 2026-11-02** (« I approve 1.b »). Points 2 à 4 : valeurs par défaut gardées (« Keep defaults »). Rien n'est codé, installé ni modifié sur les hôtes. Rédigée le 2026-11-02.

Ce que l'UI annonce déjà : lecture seule (conteneurs, labels Compose, événements), sans démarrer ni arrêter, sans lire les variables d'environnement, connexion au serveur en TLS mutuel. L'étude cherche comment tenir cette promesse même si un composant est compromis, pas seulement si le code se comporte bien.

## 0. Faits vérifiés et inconnues

Vérifié dans les sources (2026-11-02) :

- **API Docker** (spec OpenAPI v1.47, `docs.docker.com/reference/api/engine/version/v1.47.yaml`). `GET /containers/json` renvoie `Id, Names, Image, ImageID, Command, Created, Ports, SizeRw, SizeRootFs, Labels, State, Status, HostConfig, NetworkSettings, Mounts` : **pas de variables d'environnement**. `GET /containers/{id}/json` (inspect) renvoie `Config.Env` en clair. `GET /events` accepte `since`, `until` et `filters` ; les attributs d'un événement dépendent du type d'objet (l'exemple montre des labels, `image`, `name`).
- **Accès au démon** (docs Docker, « Protect the Docker daemon socket ») : quiconque a les clés TLS peut « give any instructions to your Docker daemon », donc root sur l'hôte. Le modèle d'autorisation natif est « all or nothing » ; seul un plugin d'autorisation, déclaré dans la config de `dockerd`, affine les droits.
- **Hawser** (README `Finsys/hawser`, MIT) : en mode standard il « proxies all Docker API endpoints » (création, exec, arrêt…), écoute sur 2376, jeton `X-Hawser-Token`, TLS optionnel. En mode edge, il ouvre un WebSocket sortant vers Dockhand ; protocole non documenté. Aucune option de lecture seule ni d'allowlist. Images amd64, arm64, armv7.
- **Beszel** (beszel.dev, MIT) : agent avec le socket monté en `:ro`, connexion SSH (le hub se connecte à l'agent, port 45876) ou WebSocket (l'agent se connecte au hub avec un jeton). Collecte CPU, mémoire, réseau et état par conteneur. L'API REST (PocketBase) ne documente que la collection `systems`, et sa structure « may change in minor releases ». Rien sur les labels ou Compose.
- **Proxys de socket** :
  - `wollomatic/socket-proxy` (Go, image `scratch`, MIT) : allowlist par méthode HTTP en regex sur le chemin, tout ce qui n'est pas listé est refusé ; peut écouter sur un **socket Unix** au lieu d'un port TCP ; allowlist d'IP clientes ; exemples en non-root, `read_only`, `cap_drop: ALL`. Images multi-arch signées (Cosign). Architectures non listées dans le README.
  - `Tecnativa/docker-socket-proxy` (HAProxy, Apache-2.0) : droits par section (`CONTAINERS=1`…), `POST` refusé par défaut. `CONTAINERS=1` ouvre tout le préfixe `/containers`, donc a priori l'inspect (variables d'environnement), `logs`, `archive` (fichiers du conteneur) et `export`. Le README demande `--privileged`.
  - `linuxserver/docker-socket-proxy` (HAProxy, GPL-3.0) : même modèle, avec des interrupteurs séparés pour `logs`, `archive`, `export`… ; `ALLOW_START/STOP/RESTARTS` fonctionnent **même avec `POST=0`**. amd64 et arm64, pas armhf.

À confirmer avant l'implémentation (commandes en lecture seule, à lancer par toi) :

| Hôte | Question | Commande |
|---|---|---|
| Pi | ~~OS 64 bits ? version de Docker~~ | Relevé le 2026-11-02 : `aarch64`, Docker 29.9.2 (API 1.56), socket `root:docker` 660, gid 985 → `DOCKER_GID=985` |
| QNAP | ~~Chemin du socket, version de Docker~~ | Relevé le 2026-11-02 : `/var/run/docker.sock`, `0:0` 660 → `DOCKER_GID=0` ; Docker 27.1.2-qnap4 (API 1.46). L'agent appelle l'API sans préfixe de version : il suit celle du démon |
| QNAP | ~~Où vit le compose de Hawser~~ | Relevé le 2026-11-02 : `/share/docker/hawser` (les piles de Dockhand sont sous `/data/compose/<n>`). Survie à une mise à jour du firmware non vérifiée |
| mini-PC | Pare-feu | Relevé le 2026-11-02 : ufw actif (iptables-nft), port 8443 libre. Règles : 22/tcp depuis 192.0.2.0/24 et sur tailscale0, 445/tcp, 3702 et 5355 (tcp/udp) depuis le LAN ; rien d'autre. Serveur hors Docker : ajouter `ufw allow from 192.0.2.22 to any port 8443 proto tcp` et de même pour 192.0.2.10. Si le serveur tourne dans Docker avec un port publié, Docker contourne ufw : lier le port à l'IP LAN et filtrer dans `DOCKER-USER` |
| Pi, QNAP | Le mini-PC est-il joignable sur 8443 ? | `nc -zv 192.0.2.14 8443`, une fois le serveur déployé |
| — | ~~Architectures publiées de `wollomatic/socket-proxy`~~ | Vérifié le 2026-11-02 : 1.13.2 existe en amd64, arm64 et arm/v7 (digest épinglé dans `deploy/agent/compose.yml`) |

La documentation QNAP n'a pas donné le chemin du socket ni la persistance d'une config du démon ; je ne le suppose pas.

## 1. Lire Docker avec le moins de droits

Le point commun : **qui tient le socket est root sur l'hôte**. Ce qu'on peut choisir, c'est quel composant le tient, ce qu'il a le droit d'en faire, et s'il est joignable par le réseau.

### 1.a Socket monté en `:ro` dans l'agent

`:ro` empêche de modifier le fichier du socket, pas de s'y connecter : toute l'API reste disponible (créer un conteneur privilégié, `exec`, lire les variables d'environnement). La restriction ne tient que dans le code de l'agent.

- Risque : une faille dans l'agent (ou dans une dépendance, le SDK Docker compris) donne root sur l'hôte. C'est le modèle de Beszel et de Hawser.
- Pi / QNAP : aucune contrainte particulière.
- Verdict : acceptable seulement en dernier recours, et jamais avec un agent qui écoute sur le réseau.

### 1.b Proxy filtrant devant le socket

Un petit conteneur tient le socket et ne laisse passer qu'une liste fermée de requêtes ; l'agent ne parle qu'au proxy.

- Avec `wollomatic/socket-proxy`, l'allowlist peut être exacte :
  `GET` sur `/(v1\.[0-9]+/)?(_ping|version|containers/json|events)` et rien d'autre (pas d'inspect, de logs, d'archive, d'export, aucun `POST`).
  Il sort sur un socket Unix dans un volume partagé avec l'agent : le proxy peut tourner **sans aucun réseau** (`network_mode: none`), en non-root (groupe docker), système de fichiers en lecture seule, `cap_drop: ALL`, `no-new-privileges`.
- Avec Tecnativa ou linuxserver, la granularité par section ouvre l'inspect (donc les variables d'environnement) dès qu'on autorise les conteneurs. À écarter pour notre besoin.
- Risque résiduel : le proxy lui-même (code court, sans dépendance, mais tiers) ; une erreur de regex (le README prévient : grouper les alternatives). Les données autorisées contiennent quand même des choses sensibles, voir 1.f.
- Pi / QNAP : image Go statique, vérifier la présence d'arm64. Sur le QNAP, chemin du socket à confirmer. Un conteneur de plus par hôte (quelques Mio de mémoire).
- Verdict : **meilleure séparation pour un coût faible**.

### 1.c API Docker distante en TLS (port 2376)

Le serveur Hub parlerait directement aux démons, sans agent.

- Il faut modifier la config du démon sur les trois hôtes (`daemon.json` ou override systemd) et le redémarrer : interdit sans accord, et ça arrête les 38 conteneurs le temps du redémarrage.
- Un certificat client vaut root sur l'hôte : la clé stockée sur le mini-PC donnerait la main sur les trois machines. Le seul moyen de restreindre est un plugin d'autorisation, encore une modification du démon.
- Le démon écoute sur le réseau : nouvelle surface d'attaque sur chaque hôte.
- QNAP : persistance d'une config du démon de Container Station après mise à jour non garantie (non documentée).
- Verdict : **à écarter**.

### 1.d Réutiliser Hawser

- Hub devrait détenir un jeton Hawser, qui donne l'API complète (création, exec, arrêt) du Pi et du QNAP : contraire à la promesse de lecture seule. Une fuite de la base Hub deviendrait une prise de contrôle des deux hôtes.
- Le mode edge n'a pas de protocole documenté ; le mode standard dépend des choix de Dockhand (version, jetons, ports).
- Rien sur le mini-PC lui-même : il faudrait y installer Hawser.
- Verdict : **à écarter**. Hawser tient déjà le socket du Pi et du QNAP ; ce risque existe et ne dépend pas d'Hub.

### 1.e Réutiliser Beszel

- Le hub n'expose pas de collection de conteneurs documentée, et Beszel ne collecte ni labels ni projet Compose : impossible de reconstruire services, endpoints et applications.
- Son API peut changer à une version mineure.
- Verdict : **pas pour la découverte**. Intéressant plus tard comme source de métriques (CPU, mémoire) pour un widget, en lecture via le hub, sans toucher aux agents.

### 1.f Ce que même la lecture autorisée expose

Deux champs de `containers/json` et des événements peuvent contenir des secrets :

- **`Labels`** : par exemple `traefik.http.middlewares.*.basicauth.users` (utilisateur et hash), des jetons dans des labels de Homepage ou de Watchtower.
- **`Command`** : des mots de passe passés en argument (`--password=…`).

L'agent doit donc filtrer **avant l'envoi** : `Command` jamais transmis ; labels transmis par allowlist de préfixes (`com.docker.compose.*`, `org.opencontainers.image.*`, `hub.*`, `homepage.*` hors champs de type clé) et, pour les autres, la clé sans la valeur. Même filtre sur les attributs des événements. Le serveur applique le même filtre en défense (un agent ancien ou modifié ne doit pas remplir la base de secrets).

## 2. Connexion agent → serveur

### Qui ouvre la connexion

**L'agent appelle le serveur**, jamais l'inverse.

- Aucun port d'écoute sur le Pi ni sur le QNAP : un agent n'a pas de surface réseau entrante.
- Le serveur n'a **aucun canal de commande** vers les agents : un serveur compromis ne peut rien demander d'autre qu'une rescan. L'agent pousse un instantané, puis les événements ; le serveur peut seulement répondre « renvoie un instantané ».
- Le serveur écoute sur un port dédié aux agents (par exemple 8443), séparé de l'UI, lié à l'IP LAN du mini-PC (et à l'IP Tailscale si un hôte distant arrive un jour). Caddy n'est pas dans ce chemin : terminer le TLS chez lui casserait l'authentification mutuelle de bout en bout.
- L'agent du mini-PC suit le même chemin (même protocole, même mTLS), sans exception.
- Transport : HTTPS/2, une requête courte par envoi (instantané, changements, battement toutes les 15 s). Un événement Docker ne sert qu'à relire le conteneur concerné par `containers/json` ; ses attributs (qui portent tous les labels) ne sont jamais lus.

### Identité mutuelle

- **Autorité de certification privée, propre à Hub**, créée au premier démarrage : clé ECDSA P-256 dans un fichier hors base (`HUB_AGENT_CA_KEY_FILE`, `0600`, jamais écrasé), comme la clé des secrets. Elle ne sert qu'aux agents ; elle n'a rien à voir avec le certificat public de `home.example.com`.
- Certificat serveur signé par cette AC, avec les IP et noms qu'utilisent les agents (`HUB_AGENT_NAMES`). Réémis à chaque démarrage, sa clé reste en mémoire et ne touche jamais le disque.
- Certificat agent : identité `hub://agent/<id de l'hôte>` dans le SAN (URI). Le serveur relie chaque certificat à une ligne `agents` (empreinte, dates, état) et vérifie à chaque connexion qu'il n'est pas révoqué. Pas de liste de révocation à publier : le serveur est le seul vérificateur.
- Les données reçues sont rattachées à l'hôte du certificat, jamais à un identifiant envoyé dans le message : un agent compromis ne peut parler qu'au nom de son propre hôte. Taille des messages bornée, JSON strict.
- TLS 1.3 seulement.

### Enrôlement d'un nouvel hôte

1. Réglages → Hôtes et agents → « Ajouter un hôte » (ou `hub-server -enroll-host <hôte>` sur le serveur) : le serveur crée l'hôte et un **code d'enrôlement** à usage unique (32 octets aléatoires, 15 minutes, seul son SHA-256 est gardé), et affiche le compose à copier avec l'empreinte de l'AC.
2. Au premier démarrage, l'agent **génère sa clé privée sur l'hôte** (elle n'en sort jamais), se connecte en vérifiant l'empreinte de l'AC (pas de confiance au premier contact), envoie une demande de certificat avec le code.
3. Le serveur signe, consomme le code et l'UI affiche l'hôte « connecté » avec le nom et la version de Docker remontés par l'agent, pour que tu confirmes que c'est bien la machine attendue.
4. Le code passe par un fichier (secret Compose) plutôt qu'une variable d'environnement visible dans `docker inspect` ; une fois utilisé, il ne sert plus à rien.

Journal d'audit : `agent_enrollment_created`, `agent_enrolled`, `agent_enrollment_failed`, `agent_revoked`.

### Rotation et révocation

- Certificat agent : 30 jours, renouvelé automatiquement aux deux tiers, sur la connexion mTLS existante, **avec une nouvelle clé**. Validité antidatée de 5 minutes (le Pi n'a pas d'horloge sauvegardée et démarre parfois avant la synchro NTP).
- Agent éteint plus de 30 jours : certificat expiré, l'hôte passe « à réenrôler » avec un nouveau code. Rien d'autre ne permet de revenir.
- Certificat serveur : 1 an, réémis à chaque démarrage ; les agents font confiance à l'AC, pas au certificat.
- AC : 10 ans. Rotation manuelle documentée (les agents acceptent l'ancienne et la nouvelle AC pendant la transition), à prévoir mais pas à coder en premier.
- Révocation depuis l'UI ou en ligne de commande : l'agent est refusé (`403`) dès sa requête suivante, au plus 15 s plus tard (requêtes courtes, pas de connexion longue à couper). Le code en attente de l'hôte est annulé avec.

### Comportement en cas de panne

| Panne | Agent | Serveur / UI |
|---|---|---|
| Serveur arrêté ou injoignable | Reconnexion avec attente croissante et aléa (1 s → 60 s). Pas de file d'événements sur disque | Au retour : instantané complet, puis événements. Les manques sont rattrapés par l'instantané (réconciliation) |
| Agent ou hôte arrêté | — | Hôte « injoignable » après 3 battements manqués (45 s). Dernier inventaire gardé et marqué périmé ; aucun conteneur supprimé |
| Démon Docker ou proxy arrêté | Signale « Docker indisponible », réessaie | État distinct de « agent injoignable » |
| Horloge fausse (Pi au démarrage) | Échec TLS, réessaie | Rien de faux n'est enregistré |
| Certificat expiré | Arrête d'essayer en boucle rapide, logue clairement | Hôte « à réenrôler » |
| Agent compromis | Ne peut lire que les 4 routes du proxy et parler au nom de son hôte | Données filtrées à nouveau, bornées |
| Serveur compromis | Aucun ordre possible vers les agents | — |

Conformément à ARCHITECTURE.md, l'absence de données n'est jamais présentée comme une panne du service.

## 3. Comparatif

| | 1.a socket `:ro` | 1.b proxy filtrant + agent | 1.c API TLS distante | 1.d Hawser | 1.e Beszel |
|---|---|---|---|---|---|
| Droits effectifs si compromis | root sur l'hôte | 4 lectures | root sur les 3 hôtes | API complète Pi + QNAP | — |
| Lit les variables d'environnement | possible | non (inspect bloqué) | oui | oui | non |
| Labels et Compose | oui | oui | oui | oui | non |
| Port ouvert sur l'hôte | non | non | oui (2376) | déjà ouvert | déjà ouvert |
| Modifie le démon Docker | non | non | oui, avec redémarrage | non | non |
| À installer par hôte | 1 conteneur | 2 conteneurs | rien, mais config démon | rien | rien |
| Pi arm64 | oui | oui (arm64 du proxy à vérifier) | oui | oui | oui |
| QNAP | chemin du socket à confirmer | idem | persistance incertaine | déjà en place | déjà en place |
| Dépendance à un tiers | non | proxy (petit, MIT) | non | Dockhand (BSL) | API instable |

## 4. Recommandation

**Option 1.b : agent Hub en Go derrière `wollomatic/socket-proxy`, connexion sortante en mTLS vers le serveur, AC privée, enrôlement par code à usage unique.**

Pourquoi :

1. C'est la seule option où la promesse de l'UI (lecture seule, pas de variables d'environnement) est tenue **par un composant qui n'est pas l'agent** : même avec une faille dans l'agent, le proxy refuse tout sauf 4 lectures.
2. Aucun port ouvert sur les hôtes et aucune modification de Docker : seuls deux conteneurs sont ajoutés, qu'on supprime sans laisser de trace (règle 2 de CLAUDE.md).
3. Le serveur n'a aucun pouvoir sur les hôtes : la base ou le serveur compromis n'ouvre pas le Pi ni le QNAP, à la différence de Hawser ou de l'API distante.
4. Coût modeste : le proxy est une image `scratch` sans dépendance, l'agent un binaire Go statique (amd64 et arm64 sont déjà dans la CI).

Défense en profondeur côté agent : client HTTP qui refuse lui-même toute autre méthode ou route, filtre des labels et de `Command`, image `scratch`, utilisateur non-root, système de fichiers en lecture seule, `cap_drop: ALL`, aucun port publié. Côté proxy : `network_mode: none`, sortie sur socket Unix en `0600`, partagé par un volume avec le seul agent.

Ce que la recommandation ne règle pas : le proxy reste un composant tiers qui tient le socket (version épinglée par digest, mise à jour suivie) ; Hawser et Beszel gardent leurs propres accès complets, hors du périmètre d'Hub.

### Première étape proposée après validation

Sur ce poste uniquement (Docker Desktop), sans toucher aux hôtes : le serveur avec l'AC, l'enrôlement et le port agents ; l'agent derrière le proxy ; tests qui vérifient que l'inspect, les logs, l'archive et tout `POST` sont refusés par le proxy, que les variables d'environnement, `Command` et les labels sensibles n'arrivent jamais en base. Le déploiement sur le mini-PC, puis le Pi et le QNAP, viendra ensuite, hôte par hôte, avec ton accord à chaque fois.

### Points à trancher

1. L'option 1.b et ce découpage agent + proxy.
2. Le port dédié aux agents (8443 proposé) et son ouverture dans le pare-feu du mini-PC si besoin.
3. La durée des certificats agents (30 jours proposé).
4. L'allowlist de labels (préfixes ci-dessus).
