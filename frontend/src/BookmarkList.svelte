<script lang="ts">
  import { Button, EmptyState, Modal, Spinner } from "@kenn-io/kit-ui";
  import { api, type Bookmark } from "./lib/api.ts";
  import { activeTagsOf, displayTitle } from "./lib/query.ts";
  import { navigate, type View } from "./lib/view.ts";
  import BookmarkRow from "./BookmarkRow.svelte";

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
  }: Props = $props();

  let busyId = $state<number | null>(null);
  let actionError = $state("");
  let confirming = $state<Bookmark | null>(null);
  let confirmTrigger: HTMLElement | null = null;
  let deleting = $state(false);
  let areaEl: HTMLElement | undefined;

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
    <ul class="bm-list" class:stale={status === "loading"}>
      {#each items as b (b.id)}
        <BookmarkRow
          bookmark={b}
          {activeTags}
          busy={busyId === b.id}
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
      <span class="grow"></span>
      <Button size="sm" disabled={view.page <= 1} onclick={() => navigate({ page: view.page - 1 })}>
        Previous
      </Button>
      <Button size="sm" disabled={last >= count} onclick={() => navigate({ page: view.page + 1 })}>
        Next
      </Button>
    </div>
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
