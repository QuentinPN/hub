# Agent Hub : binaire statique seul, sans shell ni paquet. Se construit depuis la racine du dépôt :
#   docker build -f deploy/agent/Dockerfile .   (scripts/build-agent.sh, et la CI à chaque push sur main)
# Le binaire se compile pour la cible sur la machine de build : pas d'émulation pour l'arm64 du Pi.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /hub-agent ./cmd/hub-agent

FROM scratch
COPY --from=build /hub-agent /hub-agent
# Répertoire d'état (certificat et clé) qui appartient à l'utilisateur non-root : un volume
# nommé vide reprend ce propriétaire à son premier montage.
COPY --chown=65534:65534 deploy/agent/state/ /var/lib/hub-agent/
USER 65534:65534
ENTRYPOINT ["/hub-agent"]
