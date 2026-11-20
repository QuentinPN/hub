# Serveur Hub : binaire statique avec l'UI embarquée. Se construit depuis la racine du dépôt :
#   docker build -f deploy/server/Dockerfile .   (scripts/build-server.sh, et la CI à chaque push sur main)

# 1. L'interface (SvelteKit, statique). Node 24 comme la CI.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:116b800c3754b440ee0aa3ed2f20f5bf3d99a554bfd9adeea09ea8017f6e85f5 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Le binaire, avec l'UI embarquée (server/internal/webui). Go suit server/go.mod.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/web/build/ internal/webui/dist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /hub-server ./cmd/hub-server

# distroless plutôt que scratch : les autorités de certification pour télécharger les icônes.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=build /hub-server /hub-server
# Répertoire des clés, qui appartient à l'utilisateur non-root : un volume nommé vide reprend
# ce propriétaire à son premier montage.
COPY --chown=65532:65532 deploy/server/state/ /var/lib/hub/
ENTRYPOINT ["/hub-server"]
