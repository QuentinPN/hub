<script module lang="ts">
	// Passe à true quand l'onglet est prêt : la fiche le rend alors cliquable.
	export const ready = false;
</script>

<script lang="ts">
	import type { ServiceDetail } from '#lib/api/types.ts';

	let { service }: { service: ServiceDetail } = $props();
</script>

<p class="muted">Historique de {service.name} : à venir.</p>
