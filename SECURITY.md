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
- Écritures vers un service (Request seulement : approuver, refuser, demander) : uniquement à la demande de l'utilisateur connecté, session + même origine, chemin construit depuis un entier validé, auditées. Les clés Request ont aujourd'hui les droits du propriétaire (réglages compris) : les remplacer par des clés à droits limités dès qu'Request les propose.
- Seule route publique hors connexion : `POST /api/v1/hooks/{slug}` (webhooks entrants). Secret de 32 octets aléatoires par connexion, chiffré comme les clés (`widget:<slug>:webhook_secret`), comparé en temps constant, montré une seule fois, supprimé avec la connexion ; même 401 pour un mauvais secret, un service inconnu ou sans webhook ; corps de 64 Kio au plus, lu en entier et refusé (400) s'il n'est pas du JSON valide ; 20 refus par minute et par IP au plus, au-delà 429 avec `Retry-After`, sans rien chercher du service ; un appel ne fait que réveiller une relecture espacée de 5 s.
