<script lang="ts">
  import { SearchIcon } from "@lucide/svelte";
  import { Button } from "@ui-sdk/svelte/button";
  import { Input, InputGroup } from "@ui-sdk/svelte/input";
  import TagButton from "$lib/components/TagButton.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
  } from "./design-system/custom/table";
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
    void focusSel(".rename-row input");
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
      <InputGroup>
        <SearchIcon aria-hidden="true" />
        <Input
          type="search"
          aria-label="Filter tags"
          placeholder="Filter tags"
          bind:value={filter}
        />
      </InputGroup>
    </div>
    {#if actionError}<p class="notice error" role="alert">{actionError}</p>{/if}
    {#if shown.length === 0}
      <p class="muted">No tags match “{filter}”.</p>
    {:else}
      <Table class="tags-table" aria-label="Tags">
        <colgroup>
          <col />
          <col class="col-num" />
          <col class="col-actions" />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead align="end">Bookmarks</TableHead>
            <TableHead align="end"><span class="sr-only">Actions</span></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {#each shown as t (t.id)}
            <TableRow>
              <TableCell>
                {#if renaming?.id === t.id}
                  <div class="rename-row">
                    <Input
                      aria-label="New name for {t.name}"
                      value={renaming.text}
                      disabled={busy}
                      onValueChange={(v) => {
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
                  <TagButton
                    name={t.name}
                    onclick={() => navigate({ pageName: "list", q: `#${t.name}`, saved: null })}
                  />
                {/if}
              </TableCell>
              <TableCell align="end">{t.count}</TableCell>
              <TableCell align="end">
                <span class="row-actions">
                  {#if renaming?.id === t.id}
                    <Button data-variant="solid" disabled={busy} onclick={() => commitRename(t)}>
                      {merge ? "Merge" : "Save"}
                    </Button>
                    <Button disabled={busy} onclick={cancelRename}>Cancel</Button>
                  {:else}
                    <Button
                      data-variant="ghost"
                      class="rename-{t.id}"
                      onclick={() => startRename(t)}>Rename</Button
                    >
                    <Button
                      data-variant="ghost"
                      class="row-delete delete-{t.id}"
                      disabled={busy}
                      onclick={() => startDelete(t)}>Delete</Button
                    >
                  {/if}
                </span>
              </TableCell>
            </TableRow>
          {/each}
        </TableBody>
      </Table>
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
