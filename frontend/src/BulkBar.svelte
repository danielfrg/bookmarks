<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";
  import type { BulkAction } from "./lib/api.ts";
  import { commitTagText } from "./lib/query.ts";
  import TagInput from "./TagInput.svelte";

  interface Props {
    count: number;
    busy: boolean;
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
    busy,
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
    {#if error}<span class="bulk-err" role="alert">{error}</span>{/if}
    <span class="grow"></span>
    <Button data-variant="ghost" onclick={onexit} disabled={busy}>Done</Button>
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
          data-variant="solid"
          disabled={busy || pending.length === 0}
          onclick={() => go(panel as "tag" | "untag", pending)}
        >
          {panel === "tag" ? "Add" : "Remove"} on {count}
          {noun}
        </Button>
        <Button disabled={busy} onclick={closePanel}>Cancel</Button>
      </div>
    </div>
  {:else if panel === "delete"}
    <div class="bulk-panel bulk-row">
      <span>Delete {count} {noun}?</span>
      <Button data-variant="destructive" disabled={busy} onclick={() => go("delete")}>
        Yes, delete
      </Button>
      <Button disabled={busy} onclick={closePanel}>Cancel</Button>
    </div>
  {:else}
    <div class="bulk-row bulk-actions">
      <Button disabled={busy || count === 0} onclick={() => go("read")}>Mark read</Button>
      <Button disabled={busy || count === 0} onclick={() => go("unread")}>Mark unread</Button>
      <Button disabled={busy || count === 0} onclick={() => open("tag")}>Add tags…</Button>
      <Button disabled={busy || count === 0} onclick={() => open("untag")}>Remove tags…</Button>
      <Button disabled={busy || count === 0} onclick={() => go("refresh_metadata")}>
        Refresh metadata
      </Button>
      <Button disabled={busy || count === 0} onclick={() => go("capture")}>Capture</Button>
      <Button disabled={busy || count === 0} onclick={() => go("extract")}>Re-extract</Button>
      <Button
        data-variant="destructive"
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
