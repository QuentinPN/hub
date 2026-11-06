<script lang="ts">
	import type { Host } from '#lib/api/types.ts';

	let { host }: { host: Host } = $props();
</script>

<article class="host" class:offline={!host.reachable} aria-label={host.name}>
	<div class="head">
		<div>
			<div class="name">{host.name}</div>
			<div class="muted small">{host.os ? `${host.os} · ` : ''}{host.containers} conteneurs</div>
		</div>
		<span class="small" class:ok={host.reachable}>
			{host.reachable ? 'Agent connecté' : 'Agent injoignable'}
		</span>
	</div>
	{#if host.reachable}
		<div class="metrics small">
			{#each host.metrics as m (m.label)}
				<span class="muted">{m.label}</span>
				<div
					class="bar"
					role="meter"
					aria-label={m.label}
					aria-valuenow={m.percent}
					aria-valuemin={0}
					aria-valuemax={100}
				>
					<span style:width="{m.percent}%" class:warn={m.percent >= 70}></span>
				</div>
				<span class="tabular pct">{m.percent} %</span>
			{/each}
		</div>
	{:else}
		<p class="muted small note">
			Dernier contact {host.lastSeenLabel}. Ses services sont affichés « inconnu », pas « arrêté ».
		</p>
	{/if}
</article>

<style>
	.host {
		display: flex;
		flex: 1 1 300px;
		flex-direction: column;
		gap: 14px;
		min-width: 0;
		padding: 22px;
		border: 1px solid var(--border-1);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.host.offline {
		background: var(--hatch);
	}
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.name {
		font-size: 17px;
		font-weight: 600;
	}
	.small {
		font-size: var(--fs-small);
	}
	.ok {
		color: var(--status-online);
	}
	.metrics {
		display: grid;
		grid-template-columns: 52px minmax(0, 1fr) 44px;
		gap: 10px;
		align-items: center;
	}
	.bar {
		height: 6px;
		overflow: hidden;
		border-radius: 99px;
		background: rgba(255, 255, 255, 0.1);
	}
	.bar span {
		display: block;
		height: 100%;
		border-radius: 99px;
		background: var(--text);
	}
	.bar span.warn {
		background: var(--status-degraded);
	}
	.pct {
		text-align: right;
	}
	.note {
		margin: 0;
		line-height: 1.45;
	}
</style>
