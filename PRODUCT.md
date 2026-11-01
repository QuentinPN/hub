# Product requirements — HomeLab Dashboard

## Objectif
Un point d’entrée unique pour découvrir, organiser, consulter et surveiller les applications Docker auto-hébergées.

## Périmètre
- Importer tous les services déjà présents sur chaque hôte connecté lors du premier scan.
- Détecter automatiquement nouveaux services, mises à jour, arrêts, suppressions et renommages; rescan périodique + événements quand disponibles.
- Réconcilier sans doublons et sans effacer favoris, catégories, URLs et réglages.
- Reconnaître les applications connues via images, labels, noms et configuration; classification incertaine à confirmer.
- Afficher état, hôte, catégorie, lien, icône, tags, latence et dernière observation; état « inconnu » distinct de « arrêté ».
- Vue d’ensemble, favoris, recherche globale, groupes par hôte/catégorie, vues personnalisées, édition manuelle.
- Widgets spécialisés et génériques, choix du taux de rafraîchissement, skeleton/loading/error states.
- Notifications optionnelles (nouveau service, indisponibilité durable, rétablissement), anti-spam.
- Mobile, desktop, navigation clavier, reduced motion.

## Hors périmètre initial
Orchestration de conteneurs, accès shell, mises à jour automatiques, exposition publique d’API sensibles, écriture dans les apps intégrées.

## Critères d’acceptation
1. Un service déjà actif sur chaque hôte apparaît après connexion et scan initial.
2. Un nouveau service apparaît automatiquement sans modification manuelle de config.
3. Un service supprimé passe à « absent » ou est archivé selon politique; ses réglages ne sont pas perdus.
4. Aucune clé API n’est envoyée au navigateur.
5. Si un agent tombe, les services ne sont pas présentés à tort comme arrêtés.
6. L’UI reste exploitable si un widget échoue.
7. Une validation explicite du design précède le code de production.

## Questions à trancher après les maquettes
Liste exacte des hôtes et applications, accès LAN/VPN/public, auth souhaitée, URL de chaque app, stratégie de découverte réseau, métriques nécessaires et rôles utilisateurs.
