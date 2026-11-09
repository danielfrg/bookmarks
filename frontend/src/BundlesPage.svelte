<script lang="ts">
  import { Button, EmptyState, IconButton, Spinner } from "@kenn-io/kit-ui";
  import ArrowDownIcon from "@lucide/svelte/icons/arrow-down";
  import ArrowUpIcon from "@lucide/svelte/icons/arrow-up";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import { tick } from "svelte";
  import { api, type Bundle } from "./lib/api.ts";
  import { byOrder, moveItem } from "./lib/pages.ts";
  import { navigate } from "./lib/view.ts";
  import BundleEditor from "./BundleEditor.svelte";
  import PageHeader from "./PageHeader.svelte";

  interface Props {
    onchanged: () => void;
  }
  let { onchanged }: Props = $props();

  let bundles = $state<Bundle[]>([]);
  let tagNames = $state<string[]>([]);
  let status = $state<"loading" | "ready" | "error">("loading");
  let loadError = $state("");
  let actionError = $state("");
  let busy = $state(false);
  let deleting = $state<number | null>(null);
  // undefined: closed; null: new; Bundle: editing.
  let editing = $state<Bundle | null | undefined>(undefined);
  let trigger: HTMLElement | null = null;

  let attempt = $state(0);
  $effect(() => {
    void attempt;
    const ctrl = new AbortController();
    status = "loading";
    api
      .bundles()
      .then((b) => {
        if (ctrl.signal.aborted) return;
        bundles = byOrder(b);
        status = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        loadError = e instanceof Error ? e.message : String(e);
        status = "error";
      });
    return () => ctrl.abort();
  });
  $effect(() => {
    let live = true;
    api
      .tags()
      .then((t) => live && (tagNames = t.map((x) => x.name)))
      .catch(() => {});
    return () => (live = false);
  });

  const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
  async function focusSel(sel: string) {
    await tick();
    document.querySelector<HTMLElement>(sel)?.focus();
  }

  async function refresh() {
    bundles = byOrder(await api.bundles());
  }
  async function move(index: number, dir: -1 | 1) {
    if (busy) return;
    const ids = bundles.map((b) => b.id);
    const next = moveItem(ids, index, dir);
    busy = true;
    actionError = "";
    try {
      await api.reorderBundles(next);
      await refresh();
      onchanged();
      const moved = ids[index];
      const same = dir === -1 ? "up" : "down";
      const other = dir === -1 ? "down" : "up";
      void focusSel(
        `.move-${moved}-${same}:not(:disabled), .move-${moved}-${other}:not(:disabled)`,
      );
    } catch (e) {
      actionError = message(e);
    } finally {
      busy = false;
    }
  }
  function startDelete(b: Bundle) {
    actionError = "";
    deleting = b.id;
    void focusSel(".confirm-cancel");
  }
  function cancelDelete() {
    const id = deleting;
    deleting = null;
    if (id !== null) void focusSel(`.delete-${id}`);
  }
  async function doDelete(b: Bundle) {
    if (busy) return;
    busy = true;
    actionError = "";
    try {
      await api.deleteBundle(b.id);
      deleting = null;
      bundles = bundles.filter((x) => x.id !== b.id);
      onchanged();
    } catch (e) {
      actionError = message(e);
    } finally {
      busy = false;
    }
  }
  function open(b: Bundle | null, e: Event) {
    trigger = e.currentTarget as HTMLElement;
    editing = b;
  }
  async function closeEditor() {
    editing = undefined;
    await tick();
    requestAnimationFrame(() => trigger?.focus());
  }
  async function saved() {
    onchanged();
    try {
      await refresh();
    } catch (e) {
      actionError = message(e);
    }
    void closeEditor();
  }
  function describe(b: Bundle): string {
    const parts: string[] = [];
    if (b.search) parts.push(b.search);
    if (b.any_tags.length) parts.push(`any: ${b.any_tags.map((t) => "#" + t).join(" ")}`);
    if (b.all_tags.length) parts.push(`all: ${b.all_tags.map((t) => "#" + t).join(" ")}`);
    if (b.excluded_tags.length) parts.push(`not: ${b.excluded_tags.map((t) => "#" + t).join(" ")}`);
    if (b.filter_unread !== "off") parts.push(b.filter_unread === "yes" ? "unread" : "read");
    return parts.join(" · ");
  }
</script>

<main class="main page">
  <PageHeader title="Bundles">
    {#if status === "ready"}
      <Button tone="info" surface="solid" onclick={(e) => open(null, e)}
        ><PlusIcon size={14} aria-hidden="true" /> New bundle</Button
      >
    {/if}
  </PageHeader>
  {#if status === "loading"}
    <div class="center"><Spinner label="Loading bundles" /></div>
  {:else if status === "error"}
    <EmptyState title="Could not load bundles" description={loadError}>
      <Button onclick={() => attempt++}>Retry</Button>
    </EmptyState>
  {:else}
    {#if actionError}<p class="notice error" role="alert">{actionError}</p>{/if}
    {#if bundles.length === 0}
      <EmptyState
        title="No bundles yet"
        description="A bundle saves a search with tag rules so you can reopen it in one click."
      >
        <Button tone="info" surface="solid" onclick={(e) => open(null, e)}>New bundle</Button>
      </EmptyState>
    {:else}
      <ul class="bundle-rows">
        {#each bundles as b, i (b.id)}
          <li class="bundle-row">
            <span class="reorder">
              <IconButton
                class="move-{b.id}-up"
                ariaLabel="Move {b.name} up"
                disabled={busy || i === 0}
                onclick={() => move(i, -1)}><ArrowUpIcon size={14} aria-hidden="true" /></IconButton
              >
              <IconButton
                class="move-{b.id}-down"
                ariaLabel="Move {b.name} down"
                disabled={busy || i === bundles.length - 1}
                onclick={() => move(i, 1)}
                ><ArrowDownIcon size={14} aria-hidden="true" /></IconButton
              >
            </span>
            <div class="bundle-main">
              <button
                type="button"
                class="name-link"
                onclick={() => navigate({ pageName: "list", bundle: b.id, q: "" })}>{b.name}</button
              >
              {#if describe(b)}<span class="small muted mono bundle-desc">{describe(b)}</span>{/if}
            </div>
            <span class="row-actions">
              {#if deleting === b.id}
                <span class="small confirm-inline"
                  >Delete “{b.name}”? Bookmarks are not affected.</span
                >
                <Button class="confirm-cancel" size="sm" disabled={busy} onclick={cancelDelete}
                  >Cancel</Button
                >
                <Button
                  size="sm"
                  tone="danger"
                  surface="solid"
                  disabled={busy}
                  onclick={() => doDelete(b)}>Delete</Button
                >
              {:else}
                <Button size="sm" class="edit-{b.id}" onclick={(e) => open(b, e)}>Edit</Button>
                <Button size="sm" tone="danger" class="delete-{b.id}" onclick={() => startDelete(b)}
                  >Delete</Button
                >
              {/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

{#if editing !== undefined}
  <BundleEditor bundle={editing} suggestions={tagNames} onsaved={saved} onclose={closeEditor} />
{/if}
