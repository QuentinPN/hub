# Hub — HomeLab Dashboard

Dashboard auto-hébergé pour les services Docker du mini-PC Debian, du QNAP et du Raspberry Pi. Le cadrage est dans [PRODUCT.md](PRODUCT.md), [ARCHITECTURE.md](ARCHITECTURE.md), [DESIGN.md](DESIGN.md) et [DECISIONS.md](DECISIONS.md).

**État : jalon 3 en cours (shell UI, navigation, responsive, accessibilité).** L'inventaire et les réglages sont enregistrés dans PostgreSQL ; l'accès demande un compte local (mot de passe, sessions, passkeys). L'état des services (en ligne, latence, données des widgets) est encore **fictif** (`server/internal/fixture`). Aucun hôte n'est contacté et rien n'accède à Docker.

Pages : accueil (`/`), catalogue « Tous les services » (`/services`, `Ctrl+K` depuis n'importe quelle page), fiche service (`/services/{slug}`), réglages en lecture seule (`/settings`).

API : `GET /api/v1/home`, `/api/v1/services`, `/api/v1/services/{slug}`, `/api/v1/settings`, `GET /healthz`, `PUT /api/v1/settings/discovery`, et les routes de compte sous `/api/v1/auth/` (état, setup, connexion par mot de passe ou passkey, sessions, passkeys).

## Structure

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

Le cookie de session est `__Host-` et `Secure` : le navigateur ne l'accepte qu'en HTTPS, ou sur `http://localhost` (contexte sûr). Un accès en HTTP par l'IP du poste (`http://192.0.2.x:8080`) ne permet donc pas de se connecter ; en production, l'accès passe par Caddy en HTTPS.

Passkeys (Windows Hello, téléphone, clé de sécurité) : une passkey est liée à un nom de domaine servi en HTTPS, jamais à une IP. En production, définir `HUB_ORIGIN` sur l'adresse du dashboard (`HUB_ORIGIN=https://home.example.com`). Sans elle, les passkeys ne marchent que sur `http://localhost:5173` et `http://localhost:8080`. Changer de domaine rend les passkeys existantes inutilisables ; le mot de passe reste valable. On les ajoute dans Réglages → Sécurité et accès, en confirmant avec le mot de passe.

UI avec rechargement à chaud (port 5173 ; Vite redirige `/api` vers 8080) :

```bash
cd web && npm install && npm run dev
```

Binaire complet avec l'UI embarquée (Git Bash sous Windows). Pour le Pi, préfixer `GOOS=linux GOARCH=arm64` :

```bash
bash scripts/build.sh
```

## Vérifier

```bash
cd web && npm run check && npm run lint && npm test
```

Tests Playwright de l'UI (navigation, `Ctrl+K`, focus, axe, débordement et mouvement réduit, en 1440 et 390 px). L'API y est simulée à partir de réponses enregistrées (`web/e2e/fixtures`) : ni serveur Go ni base. La première fois, installer Chromium (`npx playwright install chromium`), ou utiliser le Chrome du poste avec `PLAYWRIGHT_CHANNEL=chrome` :

```bash
cd web && npm run test:e2e
```

Les tests de base de données utilisent `HUB_TEST_DATABASE_URL` (chaque test crée puis supprime son propre schéma) et sont ignorés sans elle :

```bash
cd server && gofmt -l . && go vet ./... && HUB_TEST_DATABASE_URL="postgres://hub:hub-dev-only@127.0.0.1:55432/hub?sslmode=disable" go test ./...
```

La CI GitHub Actions (`.github/workflows/ci.yml`) lance ces mêmes étapes, plus golangci-lint et les builds Linux amd64 et arm64.
