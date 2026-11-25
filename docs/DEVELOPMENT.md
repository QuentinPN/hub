# Guide développeur de Hub

Mise en route, architecture, scripts, tests et exploitation de Hub, le dashboard auto-hébergé pour services Docker. La présentation du produit est dans le [README](../README.md) ([version française](../README.fr.md)).

Le cadrage est dans [PRODUCT.md](../PRODUCT.md), [ARCHITECTURE.md](../ARCHITECTURE.md), [DESIGN.md](../DESIGN.md) et [DECISIONS.md](../DECISIONS.md) ; l'avancement par phase est dans [ROADMAP.md](../ROADMAP.md). Le quotidien d'une installation (mise à jour, sauvegarde, pannes) est dans [EXPLOITATION.md](EXPLOITATION.md).

Pages : accueil (`/`), catalogue « Tous les services » (`/services`, `Ctrl+K` depuis n'importe quelle page), fiche service (`/services/{slug}`), réglages (`/settings`), et les pages Médias, Téléchargements et Infra.

API : sous `/api/v1/` (accueil, services, réglages, comptes et sessions, passkeys, agents, widgets), plus `GET /healthz` et `GET /readyz`. Le code des routes est dans `server/internal/api`.

## Structure

Les commandes de ce guide se lancent depuis la racine du dépôt, sauf mention contraire.

- `server/` : API Go et binaire unique. Il embarque l'UI compilée dans `server/internal/webui/dist`.
- `web/` : UI SvelteKit 3 / Svelte 5 en mode SPA, TypeScript strict. Les imports internes passent par `#lib/…`.
- `scripts/build.sh` : construit l'UI, l'embarque et produit `bin/hub-server`.

## Développer

Prérequis : Go 1.27 ou plus récent, Node 24 ou plus récent.

PostgreSQL de développement (Docker Desktop, sur ce poste uniquement) :

```bash
docker compose -f deploy/dev/compose.yml up -d --wait
```

API (port 8080, écoute sur la boucle locale par défaut). `HUB_DATABASE_URL` est obligatoire : le compte et les sessions vivent en base. `HUB_SECRET_KEY_FILE` aussi : c'est le fichier de la clé qui chiffre les secrets des connecteurs, créé au premier démarrage s'il n'existe pas (`dev-secrets.key` est ignoré par git). Les migrations s'appliquent au démarrage :

```bash
cd server && HUB_DATABASE_URL="postgres://hub:hub-dev-only@127.0.0.1:55432/hub?sslmode=disable" HUB_SECRET_KEY_FILE=dev-secrets.key go run ./cmd/hub-server
```

Clé des secrets : les clés d'API et jetons des connecteurs sont chiffrés en AES-256-GCM avant d'entrer en base, avec une clé qui reste dans ce fichier. En production, le placer hors du volume PostgreSQL (`chmod 600`) et **le sauvegarder à part** : sans lui, les secrets enregistrés sont perdus (il suffira de les ressaisir), mais une sauvegarde de la base seule ne les révèle pas. Rotation : `hub-server -new-secret-key` affiche une nouvelle ligne à ajouter à la fin du fichier ; au démarrage suivant, chaque secret est rechiffré avec cette dernière clé. Une fois les logs confirmés (`secrets re-sealed`), l'ancienne ligne peut être retirée.

Premier démarrage : tant qu'aucun compte n'existe, le serveur écrit dans ses logs un code à usage unique (`setup_code=XXXX-XXXX-XXXX-XXXX`, renouvelé à chaque démarrage). Ouvrir le dashboard, saisir ce code, puis choisir l'identifiant et le mot de passe (12 caractères minimum).

Mot de passe : il se change dans Réglages → Sécurité et accès, en confirmant avec l'actuel ; les autres navigateurs sont alors déconnectés. S'il est perdu, le changer sur la machine qui fait tourner Hub, avec les mêmes variables d'environnement (dans le conteneur, par exemple) : `hub-server -reset-password` demande le nouveau deux fois sans l'afficher, puis ferme toutes les sessions. Les passkeys restent valables. Aucune route HTTP ne permet cette réinitialisation : il faut déjà avoir la main sur le serveur.

```bash
docker exec -it hub hub-server -reset-password
```

Le cookie de session est `__Host-` et `Secure` : le navigateur ne l'accepte qu'en HTTPS, ou sur `http://localhost` (contexte sûr). Un accès en HTTP par l'IP du poste (`http://192.0.2.x:8080`) ne permet donc pas de se connecter ; en production, l'accès passe par Caddy en HTTPS.

Passkeys (Windows Hello, téléphone, clé de sécurité) : une passkey est liée à un nom de domaine servi en HTTPS, jamais à une IP. En production, définir `HUB_ORIGIN` sur l'adresse du dashboard (`HUB_ORIGIN=https://home.example.com:8444`, port compris). Sans elle, les passkeys ne marchent que sur `http://localhost:5173` et `http://localhost:8080`. Changer de domaine rend les passkeys existantes inutilisables ; le mot de passe reste valable. On les ajoute dans Réglages → Sécurité et accès, en confirmant avec le mot de passe.

UI avec rechargement à chaud (port 5173 ; Vite redirige `/api` vers 8080) :

```bash
cd web && npm install && npm run dev
```

Binaire complet avec l'UI embarquée (Git Bash sous Windows). Pour le Pi, préfixer `GOOS=linux GOARCH=arm64` :

```bash
bash scripts/build.sh
```

## Agent Docker

L'agent lit les conteneurs d'un hôte à travers un proxy qui n'autorise que quatre lectures, et les envoie au serveur en TLS mutuel. Il n'ouvre aucun port et ne lance aucune commande Docker ([AGENT-STUDY.md](../AGENT-STUDY.md)).

Côté serveur, `HUB_AGENT_CA_FILE` active le port agents (`HUB_AGENT_ADDR`, `127.0.0.1:8443` par défaut ; `HUB_AGENT_NAMES` liste les IP et noms que les agents appellent). Le fichier de l'AC est créé au premier démarrage : **le sauvegarder à part**, sans lui chaque agent doit être réenrôlé. Les agents appellent `https://<premier nom de HUB_AGENT_NAMES>:<port de HUB_AGENT_ADDR>`.

Enrôler un hôte, annuler son code ou révoquer son agent se fait dans Réglages → Hôtes et agents : le code (valable 15 minutes, une seule fois) n'y est montré qu'une fois, avec le `.env` à copier sur l'hôte. Les mêmes actions existent en ligne de commande sur le serveur :

```bash
hub-server -enroll-host rpi5 > enroll-code
```

```bash
hub-server -revoke-host rpi5
```

Sur l'hôte : l'image `git.example.com/quentinpn/hub-agent` (amd64 et arm64, publiée à chaque push sur `main`), placer le code dans `deploy/agent/enroll-code`, puis lancer `deploy/agent/compose.yml` avec `HUB_SERVER_URL`, `HUB_CA_PIN` (affiché par `-enroll-host`) et `DOCKER_GID` (`stat -c %g /var/run/docker.sock`), tous trois donnés par l'écran d'enrôlement. `scripts/check-agent-proxy.sh` vérifie ensuite que le proxy refuse tout le reste.

## Déployer

Le serveur et sa base se déploient avec Docker Compose : procédure, pare-feu, sauvegarde, restauration et retrait dans [deploy/server/README.md](../deploy/server/README.md).

## Hooks git (hk)

Les hooks sont gérés par [hk](https://hk.jdx.dev) (`hk.pkl`). Une seule fois par poste : installer hk 2.13.0 (Windows : `hk.exe` de la [release GitHub](https://github.com/jdx/hk/releases/tag/v2.13.0), sinon `cargo install hk --version 2.13.0 --locked`), puis lancer `hk install` dans le clone. Facultatif : `git config hk.stashUntracked false` évite environ 1 s au pre-commit quand des fichiers non suivis traînent.

- `pre-commit` : Prettier et ESLint (`web/`), `gofmt` (`server/`) sur les seuls fichiers indexés, avec correction et réindexation ; une erreur restante bloque le commit. Environ 0,3 s, 4 s dès qu'un fichier ESLint est indexé (le chargement de la config ESLint prend 3 s).
- `pre-push` : les contrôles de la CI sans service, uniquement pour le dossier modifié : `web/` (`check`, `lint`, `test`, `build`, environ 15 s) et `server/` (`gofmt`, `go vet`, `go test`, environ 25 s). Restent à la CI : e2e, `npm audit`, golangci-lint, `-race`.

## Vérifier

```bash
cd web && npm run check && npm run lint && npm test
```

Icônes des services : [selfh.st/icons](https://selfh.st/icons) (CC BY 4.0), servies par `GET /api/v1/icons/{ref}`. Un lot de départ est embarqué dans le binaire ; une icône absente est téléchargée une fois par le serveur depuis `cdn.jsdelivr.net`, vérifiée, puis gardée dans PostgreSQL. `HUB_ICONS_REMOTE=off` coupe ces téléchargements (lot embarqué seul). Le serveur a donc besoin d'un accès sortant HTTPS vers `cdn.jsdelivr.net`, et vers rien d'autre.

Tests Playwright de l'UI (navigation, `Ctrl+K`, focus, axe, débordement et mouvement réduit, en 1440 et 390 px). L'API y est simulée à partir de réponses enregistrées (`web/e2e/fixtures`) : ni serveur Go ni base. La première fois, installer Chromium (`npx playwright install chromium`), ou utiliser le Chrome du poste avec `PLAYWRIGHT_CHANNEL=chrome` :

```bash
cd web && npm run test:e2e
```

Le serveur démarre sans aucune donnée d'exemple : le catalogue reste vide jusqu'à ce que des agents remontent de vrais conteneurs. `HUB_DEMO_DATA=on` remplit une base vide avec l'inventaire d'exemple, pour le développement.

Les tests de base de données utilisent `HUB_TEST_DATABASE_URL` (chaque test crée puis supprime son propre schéma) et sont ignorés sans elle :

```bash
cd server && gofmt -l . && go vet ./... && HUB_TEST_DATABASE_URL="postgres://hub:hub-dev-only@127.0.0.1:55432/hub?sslmode=disable" go test ./...
```

La CI Gitea Actions (`.gitea/workflows/ci.yml`) lance ces mêmes étapes, plus golangci-lint et les builds Linux amd64 et arm64.
