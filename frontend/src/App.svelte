<script lang="ts">
  import { onMount, tick } from "svelte";
  import {
    api,
    ApiError,
    type ListParams,
    type Bookmark,
    type SavedSearch,
    type Settings,
    type Tag,
  } from "./lib/api.ts";
  import { toggleTagInQuery } from "./lib/query.ts";
  import { listParams, navigate, readView } from "./lib/view.ts";
  import DetailsModal from "./DetailsModal.svelte";
  import EditModal from "./EditModal.svelte";
  import ReaderPage from "./ReaderPage.svelte";
  import BookmarkList from "./BookmarkList.svelte";
  import SavedSearchesPage from "./SavedSearchesPage.svelte";
  import SettingsPage from "./SettingsPage.svelte";
  import TagsPage from "./TagsPage.svelte";
  import SidePanel from "./SidePanel.svelte";
  import TopBar from "./TopBar.svelte";
  import ShortcutsModal from "./ShortcutsModal.svelte";

  let view = $state(readView(new URL(window.location.href)));
  let pageSize = $state(30);
  let tags = $state<Tag[]>([]);
  let savedSearches = $state<SavedSearch[]>([]);
  let items = $state<Bookmark[]>([]);
  let count = $state(0);
  let status = $state<"loading" | "ready" | "error">("loading");
  let error = $state("");
  let notice = $state("");
  let reloadKey = $state(0);
  let panelOpen = $state(false);
  // Bulk select mode on the list, toggled from the top bar.
  let selecting = $state(false);
  $effect(() => {
    if (view.pageName !== "list" || view.read !== null) selecting = false;
  });
  let sideStatus = $state<"loading" | "ready" | "error">("loading");
  let settingsReady = $state(false);
  let helpOpen = $state(false);
  let helpTrigger: HTMLElement | null = null;

  function openHelp() {
    if (helpOpen) return;
    const a = document.activeElement;
    helpTrigger = a instanceof HTMLElement && a !== document.body ? a : null;
    helpOpen = true;
  }
  async function closeHelp() {
    helpOpen = false;
    const t = helpTrigger;
    helpTrigger = null;
    await tick();
    requestAnimationFrame(() => {
      if (modalOpen(view)) return;
      (t?.isConnected
        ? t
        : (document.querySelector<HTMLElement>(".list-area") ??
          document.querySelector<HTMLElement>("main"))
      )?.focus();
    });
  }

  let deletedId: number | null = null;
  const modalOpen = (v: typeof view) =>
    v.view !== null || v.edit !== null || v.read !== null || !!v.create;
  // After a modal closes (Esc, backdrop, Close, Back), focus the control that
  // opened it: the row's View/Edit button or the + Add button. After a delete
  // the row is going away, so use the list.
  async function restoreFocus(selector: string | null, deleted = false) {
    await tick();
    requestAnimationFrame(() => {
      if (modalOpen(view)) return;
      const target =
        (deleted || !selector ? null : document.querySelector<HTMLElement>(selector)) ??
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
    const prev = view;
    view = readView(new URL(window.location.href));
    // Leaving the reader for the list: focus the list area.
    if (prev.read !== null && view.read === null && !modalOpen(view)) {
      void restoreFocus(null);
      return;
    }
    if (!modalOpen(prev)) return;
    if (!modalOpen(view)) {
      if (prev.view !== null) {
        const deleted = deletedId === prev.view;
        deletedId = null;
        void restoreFocus(`[data-view-trigger="${prev.view}"]`, deleted);
      } else if (prev.edit !== null) {
        void restoreFocus(`[data-edit-trigger="${prev.edit}"]`);
      } else if (prev.create) {
        void restoreFocus(".add-trigger");
      }
    } else if (
      (prev.view !== null && view.view !== null && prev.view !== view.view) ||
      (view.view !== null && prev.view === null) ||
      (view.edit !== null && prev.edit === null) ||
      (view.create && !prev.create)
    ) {
      // Modal swapped for another (details -> edit, add -> details, ...).
      void keepFocusInDialog();
    }
  }
  function applySettings(s: Settings) {
    const n = s.ui.items_per_page;
    if (typeof n === "number" && n > 0) pageSize = n;
  }
  onMount(() => {
    window.addEventListener("viewchange", sync);
    window.addEventListener("popstate", sync);
    api
      .settings()
      .then(applySettings)
      .catch(() => {})
      .finally(() => (settingsReady = true));
    return () => {
      window.removeEventListener("viewchange", sync);
      window.removeEventListener("popstate", sync);
    };
  });

  async function loadSide() {
    try {
      [tags, savedSearches] = await Promise.all([api.tags(), api.savedSearches()]);
      sideStatus = "ready";
    } catch {
      // The side panel is secondary, but say so rather than show an empty library.
      sideStatus = "error";
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
        if (ctrl.signal.aborted) return;
        if (res.results.length === 0 && res.count > 0 && view.page > 1) {
          // Deleted the last row of the last page: step back.
          navigate({ page: Math.max(1, Math.ceil(res.count / pageSize)) }, { replace: true });
          return;
        }
        items = res.results;
        count = res.count;
        error = "";
        notice = "";
        status = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        if (e instanceof ApiError && e.status === 404 && view.saved) {
          // The active saved search was deleted elsewhere: drop it and say so.
          notice = "That saved search no longer exists.";
          navigate({ saved: null }, { replace: true });
          return;
        }
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

<TopBar
  {view}
  {panelOpen}
  {selecting}
  ontogglepanel={() => (panelOpen = !panelOpen)}
  onselect={() => (selecting = !selecting)}
  onhelp={openHelp}
/>
{#if view.read !== null}
  {#key view.read}
    <ReaderPage id={view.read} />
  {/key}
{:else if view.pageName === "list"}
  <div class="layout">
    <main class="main" tabindex="-1">
      {#if notice}<p class="notice" role="status">{notice}</p>{/if}
      <BookmarkList
        bind:selectMode={selecting}
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
        onhelp={openHelp}
        suggestions={tags.map((t) => t.name)}
      />
    </main>
    <SidePanel {view} {tags} {savedSearches} status={sideStatus} open={panelOpen} ontag={onTag} />
  </div>
{:else if view.pageName === "tags"}
  <TagsPage onchanged={reload} />
{:else if view.pageName === "saved"}
  <SavedSearchesPage onchanged={reload} />
{:else}
  <SettingsPage section={view.section} onsettings={applySettings} onchanged={reload} />
{/if}

{#if view.view !== null}
  {#key view.view}
    <DetailsModal id={view.view} onchanged={reload} ondeleted={(id) => (deletedId = id)} />
  {/key}
{/if}

{#if view.create}
  <EditModal
    mode="create"
    initialURL={view.create.url}
    initialTitle={view.create.title}
    onchanged={reload}
  />
{:else if view.edit !== null}
  {#key view.edit}
    <EditModal mode="edit" id={view.edit} onchanged={reload} />
  {/key}
{/if}

{#if helpOpen}
  <ShortcutsModal onclose={closeHelp} />
{/if}
