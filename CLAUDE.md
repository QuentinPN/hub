# Claude Code — HomeLab Dashboard

## Mission
Construire un dashboard auto-hébergé premium pour tous les services Docker du mini-PC Debian 13, du QNAP et du Raspberry Pi. Interface inspirée d’Apple TV/tvOS et visionOS, glassmorphism subtil, rapide, accessible et personnalisable.

## RÈGLE ABSOLUE — DESIGN FIRST
PHASE 0 UNIQUEMENT au démarrage. Ne pas écrire le code de production, initialiser le projet applicatif ou installer des dépendances avant validation explicite du design par l’utilisateur. Produire 3 propositions visuelles réellement distinctes (maquettes desktop + mobile), comparatif et recommandation argumentée. Demander « Quelle direction valides-tu ? ». Attendre la réponse. Les maquettes exploratoires isolées sont autorisées.

## Lecture obligatoire
Lire PRODUCT.md, ARCHITECTURE.md, DESIGN.md, ROADMAP.md, SECURITY.md, WIDGETS.md, TESTING.md et SKILLS.md. Tenir les décisions à jour dans DECISIONS.md.

## Skills à exploiter
Consulter les quatre références demandées et leurs instructions d'installation :
- https://www.ui-skills.com/skills/emilkowalski/apple-design
- https://github.com/nextlevelbuilder/ui-ux-pro-max-skill
- https://github.com/multica-ai/andrej-karpathy-skills
- https://github.com/heydamir/unslop
Ne pas supposer qu’un dépôt est un skill unique : inspecter README, licence et dossiers de skills. Ajouter si pertinents des skills spécialisés Playwright, accessibilité, sécurité Docker, SvelteKit, PostgreSQL et observabilité. Ne jamais exécuter de script distant sans inspection ni accord.

## Règles de réalisation
1. Design validé avant toute implémentation.
2. Aucun service existant ne doit être modifié ou arrêté sans accord explicite.
3. Découverte Docker automatique initiale et continue, multi-hôte, sans accès Docker direct du navigateur.
4. Séparer conteneur, application logique, endpoint, widget et hôte. Un service peut contenir plusieurs conteneurs.
5. L’inventaire doit persister après redémarrage; préserver les personnalisations au rescan.
6. Connexion réseau sécurisée, permissions minimales, secrets chiffrés et masqués.
7. Qualité : TypeScript strict, lint/format, tests, migrations, CI, documentation, accessibilité.
8. Procéder par petites étapes testables et faire valider les jalons importants.
9. Ne jamais prétendre qu’un endpoint ou une API existe sans le vérifier.

## Compaction du contexte
Compaction automatique à environ 200K tokens (`autoCompactWindow` dans `.claude/settings.json`). Le résumé de compaction doit permettre de continuer sans rien réexpliquer. Il doit garder :
- **Où on en est :** phase et jalon en cours, branche git, fichiers modifiés non commités.
- **Chaque décision et sa raison :** reprendre aussi DECISIONS.md.
- **Ce que l'utilisateur a demandé ou refusé, avec ses propres mots** : citations, pas de reformulation.
- **Ce qui reste à faire**, dans l'ordre.
- **Les fichiers clés et liens utiles :** docs du projet, canevas de maquettes https://claude.ai/artifact/YP9oAnZPuRSqH7GLsyXEtg.
- **Les pièges déjà rencontrés et comment ils ont été contournés.**

## Première réponse attendue
Résumer les besoins et inconnues critiques, proposer 3 directions de design illustrées, détailler leurs différences (layout, navigation, widgets, animations, personnalisation, responsive), puis attendre le choix. Ne pas commencer le développement.
