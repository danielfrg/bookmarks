<script lang="ts">
  import { onMount, tick } from "svelte";
  import { api, type ListParams, type Bookmark, type Bundle, type Tag } from "./lib/api.ts";
  import { toggleTagInQuery } from "./lib/query.ts";
  import { listParams, navigate, readView } from "./lib/view.ts";
  import DetailsModal from "./DetailsModal.svelte";
  import BookmarkList from "./BookmarkList.svelte";
  import SidePanel from "./SidePanel.svelte";
  import TopBar from "./TopBar.svelte";

  let view = $state(readView(new URL(window.location.href)));
  let pageSize = $state(30);
  let tags = $state<Tag[]>([]);
  let bundles = $state<Bundle[]>([]);
  let items = $state<Bookmark[]>([]);
  let count = $state(0);
  let status = $state<"loading" | "ready" | "error">("loading");
  let error = $state("");
  let reloadKey = $state(0);
  let panelOpen = $state(false);
  let settingsReady = $state(false);

  let deletedId: number | null = null;
  // After the details modal closes (Esc, backdrop, Close, Back), focus the row
  // that opened it; after a delete the row is going away, so use the list.
  async function restoreFocus(id: number) {
    const deleted = deletedId === id;
    deletedId = null;
    await tick();
    requestAnimationFrame(() => {
      if (view.view !== null) return;
      const target =
        (deleted ? null : document.querySelector<HTMLElement>(`[data-view-trigger="${id}"]`)) ??
        document.querySelector<HTMLElement>(".list-area");
      target?.focus();
    });
  }
  // A->B swap: the old modal's focus-trap teardown can pull focus out of the new one.
  async function keepFocusInDialog() {
    await tick();
    requestAnimationFrame(() => {
      const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
      if (!dialog || dialog.contains(document.activeElement)) return;
      const first = dialog.querySelector<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])',
      );
      (first ?? dialog).focus();
    });
  }
  function sync() {
    const prev = view.view;
    view = readView(new URL(window.location.href));
    if (
      prev !== null &&
      view.view === null &&
      view.edit === null &&
      view.read === null &&
      !view.create
    ) {
      void restoreFocus(prev);
    } else if (prev !== null && view.view !== null && prev !== view.view) {
      void keepFocusInDialog();
    }
  }
  onMount(() => {
    window.addEventListener("viewchange", sync);
    window.addEventListener("popstate", sync);
    api
      .settings()
      .then((s) => {
        const n = s.ui.items_per_page;
        if (typeof n === "number" && n > 0) pageSize = n;
      })
      .catch(() => {})
      .finally(() => (settingsReady = true));
    return () => {
      window.removeEventListener("viewchange", sync);
      window.removeEventListener("popstate", sync);
    };
  });

  async function loadSide() {
    try {
      [tags, bundles] = await Promise.all([api.tags(), api.bundles()]);
    } catch {
      // The side panel is secondary; the list surfaces connection errors.
    }
  }
  $effect(() => {
    void reloadKey;
    loadSide();
  });

  // Only changes to the request itself (not View/Edit/etc.) refetch the list.
  const listKey = $derived(
    view.pageName === "list" ? JSON.stringify(listParams(view, pageSize)) : "",
  );
  // Reload the list when anything the request depends on changes.
  $effect(() => {
    if (!settingsReady) return;
    void reloadKey;
    const key = listKey;
    if (key === "") return;
    const params: ListParams = JSON.parse(key);
    const ctrl = new AbortController();
    status = "loading";
    api
      .list(params, ctrl.signal)
      .then((res) => {
        if (res.results.length === 0 && res.count > 0 && view.page > 1) {
          // Deleted the last row of the last page: step back.
          navigate({ page: Math.max(1, Math.ceil(res.count / pageSize)) }, { replace: true });
          return;
        }
        items = res.results;
        count = res.count;
        error = "";
        status = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        error = e instanceof Error ? e.message : String(e);
        status = "error";
      });
    return () => ctrl.abort();
  });

  const reload = () => {
    reloadKey++;
  };
  const onTag = (tag: string) => navigate({ q: toggleTagInQuery(view.q, tag), page: 1 });
</script>

<TopBar {view} {panelOpen} ontogglepanel={() => (panelOpen = !panelOpen)} />
{#if view.pageName === "list"}
  <div class="layout">
    <main class="main">
      <BookmarkList
        {listKey}
        {reloadKey}
        {status}
        {error}
        {items}
        {count}
        {view}
        {pageSize}
        onreload={reload}
        onchanged={reload}
        ontag={onTag}
      />
    </main>
    <SidePanel {view} {tags} {bundles} open={panelOpen} ontag={onTag} />
  </div>
{:else}
  <main class="main placeholder-page">
    <h2>{view.pageName[0].toUpperCase() + view.pageName.slice(1)}</h2>
    <p class="muted">Coming soon.</p>
  </main>
{/if}

{#if view.view !== null}
  {#key view.view}
    <DetailsModal id={view.view} onchanged={reload} ondeleted={(id) => (deletedId = id)} />
  {/key}
{/if}
