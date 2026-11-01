# Security baseline

- Ne jamais exposer /var/run/docker.sock au navigateur ou au serveur web frontal.
- Un accès Docker Engine, même dit « read-only », est potentiellement hautement privilégié : utiliser un agent/proxy strictement limité aux opérations nécessaires, isolé et audité; analyser le modèle de menace.
- Communications inter-hôtes chiffrées, authentification forte, rotation et révocation des identifiants.
- Secrets API chiffrés au repos avec clé hors base de données, jamais en logs ni en payload client.
- Dashboard protégé par authentification; admin LAN/VPN par défaut; contrôle d’accès par rôles si partage.
- SSRF : allowlist explicite d’hôtes/ports de connecteurs, validation DNS/IP, blocage des destinations dangereuses; aucun fetch arbitraire contrôlé par un utilisateur non privilégié.
- CSRF, XSS, cookies sécurisés, CSP, validation Zod, rate limiting, audit log.
- Pas d’action de démarrage/arrêt/restart sans fonctionnalité distincte, autorisation forte et confirmation.
- Scans de dépendances, images épinglées, utilisateurs non-root, backups testés.
