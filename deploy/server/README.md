# Déployer le serveur Hub

À lire en entier avant de lancer quoi que ce soit. Rien ici n'arrête un service existant ; seules la route Caddy et les règles de pare-feu touchent à ce qui est déjà en place, et elles se relisent d'abord.

Ce qui a été testé sur Docker Desktop : la pile, un Caddy de test sur un réseau externe, l'enrôlement d'un agent par `hub-edge`, le redémarrage, la sauvegarde et la restauration. **Pas testé** : les règles de pare-feu, le certificat de `home.example.com`.

## Ce qui sera ajouté sur l'hôte

- Deux conteneurs, `hub-db-1` et `hub-hub-1`, et deux volumes, `hub_pgdata` et `hub_keys`.
- Deux réseaux, `hub_db` (sans sortie) et `hub-edge`. Le serveur rejoint aussi le réseau du Caddy existant.
- Le port 8443, sur l'IP de `HUB_AGENT_BIND` seulement. L'UI (8080) n'est pas publiée.

## 1. Construire et transférer l'image

Sur le poste de dev (Go, Node et Docker) :

```bash
scripts/build-server.sh
```

```bash
docker save hub-server:local | ssh hub@192.0.2.14 docker load
```

Copier ensuite `deploy/server/` sur l'hôte, sans le binaire `hub-server`, puis y faire les étapes suivantes.

## 2. Configurer

Partir de `.env.example` :

```bash
cp .env.example .env && chmod 600 .env
```

Remplir `HUB_DB_PASSWORD` (`openssl rand -hex 32`) et `HUB_CADDY_NETWORK`, puis relire le résultat :

```bash
docker compose config
```

## 3. Filtrer le port 8443

Les ports publiés par Docker contournent ufw : le filtrage se fait dans `DOCKER-USER`. Ces règles ne laissent passer que le Pi et le QNAP, sur l'interface LAN. L'agent du mini-PC passe par `hub-edge` et n'est pas concerné.

Trouver l'interface LAN (`dev …` dans la réponse) :

```bash
ip route get 192.0.2.1
```

Ajouter à la fin de `/etc/ufw/after.rules`, en remplaçant `LAN` par cette interface :

```
*filter
:DOCKER-USER - [0:0]
-A DOCKER-USER -i LAN -p tcp -m conntrack --ctorigdstport 8443 --ctdir ORIGINAL -s 192.0.2.22 -j RETURN
-A DOCKER-USER -i LAN -p tcp -m conntrack --ctorigdstport 8443 --ctdir ORIGINAL -s 192.0.2.10 -j RETURN
-A DOCKER-USER -i LAN -p tcp -m conntrack --ctorigdstport 8443 --ctdir ORIGINAL -j DROP
COMMIT
```

Sauvegarder le fichier avant de le modifier, puis recharger et contrôler :

```bash
sudo ufw reload && sudo iptables -L DOCKER-USER -n -v
```

## 4. Démarrer

```bash
docker compose up -d
```

Le code de création du compte (une seule fois) est dans les journaux :

```bash
docker compose logs hub | grep setup_code
```

## 5. Ajouter la route Caddy

Sauvegarder le Caddyfile, ajouter le bloc de `Caddyfile.example` avec le `tls` des autres sites, puis valider avant de recharger. Remplacer `caddy` par le nom du conteneur :

```bash
docker exec caddy caddy validate --config /etc/caddy/Caddyfile
```

```bash
docker exec caddy caddy reload --config /etc/caddy/Caddyfile
```

## 6. Contrôler

Depuis le Pi et le QNAP, le port doit répondre :

```bash
nc -zv 192.0.2.14 8443
```

Depuis un autre appareil du LAN, la même commande doit échouer. `https://home.example.com` doit afficher la création du compte.

## 7. Enrôler l'agent du mini-PC

Enrôler depuis Réglages → Hôtes et agents, mais mettre `HUB_SERVER_URL=https://hub:8443` dans le `.env` de l'agent : il joint le serveur par `hub-edge`. Image à transférer comme celle du serveur, après `scripts/build-agent.sh`.

```bash
docker compose -f compose.yml -f compose.same-host.yml up -d
```

Le Pi (`GOARCH=arm64 scripts/build-agent.sh`) et le QNAP utilisent `compose.yml` seul, avec l'URL montrée par l'écran d'enrôlement.

## Sauvegarder

Les deux sont nécessaires : sans `keys.tgz`, les secrets sont perdus et chaque agent doit être réenrôlé. `keys.tgz` contient les clés en clair : le garder à part de la base et hors de portée.

```bash
docker compose exec -T db pg_dump -U hub -d hub > hub.sql
```

```bash
docker run --rm -v hub_keys:/k:ro -v "$PWD":/b --entrypoint tar postgres:18.6 czf /b/keys.tgz -C /k .
```

## Restaurer

Sur des volumes vides, depuis ce dossier :

```bash
docker compose create
```

```bash
docker run --rm -v hub_keys:/k -v "$PWD":/b:ro --entrypoint tar postgres:18.6 xzpf /b/keys.tgz -C /k
```

```bash
docker compose start db
```

Attendre quelques secondes que la base soit prête, puis :

```bash
docker compose exec -T db psql -q -v ON_ERROR_STOP=1 -U hub -d hub < hub.sql
```

```bash
docker compose up -d
```

## Mettre à jour

Sauvegarder d'abord : les migrations s'appliquent au démarrage et ne se défont pas. Transférer la nouvelle image, puis `docker compose up -d`. Pour revenir en arrière : remettre le tag de l'ancienne image sur `hub-server:local` et restaurer la sauvegarde.

## Retirer Hub

Dans cet ordre, pour rendre l'hôte dans son état d'origine :

1. Retirer le bloc de Caddy, puis recharger Caddy.
2. Arrêter l'agent de cet hôte, sinon `hub-edge` reste en place : `docker compose -f compose.yml -f compose.same-host.yml down`.
3. Arrêter le serveur, sans toucher aux données : `docker compose down`. Ajouter `-v` efface **définitivement** la base et les clés.
4. Retirer le bloc de `/etc/ufw/after.rules`, puis `sudo ufw reload`. Si `DOCKER-USER` garde les règles, le vider avec `sudo iptables -F DOCKER-USER`.

Sur les autres hôtes, révoquer puis arrêter leur agent.
