<script lang="ts">
	let {
		width = '100%',
		height = '14px',
		radius = '10px'
	}: { width?: string; height?: string; radius?: string } = $props();
</script>

<span class="sk" aria-hidden="true" style:width style:height style:border-radius={radius}></span>

<style>
	.sk {
		display: block;
		flex: none;
		background: linear-gradient(90deg, var(--surface-1), var(--surface-3), var(--surface-1));
		background-size: 200% 100%;
		animation: shimmer 1.4s ease-in-out infinite;
	}
	@keyframes shimmer {
		from {
			background-position: 100% 0;
		}
		to {
			background-position: -100% 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.sk {
			animation: none;
		}
	}
</style>
