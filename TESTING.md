# Quality and testing

- Unit : normalisation Docker, réconciliation, identification, permissions, mapping widgets.
- Integration : Docker API simulée, DB migrations, connecteurs API mockés, credentials.
- E2E Playwright : import initial, ajout automatique, disparition, reconnexion agent, personnalisation conservée, erreurs widgets.
- Security : accès non autorisé, SSRF, CSRF, XSS, secrets non exposés, RBAC.
- Accessibility : axe, clavier, contrastes, zoom, reduced motion.
- Visual regression : desktop/mobile pour les trois états principaux.
- Performance : budget JS, requêtes, latence de chargement, refresh, nombre élevé de services.
- Operational : backup/restore, upgrade/rollback, hôte offline, perte DB, redémarrage dashboard.
- CI : typecheck, lint, tests, build, dependency scan.
