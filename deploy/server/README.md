# Déployer le serveur Hub

Le quotidien (mettre à jour, revenir en arrière, journaux, pannes connues) est dans [docs/EXPLOITATION.md](../../docs/EXPLOITATION.md).

À lire en entier avant de lancer quoi que ce soit. Rien ici n'arrête un service existant, et le Caddy des sites publics n'est pas touché : Hub a son propre Caddy, sur le port 8444. Seules les règles de pare-feu modifient ce qui est en place, et elles se relisent d'abord.

Déployé sur le mini-PC le 2026-11-06 avec cette procédure : vrai certificat, pare-feu contrôlé depuis ce poste (bloqué) et le Pi (ouvert), compte, passkey et agents du mini-PC, du Pi et du QNAP connectés. Sauvegarde et restauration testées sur Docker Desktop seulement. **Pas encore testé** : le redémarrage de l'hôte, avec le Wi-Fi qui doit avoir son IP pour que les ports se publient.

## Ce qui sera ajouté sur l'hôte

- Trois conteneurs, `hub-db-1`, `hub-hub-1` et `hub-caddy-1`, et quatre volumes, `hub_pgdata`, `hub_keys`, `hub_caddy-data` et `hub_caddy-config`.
- Deux réseaux, `hub_db` (sans sortie) et `hub-edge`.
- Si les sauvegardes sont activées, le dossier `HUB_BACKUP_HOST_DIR`, monté dans `hub` sur `/sauvegardes` par `compose.backup.yml`. Docker ne le crée pas.
- Les ports 8443 (agents) et 8444 (UI), sur l'IP de `HUB_BIND` seulement. Ne les redirige pas sur ta box : Hub reste en LAN.

Accès depuis le VPN : pas encore en place. Le port n'écoute pas sur l'IP Tailscale, et `home.example.com` pointe vers l'IP LAN.

## 1. Construire et transférer l'image

Sur le poste de dev (Go, Node et Docker) :

```bash
scripts/build-server.sh
```

```bash
docker save hub-server:local hub-caddy:local | ssh hub@192.0.2.14 docker load
```

Copier ensuite `compose.yml`, `compose.backup.yml`, `Caddyfile` et `.env.example` de `deploy/server/` sur l'hôte, puis y faire les étapes suivantes.

## 2. Configurer

Partir de `.env.example` :

```bash
cp .env.example .env && chmod 600 .env
```

Remplir `HUB_DB_PASSWORD` (`openssl rand -hex 32`) et `CLOUDFLARE_API_TOKEN`. Le token se crée dans Cloudflare, limité à Zone → DNS → Edit sur `example.com`. Relire ensuite le résultat sans l'afficher ailleurs, il contient les secrets :

```bash
docker compose config
```

Créer aussi dans Cloudflare l'enregistrement `A` de `home.example.com` vers `192.0.2.14`, **sans le proxy** (nuage gris). Depuis un appareil du LAN, vérifier que le nom se résout : certaines box bloquent les réponses DNS vers une IP privée.

```bash
nslookup home.example.com
```

## 3. Filtrer le port 8443

Le 8444 reste ouvert à tout le LAN, il mène à la page de connexion. Les ports publiés par Docker contournent ufw : le filtrage se fait dans `DOCKER-USER`. Ces règles ne laissent passer que le Pi et le QNAP, sur l'interface LAN. L'agent du mini-PC passe par `hub-edge` et n'est pas concerné.

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
sudo sh -c 'iptables-restore -n --test < /etc/ufw/after.rules' && sudo ufw reload && sudo iptables -L DOCKER-USER -n -v
```

## 4. Démarrer

```bash
docker compose up -d
```

Le code de création du compte (une seule fois) est dans les journaux :

```bash
docker compose logs hub | grep setup_code
```

## 5. Vérifier le certificat

Caddy l'obtient seul au démarrage, en une à deux minutes. En cas d'échec, l'erreur Cloudflare ou ACME est ici :

```bash
docker compose logs caddy
```

## 6. Contrôler

Depuis le Pi et le QNAP, le port doit répondre :

```bash
nc -zv 192.0.2.14 8443
```

Depuis un autre appareil du LAN, la même commande doit échouer. Sur le QNAP, `nc` n'est pas netcat : y utiliser `curl -sk -m5 -o /dev/null -w '%{http_code}
' https://192.0.2.14:8443/`, qui doit afficher `404`. `https://home.example.com:8444` doit afficher la création du compte, avec un certificat reconnu.

## 7. Enrôler l'agent du mini-PC

Enrôler depuis Réglages → Hôtes et agents, mais mettre `HUB_SERVER_URL=https://hub:8443` dans le `.env` de l'agent : il joint le serveur par `hub-edge`. Image à transférer comme celle du serveur, après `scripts/build-agent.sh`.

```bash
docker compose -f compose.yml -f compose.same-host.yml up -d
```

Le Pi (`GOARCH=arm64 scripts/build-agent.sh`) et le QNAP utilisent `compose.yml` seul, avec l'URL montrée par l'écran d'enrôlement.

## Sauvegarder et restaurer

Hub se sauvegarde lui-même, chiffré, dans un partage du QNAP monté sur l'hôte (`HUB_BACKUP_HOST_DIR` et `compose.backup.yml`), ou ne se sauvegarde pas du tout avec `HUB_BACKUP=off`. Le montage, la vérification et la restauration sont dans [docs/EXPLOITATION.md](../../docs/EXPLOITATION.md).

## Mettre à jour

Sauvegarder d'abord (Réglages → Sauvegarde) : les migrations s'appliquent au démarrage et ne se défont pas. Transférer la nouvelle image, puis `docker compose up -d`. Pour revenir en arrière : remettre le tag de l'ancienne image sur `hub-server:local` et restaurer la sauvegarde.

## Retirer Hub

Dans cet ordre, pour rendre l'hôte dans son état d'origine :

1. Arrêter l'agent de cet hôte, sinon `hub-edge` reste en place : `docker compose -f compose.yml -f compose.same-host.yml down`.
2. Arrêter le serveur, sans toucher aux données : `docker compose down`. Ajouter `-v` efface **définitivement** la base et les clés.
3. Retirer le bloc de `/etc/ufw/after.rules`, puis `sudo ufw reload`. Si `DOCKER-USER` garde les règles, le vider avec `sudo iptables -F DOCKER-USER`.

4. Supprimer l'enregistrement DNS et le token dans Cloudflare.

Sur les autres hôtes, révoquer puis arrêter leur agent.
