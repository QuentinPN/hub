# Hub — HomeLab Dashboard

Dashboard auto-hébergé pour les services Docker du mini-PC Debian, du QNAP et du Raspberry Pi. Le cadrage est dans [PRODUCT.md](PRODUCT.md), [ARCHITECTURE.md](ARCHITECTURE.md), [DESIGN.md](DESIGN.md) et [DECISIONS.md](DECISIONS.md).

**État : jalon 1.** Les données sont **fictives** (`server/internal/fixture`). Aucun hôte n'est contacté et rien n'accède à Docker.

Pages : accueil (`/`), catalogue « Tous les services » (`/services`, `Ctrl+K` depuis n'importe quelle page), fiche service (`/services/{slug}`), réglages en lecture seule (`/settings`).

API en lecture seule : `GET /api/v1/home`, `/api/v1/services`, `/api/v1/services/{slug}`, `/api/v1/settings`, et `GET /healthz`.

## Structure

- `server/` : API Go et binaire unique. Il embarque l'UI compilée dans `server/internal/webui/dist`.
- `web/` : UI SvelteKit 3 / Svelte 5 en mode SPA, TypeScript strict. Les imports internes passent par `#lib/…`.
- `scripts/build.sh` : construit l'UI, l'embarque et produit `bin/hub-server`.

## Développer

Prérequis : Go 1.27 ou plus récent, Node 24 ou plus récent.

API (port 8080, écoute sur la boucle locale par défaut) :

```bash
cd server && go run ./cmd/hub-server
```

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

```bash
cd server && gofmt -l . && go vet ./... && go test ./...
```

La CI GitHub Actions (`.github/workflows/ci.yml`) lance ces mêmes étapes, plus golangci-lint et les builds Linux amd64 et arm64.
