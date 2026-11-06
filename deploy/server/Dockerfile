# Serveur Hub : binaire statique avec l'UI embarquée. Construit par scripts/build-server.sh.
# distroless plutôt que scratch : les autorités de certification pour télécharger les icônes.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY hub-server /hub-server
# Répertoire des clés, qui appartient à l'utilisateur non-root : un volume nommé vide reprend
# ce propriétaire à son premier montage.
COPY --chown=65532:65532 state/ /var/lib/hub/
ENTRYPOINT ["/hub-server"]
