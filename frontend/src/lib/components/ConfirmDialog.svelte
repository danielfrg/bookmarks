<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";
  import {
    AlertDialog,
    AlertDialogBackdrop,
    AlertDialogClose,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogTitle,
  } from "@ui-sdk/svelte/alert-dialog";

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

<!-- The parent owns the open state; a close request while busy is ignored. -->
<AlertDialog
  open
  onOpenChange={(open, details) => {
    if (open) return;
    details.cancel();
    if (!busy) oncancel();
  }}
>
  <AlertDialogBackdrop />
  <AlertDialogContent>
    <div data-slot="stack">
      <AlertDialogTitle class="confirm-title">{title}</AlertDialogTitle>
      <AlertDialogDescription>{message}</AlertDialogDescription>
    </div>
    {#if error}<p data-slot="hint" role="alert">{error}</p>{/if}
    <div data-slot="row">
      <AlertDialogClose disabled={busy}>Cancel</AlertDialogClose>
      <Button data-variant="destructive" disabled={busy} onclick={onconfirm}>{confirmLabel}</Button>
    </div>
  </AlertDialogContent>
</AlertDialog>
