<script lang="ts">
  import { Button, EmptyState, Spinner, TextInput } from "@kenn-io/kit-ui";
  import { tick } from "svelte";
  import { api, type Tag } from "./lib/api.ts";
  import { filterTags, normalizeTagName, wouldMerge } from "./lib/pages.ts";
  import { navigate } from "./lib/view.ts";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import PageHeader from "./PageHeader.svelte";

  interface Props {
    onchanged: () => void;
  }
  let { onchanged }: Props = $props();

  let tags = $state<Tag[]>([]);
  let status = $state<"loading" | "ready" | "error">("loading");
  let loadError = $state("");
  let filter = $state("");
  let actionError = $state("");
  let busy = $state(false);

  let renaming = $state<{ id: number; text: string; confirmed: boolean } | null>(null);
  /** The tag awaiting delete confirmation. */
  let deleting = $state<Tag | null>(null);
  let deleteError = $state("");

  const shown = $derived(filterTags(tags, filter));
  const merge = $derived(renaming ? wouldMerge(tags, renaming.id, renaming.text) : null);

  let attempt = $state(0);
  $effect(() => {
    void attempt;
    const ctrl = new AbortController();
    status = "loading";
    api
      .tags()
      .then((t) => {
        if (ctrl.signal.aborted) return;
        tags = t;
        status = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        loadError = e instanceof Error ? e.message : String(e);
        status = "error";
      });
    return () => ctrl.abort();
  });

  async function focusSel(sel: string) {
    await tick();
    document.querySelector<HTMLElement>(sel)?.focus();
  }
  function startRename(t: Tag) {
    deleting = null;
    actionError = "";
    renaming = { id: t.id, text: t.name, confirmed: false };
  }
  function cancelRename() {
    const id = renaming?.id;
    renaming = null;
    actionError = "";
    if (id !== undefined) void focusSel(`.rename-${id}`);
  }
  async function commitRename(t: Tag) {
    if (!renaming || busy) return;
    const name = normalizeTagName(renaming.text);
    if (!name) return;
    if (name === t.name) return cancelRename();
    if (merge && !renaming.confirmed) {
      renaming.confirmed = true;
      return;
    }
    busy = true;
    actionError = "";
    try {
      await api.renameTag(t.id, name);
      renaming = null;
      onchanged();
    } catch (e) {
      actionError = e instanceof Error ? e.message : String(e);
      busy = false;
      return;
    }
    // Quiet refresh: merges change other rows' counts. A failure here is not a rename failure.
    try {
      tags = await api.tags();
    } catch {
      actionError = "Renamed, but the list could not be refreshed";
    }
    busy = false;
    // After a merge the renamed row is gone; fall back to the filter input.
    await tick();
    (
      document.querySelector<HTMLElement>(`.rename-${t.id}`) ??
      document.querySelector<HTMLElement>(".page-tools input")
    )?.focus();
  }
  function onRenameKey(e: KeyboardEvent, t: Tag) {
    if (e.isComposing) return;
    if (e.key === "Enter") {
      e.preventDefault();
      void commitRename(t);
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      cancelRename();
    }
  }
  // An unused tag is deleted right away; one on bookmarks asks first.
  function startDelete(t: Tag) {
    renaming = null;
    actionError = "";
    deleteError = "";
    if (t.count === 0) void doDelete(t);
    else deleting = t;
  }
  function cancelDelete() {
    const t = deleting;
    deleting = null;
    if (t) void focusSel(`.delete-${t.id}`);
  }
  async function doDelete(t: Tag) {
    if (busy) return;
    busy = true;
    actionError = "";
    deleteError = "";
    try {
      await api.deleteTag(t.id);
      deleting = null;
      tags = tags.filter((x) => x.id !== t.id);
      onchanged();
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      if (deleting) deleteError = message;
      else actionError = message;
    } finally {
      busy = false;
    }
  }
</script>

<main class="main page">
  <PageHeader title="Tags" />
  {#if status === "loading"}
    <div class="center"><Spinner label="Loading tags" /></div>
  {:else if status === "error"}
    <EmptyState title="Could not load tags" description={loadError}>
      <Button onclick={() => attempt++}>Retry</Button>
    </EmptyState>
  {:else if tags.length === 0}
    <EmptyState title="No tags yet" description="Tags you add to bookmarks show up here." />
  {:else}
    <div class="page-tools">
      <TextInput
        type="search"
        ariaLabel="Filter tags"
        placeholder="Filter tags"
        bind:value={filter}
      />
    </div>
    {#if actionError}<p class="notice error" role="alert">{actionError}</p>{/if}
    {#if shown.length === 0}
      <p class="muted">No tags match “{filter}”.</p>
    {:else}
      <table class="data-table tags-table">
        <colgroup>
          <col />
          <col class="col-num" />
          <col class="col-actions" />
        </colgroup>
        <thead>
          <tr
            ><th scope="col">Name</th><th scope="col" class="num">Bookmarks</th><th scope="col"
              ><span class="sr-only">Actions</span></th
            ></tr
          >
        </thead>
        <tbody>
          {#each shown as t (t.id)}
            <tr>
              <td>
                {#if renaming?.id === t.id}
                  <div class="rename-row">
                    <TextInput
                      autofocus
                      size="sm"
                      ariaLabel="New name for {t.name}"
                      value={renaming.text}
                      disabled={busy}
                      oninput={(v) => {
                        if (renaming) renaming = { ...renaming, text: v, confirmed: false };
                      }}
                      onkeydown={(e) => onRenameKey(e, t)}
                    />
                    {#if merge}
                      <span class="small merge-note" role="status">
                        Will merge into #{merge.name}{renaming.confirmed
                          ? " — press Enter again to confirm"
                          : ""}
                      </span>
                    {/if}
                  </div>
                {:else}
                  <button
                    type="button"
                    class="name-link mono"
                    onclick={() => navigate({ pageName: "list", q: `#${t.name}`, saved: null })}
                    >#{t.name}</button
                  >
                {/if}
              </td>
              <td class="num">{t.count}</td>
              <td class="row-actions">
                {#if renaming?.id === t.id}
                  <Button
                    size="sm"
                    tone="info"
                    surface="solid"
                    disabled={busy}
                    onclick={() => commitRename(t)}
                  >
                    {merge ? "Merge" : "Save"}
                  </Button>
                  <Button size="sm" disabled={busy} onclick={cancelRename}>Cancel</Button>
                {:else}
                  <Button size="sm" class="ghost rename-{t.id}" onclick={() => startRename(t)}
                    >Rename</Button
                  >
                  <Button
                    size="sm"
                    tone="danger"
                    class="ghost delete-{t.id}"
                    disabled={busy}
                    onclick={() => startDelete(t)}>Delete</Button
                  >
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  {/if}
</main>

{#if deleting}
  <ConfirmDialog
    title="Delete #{deleting.name}?"
    message="It will be removed from {deleting.count} {deleting.count === 1
      ? 'bookmark'
      : 'bookmarks'}. The bookmarks themselves are kept."
    {busy}
    error={deleteError}
    onconfirm={() => deleting && doDelete(deleting)}
    oncancel={cancelDelete}
  />
{/if}
