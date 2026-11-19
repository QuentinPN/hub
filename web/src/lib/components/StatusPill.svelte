<script lang="ts">
	import { statusLabel, statusTone } from '#lib/request.ts';

	let { status }: { status: string } = $props();
</script>

<span class="pill {statusTone(status)}">{statusLabel(status)}</span>

<style>
	.pill {
		display: inline-block;
		flex: none;
		padding: 3px 10px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: var(--surface-1);
		font-size: 12px;
		font-weight: 600;
		white-space: nowrap;
	}
	.ok {
		border-color: var(--tint-ok-border);
		background: var(--tint-ok-bg);
		color: var(--text-ok);
	}
	.bad {
		border-color: var(--tint-danger-border);
		background: var(--tint-danger-bg);
		color: var(--text-danger);
	}
	.accent {
		border-color: var(--tint-accent-border);
		background: var(--tint-accent-bg);
		color: var(--accent-text);
	}
</style>
