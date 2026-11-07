<script lang="ts">
	// Repli en attendant les vrais logos (licences à vérifier, cf. DESIGN.md).
	let { text, color, size = 44 }: { text: string; color: string; size?: number } = $props();
</script>

<span
	class="mono-tile"
	aria-hidden="true"
	style:width="{size}px"
	style:height="{size}px"
	style:background={color}
	style:font-size="{Math.round(size * 0.4)}px"
	style:border-radius="{Math.round(size * 0.26)}px">{text}</span
>

<style>
	.mono-tile {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		font-weight: 700;
		color: var(--on-color);
	}
</style>
