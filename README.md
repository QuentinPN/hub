<div align="center">

[English](README.md) · [Français](README.fr.md)

# Hub

**One beautiful front door for everything you self-host.**

Hub finds the apps running on your home servers, keeps track of what is up or down, and puts them all in a fast, TV-style dashboard that works as well on a phone as on a desktop.

![Hub home page: a cinematic dark dashboard with favourite apps, server health and what is coming next](docs/screenshots/hub.webp)

</div>

## Highlights

- **Finds your apps on its own.** Connect a machine once and Hub lists every Docker app already running on it, then notices new, stopped or removed ones. No manual inventory to maintain.
- **Every machine at a glance.** See which apps are online and how many containers each host runs. Connect your monitoring tool and each server's processor, memory and disk load appear right on the home page.
- **Live widgets, not just links.** Cards for your media server, library managers, request manager and server monitoring show real activity, and one failing service never blocks the rest of the page.
- **Yours to arrange.** Favourites, reorderable home rows, a searchable catalogue of all services (press `Ctrl+K` anywhere), dark or light theme, comfortable or compact density.
- **Alerts that respect your evenings.** Get a message on your phone (ntfy) or by e-mail when a service or a machine goes down and when it comes back, with quiet hours and no spam on short blips.
- **Private and safe by design.** Local accounts with passkey support, API keys encrypted at rest and never sent to the browser, and agents that can only read from Docker and never run commands.
- **Easy to run and to restore.** One server program with the interface built in, a PostgreSQL database, and encrypted one-file backups you can schedule and restore.

## How it works

1. **A small agent runs on each machine.** It reads the list of containers through a read-only proxy and reports to Hub over a mutually authenticated, encrypted connection. It opens no port and cannot start or stop anything.
2. **The Hub server does the talking.** It keeps the inventory in PostgreSQL, asks your services for their status server-side, and pushes updates to the browser as they happen.
3. **Your browser only ever talks to Hub.** It never contacts your machines or your apps directly, so no secret ever reaches it.

```mermaid
flowchart LR
    B[Your browser] --> H[Hub server]
    H --> D[(PostgreSQL)]
    A1[Agent: machine 1] --> H
    A2[Agent: machine 2] --> H
    H --> S["Your apps' APIs"]
```

## Quick start

Try it on your computer with demo data. You need Docker, Go 1.27+ and Node 24+, on Linux, or Windows with Git Bash.

```bash
git clone https://github.com/QuentinPN/hub.git && cd hub
docker compose -f deploy/dev/compose.yml up -d --wait
bash scripts/build.sh
HUB_DATABASE_URL="postgres://hub:hub-dev-only@127.0.0.1:55432/hub?sslmode=disable" \
  HUB_SECRET_KEY_FILE=dev-secrets.key HUB_DEMO_DATA=on ./bin/hub-server
```

Open <http://localhost:8080>. On first start the server prints a one-time setup code in its output (`setup_code=...`): enter it, then choose your username and password.

Running it for real on a home server (Docker Compose, HTTPS, backups) is covered in [deploy/server/README.md](deploy/server/README.md).

## Built with

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

For setup details, architecture, scripts and tests, see the [developer guide](docs/DEVELOPMENT.md) (in French).

## License

Released under the [MIT License](LICENSE).
