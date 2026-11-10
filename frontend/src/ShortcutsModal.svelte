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
    { keys: ["?"], label: "Show this help" },
  ];

  const search: { query: string; label: string }[] = [
    { query: "#tag", label: "Has the tag" },
    { query: "!unread", label: "Unread" },
    { query: "!untagged", label: "No tags" },
    { query: "!noarchive", label: "No archive" },
    { query: "!basic", label: "Only a basic capture" },
    { query: '"exact phrase"', label: "Phrase match" },
    { query: "a and b", label: "Both (the default)" },
    { query: "a or b", label: "Either" },
    { query: "not a", label: "Exclude" },
  ];
</script>

<Modal title="Help" {onclose} maxWidth="28rem">
  <h3 class="help-heading">Keyboard shortcuts</h3>
  <p class="muted shortcuts-note">Active on the bookmark list when no dialog is open.</p>
  <ul class="shortcuts">
    {#each rows as r (r.keys[0])}
      <li>
        <KbdBadge keys={r.keys} />
        <span>{r.label}</span>
      </li>
    {/each}
  </ul>

  <h3 class="help-heading">Search</h3>
  <ul class="shortcuts">
    {#each search as r (r.query)}
      <li>
        <code class="mono">{r.query}</code>
        <span>{r.label}</span>
      </li>
    {/each}
  </ul>
</Modal>
