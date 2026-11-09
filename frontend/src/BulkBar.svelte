<script lang="ts">
  import { Button } from "@kenn-io/kit-ui";
  import type { BulkAction } from "./lib/api.ts";
  import { commitTagText } from "./lib/query.ts";
  import type { Filter } from "./lib/view.ts";
  import TagInput from "./TagInput.svelte";

  interface Props {
    count: number;
    filter: Filter;
    busy: boolean;
    message: string;
    error: string;
    /** False when "all matching" would make a delete match everything. */
    canDelete: boolean;
    suggestions: string[];
    /** Runs the action; resolves true on success. */
    run: (action: BulkAction, tags?: string[]) => Promise<boolean>;
    onexit: () => void;
    /** True while an inline panel (tags or delete confirm) is open. */
    panelOpen?: boolean;
  }
  let {
    count,
    filter,
    busy,
    message,
    error,
    canDelete,
    suggestions,
    run,
    onexit,
    panelOpen = $bindable(false),
  }: Props = $props();

  let panel = $state<null | "tag" | "untag" | "delete">(null);
  let tags = $state<string[]>([]);
  let text = $state("");
  const noun = $derived(count === 1 ? "bookmark" : "bookmarks");
  const pending = $derived(commitTagText(tags, text));

  $effect(() => {
    panelOpen = panel !== null;
  });
  // The selection went away (new search, page or success): close any panel.
  $effect(() => {
    if (count === 0) closePanel();
  });

  function closePanel() {
    panel = null;
    tags = [];
    text = "";
  }
  function open(p: "tag" | "untag" | "delete") {
    tags = [];
    text = "";
    panel = p;
  }
  async function go(action: BulkAction, withTags?: string[]) {
    if (await run(action, withTags)) closePanel();
  }
</script>

<div class="bulk-bar" role="region" aria-label="Bulk actions">
  <div class="bulk-row">
    <strong class="bulk-count" aria-live="polite">{count} selected</strong>
    {#if message}<span class="bulk-msg" role="status">{message}</span>{/if}
    {#if error}<span class="bulk-err" role="alert">{error}</span>{/if}
    <span class="grow"></span>
    <Button size="sm" onclick={onexit} disabled={busy}>Done</Button>
  </div>
  {#if panel === "tag" || panel === "untag"}
    <div class="bulk-panel">
      <TagInput
        bind:tags
        bind:text
        {suggestions}
        disabled={busy}
        placeholder={panel === "tag" ? "Tags to add" : "Tags to remove"}
      />
      <div class="bulk-row">
        <Button
          size="sm"
          tone="info"
          surface="solid"
          disabled={busy || pending.length === 0}
          onclick={() => go(panel as "tag" | "untag", pending)}
        >
          {panel === "tag" ? "Add" : "Remove"} on {count}
          {noun}
        </Button>
        <Button size="sm" disabled={busy} onclick={closePanel}>Cancel</Button>
      </div>
    </div>
  {:else if panel === "delete"}
    <div class="bulk-panel bulk-row">
      <span>Delete {count} {noun}?</span>
      <Button size="sm" tone="danger" surface="solid" disabled={busy} onclick={() => go("delete")}>
        Yes, delete
      </Button>
      <Button size="sm" disabled={busy} onclick={closePanel}>Cancel</Button>
    </div>
  {:else}
    <div class="bulk-row bulk-actions">
      <Button
        size="sm"
        disabled={busy || count === 0}
        onclick={() => go(filter === "archived" ? "unarchive" : "archive")}
      >
        {filter === "archived" ? "Unarchive" : "Archive"}
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => go("read")}>Mark read</Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => go("unread")}>
        Mark unread
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => open("tag")}>
        Add tags…
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => open("untag")}>
        Remove tags…
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => go("refresh_metadata")}>
        Refresh metadata
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => go("capture")}>
        Capture
      </Button>
      <Button size="sm" disabled={busy || count === 0} onclick={() => go("extract")}>
        Re-extract
      </Button>
      <Button
        size="sm"
        tone="danger"
        disabled={busy || count === 0 || !canDelete}
        onclick={() => open("delete")}
      >
        Delete
      </Button>
    </div>
    {#if !canDelete}
      <p class="bulk-hint">Narrow the search to delete all matching, or select rows.</p>
    {/if}
  {/if}
</div>
