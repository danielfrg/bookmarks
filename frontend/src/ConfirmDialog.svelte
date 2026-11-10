<script lang="ts">
  // A destructive-action confirmation: bold title, one muted line, Cancel and
  // a danger button. No header bar or close button; Escape and the overlay cancel.
  import { Button, Modal } from "@kenn-io/kit-ui";

  interface Props {
    title: string;
    message: string;
    confirmLabel?: string;
    busy?: boolean;
    /** Shown under the message when the action failed. */
    error?: string;
    onconfirm: () => void;
    oncancel: () => void;
  }
  let {
    title,
    message,
    confirmLabel = "Delete",
    busy = false,
    error = "",
    onconfirm,
    oncancel,
  }: Props = $props();
</script>

<Modal ariaLabel={title} closable={false} maxWidth="30rem" onclose={() => !busy && oncancel()}>
  <div class="confirm-dialog">
    <h2 class="confirm-title">{title}</h2>
    <p class="confirm-message">{message}</p>
    {#if error}<p class="notice error" role="alert">{error}</p>{/if}
  </div>
  {#snippet footer()}
    <Button disabled={busy} onclick={oncancel}>Cancel</Button>
    <Button tone="danger" surface="solid" disabled={busy} onclick={onconfirm}>{confirmLabel}</Button
    >
  {/snippet}
</Modal>

<style>
  .confirm-dialog {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .confirm-title {
    margin: 0;
    font-size: var(--font-size-md, 0.9375rem);
    font-weight: 600;
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }
  .confirm-message {
    margin: 0;
    color: var(--text-muted);
    line-height: 1.5;
  }
</style>
