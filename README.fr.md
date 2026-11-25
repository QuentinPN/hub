<div align="center">

[English](README.md) · [Français](README.fr.md)

# Hub

**Une belle porte d'entrée unique pour tout ce que vous auto-hébergez.**

Hub repère les applications qui tournent sur vos serveurs maison, suit ce qui est en ligne ou à l'arrêt, et les réunit dans un tableau de bord rapide, façon télévision, aussi à l'aise sur téléphone que sur ordinateur.

![Page d'accueil de Hub : un tableau de bord sombre et cinématique avec les applications favorites, la santé des serveurs et les prochaines sorties](docs/screenshots/hub.webp)

</div>

## Points forts

- **Trouve vos applications tout seul.** Connectez une machine une fois : Hub liste toutes les applications Docker déjà en marche, puis remarque les nouvelles, celles qui s'arrêtent ou qui disparaissent. Aucun inventaire à tenir à la main.
- **Toutes vos machines d'un coup d'œil.** Voyez quelles applications sont en ligne et combien de conteneurs chaque hôte fait tourner. Branchez votre outil de supervision et la charge de chaque serveur (processeur, mémoire, disque) s'affiche sur la page d'accueil.
- **Des widgets vivants, pas de simples liens.** Les cartes de votre serveur multimédia, de vos gestionnaires de bibliothèque, de votre gestionnaire de demandes et de votre supervision montrent l'activité réelle ; un service en panne ne bloque jamais le reste de la page.
- **À votre main.** Favoris, rangées d'accueil réordonnables, catalogue de tous les services avec recherche (`Ctrl+K` partout), thème sombre ou clair, densité confortable ou compacte.
- **Des alertes qui respectent vos soirées.** Recevez un message sur votre téléphone (ntfy) ou par e-mail quand un service ou une machine tombe, puis quand elle revient, avec heures calmes et sans spam pour les micro-coupures.
- **Privé et sûr par conception.** Comptes locaux avec passkeys, clés d'API chiffrées au repos et jamais envoyées au navigateur, agents qui lisent Docker sans jamais lancer de commande.
- **Simple à faire tourner et à restaurer.** Un seul programme serveur avec l'interface intégrée, une base PostgreSQL, et des sauvegardes chiffrées en un seul fichier, planifiables et restaurables.

## Comment ça marche

1. **Un petit agent tourne sur chaque machine.** Il lit la liste des conteneurs à travers un proxy en lecture seule et la transmet à Hub par une connexion chiffrée et authentifiée des deux côtés. Il n'ouvre aucun port et ne peut rien démarrer ni arrêter.
2. **Le serveur Hub fait le lien.** Il garde l'inventaire dans PostgreSQL, interroge vos services côté serveur pour connaître leur état, et pousse les mises à jour au navigateur dès qu'elles arrivent.
3. **Votre navigateur ne parle qu'à Hub.** Il ne contacte jamais directement vos machines ni vos applications : aucun secret ne l'atteint.

```mermaid
flowchart LR
    B[Votre navigateur] --> H[Serveur Hub]
    H --> D[(PostgreSQL)]
    A1[Agent : machine 1] --> H
    A2[Agent : machine 2] --> H
    H --> S["API de vos applications"]
```

## Démarrage rapide

Essayez-le sur votre ordinateur avec des données d'exemple. Il faut Docker, Go 1.27+ et Node 24+, sous Linux, ou Windows avec Git Bash.

```bash
git clone https://github.com/QuentinPN/hub.git && cd hub
docker compose -f deploy/dev/compose.yml up -d --wait
bash scripts/build.sh
HUB_DATABASE_URL="postgres://hub:hub-dev-only@127.0.0.1:55432/hub?sslmode=disable" \
  HUB_SECRET_KEY_FILE=dev-secrets.key HUB_DEMO_DATA=on ./bin/hub-server
```

Ouvrez <http://localhost:8080>. Au premier démarrage, le serveur affiche un code d'installation à usage unique dans sa sortie (`setup_code=...`) : saisissez-le, puis choisissez votre identifiant et votre mot de passe.

L'installation durable sur un serveur maison (Docker Compose, HTTPS, sauvegardes) est décrite dans [deploy/server/README.md](deploy/server/README.md).

## Stack

![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Svelte](https://img.shields.io/badge/Svelte_5-FF3E00?style=for-the-badge&logo=svelte&logoColor=white)
![SvelteKit](https://img.shields.io/badge/SvelteKit-FF3E00?style=for-the-badge&logo=svelte&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=for-the-badge&logo=typescript&logoColor=white)
![Vite](https://img.shields.io/badge/Vite-646CFF?style=for-the-badge&logo=vite&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white)
![Caddy](https://img.shields.io/badge/Caddy-1F88C0?style=for-the-badge&logo=caddy&logoColor=white)
![Playwright](https://img.shields.io/badge/Playwright-2EAD33?style=for-the-badge&logo=playwright&logoColor=white)

## Documentation

Installation détaillée, architecture, scripts et tests : voir le [guide développeur](docs/DEVELOPMENT.md).

## Licence

Publié sous [licence MIT](LICENSE).
