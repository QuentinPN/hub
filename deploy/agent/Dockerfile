# Agent Hub : binaire statique seul, sans shell ni paquet. Construit par scripts/build-agent.sh.
FROM scratch
COPY hub-agent /hub-agent
# Répertoire d'état (certificat et clé) qui appartient à l'utilisateur non-root : un volume
# nommé vide reprend ce propriétaire à son premier montage.
COPY --chown=65534:65534 state/ /var/lib/hub-agent/
USER 65534:65534
ENTRYPOINT ["/hub-agent"]
