<script lang="ts">
	import type { Session } from '#lib/api/types.ts';

	let { sessions }: { sessions: Session[] } = $props();
</script>

{#each sessions as s (s.device + s.title)}
	<div class="session">
		<span class="who">
			<strong>{s.title}</strong>
			<span class="muted">{s.device} · {s.transcoding ? 'transcodage' : 'lecture directe'}</span>
		</span>
		<span class="progress-box">
			<span
				class="progress"
				role="progressbar"
				aria-label="Progression de {s.title}"
				aria-valuenow={Math.round(s.progress * 100)}
				aria-valuemin={0}
				aria-valuemax={100}><span style:width="{s.progress * 100}%"></span></span
			>
			<span class="muted small">{s.durationLabel}</span>
		</span>
	</div>
{:else}
	<p class="muted">Rien en lecture.</p>
{/each}

<style>
	p {
		margin: 4px 0 0;
	}
	.session {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 240px;
		align-items: center;
		gap: 24px;
		padding-top: 14px;
		border-top: 1px solid var(--border-1);
	}
	.who,
	.progress-box {
		display: flex;
		flex-direction: column;
		gap: 5px;
		min-width: 0;
	}
	.who strong {
		font-size: 16px;
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.progress {
		display: block;
		height: 5px;
		overflow: hidden;
		border-radius: 99px;
		background: var(--track);
	}
	.progress span {
		display: block;
		height: 100%;
		background: var(--accent-play);
	}
	.small {
		font-size: 14px;
	}
	@media (max-width: 960px) {
		.session {
			grid-template-columns: minmax(0, 1fr);
			gap: 10px;
		}
	}
</style>
