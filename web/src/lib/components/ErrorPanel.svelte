<script lang="ts">
	let { title, detail, onretry }: { title: string; detail: string; onretry?: () => void } =
		$props();
</script>

<div class="err" role="alert">
	<svg
		width="20"
		height="20"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		aria-hidden="true"
		><circle cx="12" cy="12" r="9"></circle><path d="M12 7v6M12 16.5v.5"></path></svg
	>
	<div class="text">
		<strong>{title}</strong>
		<span>{detail}</span>
	</div>
	{#if onretry}
		<button type="button" onclick={onretry}>Réessayer</button>
	{/if}
</div>

<style>
	.err {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
		padding: 14px 16px;
		border: 1px solid var(--tint-danger-border);
		border-radius: var(--r-button);
		background: var(--tint-danger-bg-strong);
		color: var(--text-danger);
	}
	.text {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 4px;
		min-width: 200px;
		color: var(--text);
	}
	.text span {
		color: var(--text-danger-2);
		font-size: var(--fs-small);
	}
	button {
		min-height: 44px;
		padding: 0 18px;
		border: 0;
		border-radius: 12px;
		background: var(--text);
		color: var(--text-inverse);
		font-weight: 600;
		cursor: pointer;
	}
</style>
