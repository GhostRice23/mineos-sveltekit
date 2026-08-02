<script lang="ts">
	interface Props {
		value: string;
		error: string;
		onchange: (name: string) => void;
		oncreate: () => void;
		onback: () => void;
	}

	let { value, error, onchange, oncreate, onback }: Props = $props();

	const namePattern = /^[a-zA-Z0-9][a-zA-Z0-9 _\-\.]{0,63}$/;
	const isValid = $derived(namePattern.test(value.trim()) && !value.includes('..'));
</script>

<div class="step">
	<div class="header">
		<h2>Name your server</h2>
	</div>

	<div class="name-input-group">
		<input
			type="text"
			placeholder="my-server"
			value={value}
			oninput={(e) => onchange(e.currentTarget.value)}
			class="name-input"
			class:invalid={value.trim() && !isValid}
		/>
		{#if value.trim() && !isValid}
			<p class="validation-error">
				Name must start with a letter or number, and contain only letters, numbers, spaces,
				hyphens, underscores, or dots. Max 64 characters.
			</p>
		{/if}
		{#if error}
			<p class="validation-error">{error}</p>
		{/if}
	</div>

</div>

<style>
	.step {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.header {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
	}

	.name-input-group {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.name-input {
		padding: 0.6rem 0.75rem;
		border: 2px solid var(--border-color, #374151);
		border-radius: 0.5rem;
		background: var(--input-bg, #1f2937);
		color: inherit;
		font-size: 1rem;
	}

	.name-input:focus {
		outline: none;
		border-color: #3b82f6;
	}

	.name-input.invalid {
		border-color: #ef4444;
	}

	.validation-error {
		margin: 0;
		font-size: 0.8rem;
		color: #ef4444;
	}

</style>
