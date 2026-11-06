# Caddy dédié à Hub, avec le module DNS Cloudflare : le certificat s'obtient sans ouvrir de port
# sur Internet. Construit par scripts/build-server.sh.
FROM caddy:2.11.9-builder-alpine@sha256:2252cf8c0a1474735a9ce57204aa71be28601298204e3ad727681c1eb159be98 AS build
# Sans setcap : le port est haut, et le compose interdit de gagner une capacité au lancement.
RUN XCADDY_SETCAP=0 xcaddy build v2.11.9 --with github.com/caddy-dns/cloudflare@v0.2.4 --output /caddy

FROM caddy:2.11.9-alpine@sha256:78bd35025c7ba85961dd149ae670315129954e77abe2383ac47c89106b35ade9
COPY --from=build /caddy /usr/bin/caddy
RUN chown -R 65532:65532 /data /config
USER 65532:65532
