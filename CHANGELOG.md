# Changelog

## 1.0.1 — 2026-11-25

### Corrections

- fail an e2e shard that receives no test (ci)
- stop downloading Go and restoring its cache in the server job (ci)
- shard the e2e tests over 3 jobs (ci)
- use the Chromium baked into hub-ci instead of downloading it (ci)

## 1.0.0 — 2026-11-20

### Nouveautés

- publish versioned images to GHCR on main (#9) (deploy)
- publish images to GHCR on main (deploy)
- republish only the images whose files changed (ci)
- automatic semver releases on main (ci)
- respect Request API key scopes (widgets)
- replace Seerr with Request widgets, actions and webhook (widgets)

### Corrections

- ignore nested keys in build context, skip provenance manifests (deploy)
- pick images changed since the last release, not the previous push (ci)
- share the release tag lookup, make the release job rerunnable (ci)
- read the Request key scope once, at save (widgets)
