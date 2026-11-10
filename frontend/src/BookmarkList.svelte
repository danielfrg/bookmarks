<script lang="ts">
  import { tick, untrack } from "svelte";
  import { slide } from "svelte/transition";
  import { Button, EmptyState, Modal, Spinner } from "@kenn-io/kit-ui";
  import { api, type Bookmark, type BulkAction } from "./lib/api.ts";
  import { activeTagsOf, bulkRequest, displayTitle } from "./lib/query.ts";
  import { shortcutFor } from "./lib/keys.ts";
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
    onhelp: () => void;
    listKey: string;
    reloadKey: number;
    suggestions?: string[];
    /** Select mode, toggled from the top bar. */
    selectMode?: boolean;
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
    onhelp,
    listKey,
    reloadKey,
    suggestions = [],
    selectMode = $bindable(false),
  }: Props = $props();

  let busyId = $state<number | null>(null);
  let actionError = $state("");
  let confirming = $state<Bookmark | null>(null);
  let confirmTrigger: HTMLElement | null = null;
  let deleting = $state(false);
  let areaEl: HTMLElement | undefined;

  // Bulk selection.
  let selected = $state<Set<number>>(new Set());
  let allMatching = $state(false);
  let bulkBusy = $state(false);
  let bulkMessage = $state("");
  let bulkError = $state("");
  let panelOpen = $state(false);
  let messageTimer: ReturnType<typeof setTimeout> | undefined;

  // Leaving select mode (from here or the top bar) drops the selection.
  $effect(() => {
    if (selectMode) return;
    untrack(() => {
      selected = new Set();
      allMatching = false;
      bulkError = "";
      panelOpen = false;
    });
  });

  // The bulk bar slides in and out, unless reduced motion is preferred.
  const slideMs = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 160;

  // A new search, filter or page drops the selection (select mode stays on).
  $effect(() => {
    void listKey;
    selected = new Set();
    allMatching = false;
    bulkError = "";
  });

  // Drop selected ids that are no longer in the loaded rows (after a reload).
  $effect(() => {
    const ids = new Set(items.map((b) => b.id));
    const cur = untrack(() => selected);
    if ([...cur].some((id) => !ids.has(id)))
      selected = new Set([...cur].filter((id) => ids.has(id)));
  });

  // Keyboard-highlighted row.
  let activeId = $state<number | null>(null);
  $effect(() => {
    void listKey;
    activeId = null;
  });
  $effect(() => {
    if (activeId !== null && !items.some((b) => b.id === activeId)) activeId = null;
  });
  async function moveActive(step: 1 | -1) {
    if (items.length === 0) return;
    const i = items.findIndex((b) => b.id === activeId);
    const n = items.length;
    const next = i === -1 ? (step === 1 ? 0 : n - 1) : (i + step + n) % n;
    activeId = items[next].id;
    await tick();
    document
      .querySelector<HTMLElement>(`[data-bm="${activeId}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }
  function onShortcut(e: KeyboardEvent) {
    if (e.defaultPrevented) return;
    const sc = shortcutFor(e);
    if (!sc) return;
    // Shortcuts are for the list with nothing modal open.
    if (confirming || view.view !== null || view.edit !== null || view.create) return;
    if (document.querySelector("[role=dialog]")) return;
    const t = e.target as HTMLElement | null;
    // Enter/arrows on a focused control keep their native meaning.
    if (e.key === "Enter" && t?.closest("a, button")) return;
    if (t?.closest("[role=combobox], [role=listbox], [role=menu]")) return;
    const row = items.find((b) => b.id === activeId);
    switch (sc) {
      case "new":
        navigate({ create: { url: "", title: "" } });
        break;
      case "search":
        document.querySelector<HTMLInputElement>(".search-wrap input")?.focus();
        break;
      case "next":
        void moveActive(1);
        break;
      case "prev":
        void moveActive(-1);
        break;
      case "open":
        if (!row) return;
        window.open(row.url, "_blank", "noopener,noreferrer");
        break;
      case "view":
        if (!row) return;
        navigate({ view: row.id });
        break;
      case "edit":
        if (!row) return;
        navigate({ edit: row.id });
        break;
      case "help":
        onhelp();
        break;
    }
    e.preventDefault();
  }

  const pageSelected = $derived(items.length > 0 && items.every((b) => selected.has(b.id)));
  const selectedCount = $derived(allMatching ? count : selected.size);
  const canSelectAll = $derived(view.saved === null);
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
    onShortcut(e);
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
  const hasFilters = $derived(view.q !== "" || view.saved !== null);
  // "All" with no search and no results: the library itself is empty.
  const libraryEmpty = $derived(
    status === "ready" && count === 0 && !hasFilters && view.filter === "all",
  );

  async function markRead(b: Bookmark) {
    busyId = b.id;
    actionError = "";
    try {
      await api.update(b.id, { unread: false });
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
  {:else if libraryEmpty}
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
        <Button onclick={() => navigate({ q: "", saved: null, page: 1 })}>Clear search</Button>
      {/if}
    </EmptyState>
  {:else}
    {#if selectMode}
      <div class="bulk-top" transition:slide={{ duration: slideMs }}>
        <BulkBar
          count={selectedCount}
          busy={bulkBusy}
          error={bulkError}
          canDelete={canDeleteSelection}
          {suggestions}
          run={runBulk}
          onexit={exitSelect}
          bind:panelOpen
        />
        <div class="select-head">
          <label class="select-all">
            <input type="checkbox" checked={pageSelected} onchange={togglePage} />
            Select all
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
                title="Bulk actions on a whole saved search are not supported; select rows instead."
              >
                Select all matching is unavailable in a saved search
              </span>
            {/if}
          {/if}
        </div>
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
          active={activeId === b.id}
          ontoggle={() => toggleRow(b.id)}
          {ontag}
          onview={() => navigate({ view: b.id })}
          onedit={() => navigate({ edit: b.id })}
          onmarkread={() => markRead(b)}
          ondelete={(el) => {
            confirmTrigger = el;
            confirming = b;
          }}
        />
      {/each}
    </ul>
    <div class="pager">
      <span class="muted">Showing {first}–{last} of {count}</span>
      <span class="grow"></span>
      <Button size="sm" disabled={view.page <= 1} onclick={() => navigate({ page: view.page - 1 })}>
        Previous
      </Button>
      <Button size="sm" disabled={last >= count} onclick={() => navigate({ page: view.page + 1 })}>
        Next
      </Button>
    </div>
    {#if !selectMode && bulkMessage}
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
