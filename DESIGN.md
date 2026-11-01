# Design brief — Apple TV × visionOS × HomeLab

## PHASE 0 : 3 directions à présenter avant développement
A — Cinematic TV : grands visuels, navigation horizontale, focus cards, hero immersif.
B — Glass Command Center : bento grid, fenêtres translucides, widgets denses mais élégants.
C — Minimal Spatial : barre latérale discrète, cartes aérées, profondeur modérée, lisibilité prioritaire.

Pour chacune : maquette desktop 1440 px, mobile 390 px, état de chargement, état erreur, exemples réels de widgets Emby/Radarr/Sonarr, navigation, palettes, typographie, densité et motion. Inclure comparaison des compromis. Attendre validation explicite.

## Direction retenue (validée le 2026-11-01) : A — Cinematic TV + fiche service
Maquettes de référence : https://claude.ai/artifact/YP9oAnZPuRSqH7GLsyXEtg (lignes A et « Hybride »).

Tokens de départ, à formaliser en phase 1 :
- **Couleurs :**
  - fond `#07080A` ; surfaces `rgba(255,255,255,.05–.10)` ; bordures `rgba(255,255,255,.07–.14)` ;
  - texte `#F5F5F7`, texte secondaire `#A1A1A6` ;
  - accent de lecture `#E8C9A6` ;
  - états : en ligne `#30D158`, dégradé `#FFD60A`, erreur `#FF453A`, inconnu `#8E8E93` en hachuré (jamais rouge).
- **Typographie :** Inter (repli sur la police système Apple). Hero 60–64 px, titres de section 22 px, corps 15–17 px. Chiffres tabulaires.
- **Rayons :** cartes 20 px, affiches 16 px, boutons 14 px, onglets en pilule.
- **Verre :** `backdrop-filter: blur(20–24px)`, réservé à la navigation, aux encadrés posés sur le hero et à la barre d'onglets mobile.
- **Animations :** focus et survol en agrandissement ×1,06 + ombre + bordure claire, 220 ms `cubic-bezier(.2,.8,.2,1)`. Pas d'agrandissement en mouvement réduit.
- **Navigation :** onglets en pilule en haut (Accueil, Médias, Téléchargements, Infra, Tous les services) ; sur mobile, barre d'onglets flottante en verre.
- **Accueil :** hero « En lecture », puis rangées horizontales (Favoris, À venir, Infrastructure).
- **Fiche service :** bandeau teinté à la couleur du service, chiffres clés, sous-onglets (Aperçu, Widgets, Conteneurs, Historique), cartes Identification / Conteneurs et accès / Connexion.
- **À dessiner en phase 1 :** le catalogue « Tous les services » (grille filtrable par hôte et catégorie, pour 34 services ou plus) et les réglages.

## Principes
- Raccourcis clavier affichés et gérés à la façon Windows (`Ctrl+K`), jamais avec les symboles Mac (`⌘`). Demande explicite de l'utilisateur, malgré l'inspiration Apple.
- Dark mode premium en priorité; mode clair éventuel.
- Verre sobre : blur ciblé, bordures fines, transparence lisible, pas de halo gratuit.
- Contrastes WCAG AA, focus visible, clavier et lecteur d’écran.
- Transitions 150–300 ms quand utiles, respects prefers-reduced-motion.
- Icônes cohérentes, vrais logos si licences et sources le permettent; fallback.
- Responsive sans miniaturiser les widgets, grille adaptable et réorganisation.
- Personnalisation : ordre, tailles, sections, favoris, visibilité, densité et fonds.
- Pas de gradients violets génériques ni de faux indicateurs système.

## Livrables après validation
Tokens (couleurs, rayon, spacing, typo, elevation, blur, motion), composants, états interactifs, prototype et tests visuels. Auditer chaque écran avec unslop.
