<script lang="ts">
  import { KbdBadge, Modal } from "@kenn-io/kit-ui";

  interface Props {
    onclose: () => void;
  }
  let { onclose }: Props = $props();

  const rows: { keys: string[]; label: string }[] = [
    { keys: ["n"], label: "Add a bookmark" },
    { keys: ["s"], label: "Focus search" },
    { keys: ["/"], label: "Focus search" },
    { keys: ["j"], label: "Next bookmark" },
    { keys: ["k"], label: "Previous bookmark" },
    { keys: ["↓"], label: "Next bookmark" },
    { keys: ["↑"], label: "Previous bookmark" },
    { keys: ["Enter"], label: "Open the highlighted link in a new tab" },
    { keys: ["o"], label: "Open the highlighted link in a new tab" },
    { keys: ["v"], label: "View details" },
    { keys: ["e"], label: "Edit" },
    { keys: ["?"], label: "Show this list" },
  ];
</script>

<Modal title="Keyboard shortcuts" {onclose} maxWidth="26rem">
  <p class="muted shortcuts-note">Active on the bookmark list when no dialog is open.</p>
  <ul class="shortcuts">
    {#each rows as r (r.keys[0])}
      <li>
        <KbdBadge keys={r.keys} />
        <span>{r.label}</span>
      </li>
    {/each}
  </ul>
</Modal>
