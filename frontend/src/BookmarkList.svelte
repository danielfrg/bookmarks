<script lang="ts">
  import { Button, EmptyState, Modal, Spinner } from "@kenn-io/kit-ui";
  import { api, type Bookmark, type BulkAction } from "./lib/api.ts";
  import { activeTagsOf, bulkRequest, displayTitle } from "./lib/query.ts";
  import { navigate, type View } from "./lib/view.ts";
  import BookmarkRow from "./BookmarkRow.svelte";
  import BulkBar from "./BulkBar.svelte";

  interface Props {
    status: "loading" | "ready" | "error";
    error: string;
    items: Bookmark[];
    count: number;
    view: View;
    pageSize: number;
    onreload: () => void;
    onchanged: () => void;
    ontag: (tag: string) => void;
    listKey: string;
    reloadKey: number;
    suggestions?: string[];
  }
  let {
    status,
    error,
    items,
    count,
    view,
    pageSize,
    onreload,
    onchanged,
    ontag,
    listKey,
    reloadKey,
    suggestions = [],
  }: Props = $props();

  let busyId = $state<number | null>(null);
  let actionError = $state("");
  let confirming = $state<Bookmark | null>(null);
  let confirmTrigger: HTMLElement | null = null;
  let deleting = $state(false);
  let areaEl: HTMLElement | undefined;

  // Bulk selection.
  let selectMode = $state(false);
  let selected = $state<Set<number>>(new Set());
  let allMatching = $state(false);
  let bulkBusy = $state(false);
  let bulkMessage = $state("");
  let bulkError = $state("");
  let panelOpen = $state(false);
  let messageTimer: ReturnType<typeof setTimeout> | undefined;

  // A new search, filter or page drops the selection (select mode stays on).
  $effect(() => {
    void listKey;
    selected = new Set();
    allMatching = false;
    bulkError = "";
  });

  const pageSelected = $derived(items.length > 0 && items.every((b) => selected.has(b.id)));
  const selectedCount = $derived(allMatching ? count : selected.size);
  const canSelectAll = $derived(view.bundle === null);
  const canDeleteSelection = $derived(
    bulkRequest({ ids: [...selected], allMatching }, view, "delete") !== null,
  );

  function toggleRow(id: number) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
    allMatching = false;
  }
  function togglePage() {
    selected = pageSelected ? new Set() : new Set(items.map((b) => b.id));
    allMatching = false;
  }
  function exitSelect() {
    selectMode = false;
    selected = new Set();
    allMatching = false;
    bulkError = "";
    panelOpen = false;
  }
  async function runBulk(action: BulkAction, tags?: string[]): Promise<boolean> {
    const req = bulkRequest({ ids: [...selected], allMatching }, view, action, tags);
    if (!req) return false;
    bulkBusy = true;
    bulkError = "";
    try {
      const res = await api.bulk(req);
      exitSelect();
      clearTimeout(messageTimer);
      bulkMessage = `Updated ${res.affected} ${res.affected === 1 ? "bookmark" : "bookmarks"}`;
      messageTimer = setTimeout(() => (bulkMessage = ""), 4000);
      onchanged();
      return true;
    } catch (e) {
      bulkError = e instanceof Error ? e.message : String(e);
      return false;
    } finally {
      bulkBusy = false;
    }
  }
  function onWindowKeydown(e: KeyboardEvent) {
    if (e.key !== "Escape" || !selectMode || panelOpen || e.defaultPrevented || bulkBusy) return;
    const t = e.target as HTMLElement | null;
    if (t?.closest("input, textarea, select, [role=dialog], [role=combobox]")) return;
    if (document.querySelector("[role=dialog]")) return;
    exitSelect();
  }
  $effect(() => () => clearTimeout(messageTimer));

  // A new request (navigation or reload) clears any stale action error.
  $effect(() => {
    void listKey;
    void reloadKey;
    actionError = "";
  });

  const activeTags = $derived(activeTagsOf(view.q));
  const first = $derived((view.page - 1) * pageSize + 1);
  const last = $derived(Math.min(count, (view.page - 1) * pageSize + items.length));
  const hasFilters = $derived(view.q !== "" || view.bundle !== null);

  async function archive(b: Bookmark) {
    busyId = b.id;
    actionError = "";
    try {
      await api.update(b.id, { is_archived: !b.is_archived });
      onchanged();
    } catch (e) {
      actionError = e instanceof Error ? e.message : String(e);
    } finally {
      busyId = null;
    }
  }

  async function confirmDelete() {
    if (!confirming) return;
    deleting = true;
    actionError = "";
    try {
      await api.remove(confirming.id);
      confirming = null;
      confirmTrigger = null;
      queueMicrotask(() => areaEl?.focus());
      onchanged();
    } catch (e) {
      actionError = e instanceof Error ? e.message : String(e);
      closeConfirm();
    } finally {
      deleting = false;
    }
  }
  function closeConfirm() {
    confirming = null;
    const t = confirmTrigger;
    confirmTrigger = null;
    if (t?.isConnected) queueMicrotask(() => t.focus());
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<section class="list-area" tabindex="-1" bind:this={areaEl} aria-busy={status === "loading"}>
  {#if actionError}
    <p class="notice error" role="alert">{actionError}</p>
  {/if}

  {#if status === "error"}
    <EmptyState title="Could not load bookmarks" description={error}>
      <Button onclick={onreload}>Retry</Button>
    </EmptyState>
  {:else if status === "loading" && items.length === 0}
    <div class="center"><Spinner label="Loading bookmarks" /></div>
  {:else if status === "ready" && count === 0 && !hasFilters && view.filter === "active"}
    <EmptyState title="No bookmarks yet" description="Save your first link to start your library.">
      <Button
        tone="info"
        surface="solid"
        onclick={() => navigate({ create: { url: "", title: "" } })}
      >
        Add bookmark
      </Button>
    </EmptyState>
  {:else if status === "ready" && count === 0}
    <EmptyState
      title="No bookmarks match"
      description={hasFilters ? "Nothing matches this search." : `No ${view.filter} bookmarks.`}
    >
      {#if hasFilters}
        <Button onclick={() => navigate({ q: "", bundle: null, page: 1 })}>Clear search</Button>
      {/if}
    </EmptyState>
  {:else}
    {#if selectMode}
      <div class="select-head">
        <label class="select-all">
          <input type="checkbox" checked={pageSelected} onchange={togglePage} />
          Select page
        </label>
        {#if allMatching}
          <span>All {count} matching selected.</span>
          <button type="button" class="link" onclick={() => (allMatching = false)}>Clear</button>
        {:else if pageSelected && count > items.length}
          {#if canSelectAll}
            <button type="button" class="link" onclick={() => (allMatching = true)}>
              Select all {count} matching
            </button>
          {:else}
            <span
              class="muted"
              title="Bulk actions on a whole bundle are not supported; select rows instead."
            >
              Select all matching is unavailable in a bundle
            </span>
          {/if}
        {/if}
      </div>
    {/if}
    <ul class="bm-list" class:stale={status === "loading"}>
      {#each items as b (b.id)}
        <BookmarkRow
          bookmark={b}
          {activeTags}
          busy={busyId === b.id}
          selecting={selectMode}
          selected={selected.has(b.id) || allMatching}
          ontoggle={() => toggleRow(b.id)}
          {ontag}
          onview={() => navigate({ view: b.id })}
          onedit={() => navigate({ edit: b.id })}
          onarchive={() => archive(b)}
          ondelete={(el) => {
            confirmTrigger = el;
            confirming = b;
          }}
        />
      {/each}
    </ul>
    <div class="pager">
      <span class="muted">Showing {first}–{last} of {count}</span>
      <Button size="sm" onclick={() => (selectMode ? exitSelect() : (selectMode = true))}>
        {selectMode ? "Cancel select" : "Select"}
      </Button>
      <span class="grow"></span>
      <Button size="sm" disabled={view.page <= 1} onclick={() => navigate({ page: view.page - 1 })}>
        Previous
      </Button>
      <Button size="sm" disabled={last >= count} onclick={() => navigate({ page: view.page + 1 })}>
        Next
      </Button>
    </div>
    {#if selectMode}
      <BulkBar
        count={selectedCount}
        filter={view.filter}
        busy={bulkBusy}
        message={bulkMessage}
        error={bulkError}
        canDelete={canDeleteSelection}
        {suggestions}
        run={runBulk}
        onexit={exitSelect}
        bind:panelOpen
      />
    {:else if bulkMessage}
      <p class="bulk-msg standalone" role="status">{bulkMessage}</p>
    {/if}
  {/if}
</section>

{#if confirming}
  <Modal title="Delete bookmark?" tone="danger" onclose={closeConfirm}>
    <p class="confirm-text">
      “{displayTitle(confirming)}” will be permanently deleted along with its archives.
    </p>
    {#snippet footer()}
      <Button onclick={closeConfirm}>Cancel</Button>
      <Button tone="danger" surface="solid" disabled={deleting} onclick={confirmDelete}>
        Delete
      </Button>
    {/snippet}
  </Modal>
{/if}
