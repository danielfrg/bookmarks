<script lang="ts">
  import { Button, Markdown, Modal, Spinner, Toggle } from "@kenn-io/kit-ui";
  import { formatRelativeTime, formatTimestamp } from "@kenn-io/kit-ui";
  import FileIcon from "@lucide/svelte/icons/file";
  import FileTextIcon from "@lucide/svelte/icons/file-text";
  import FileTypeIcon from "@lucide/svelte/icons/file-type";
  import { tick } from "svelte";
  import { api, ApiError, type Archive, type Bookmark, type ReaderPage } from "./lib/api.ts";
  import { displayTitle, formatBytes, hostOf } from "./lib/query.ts";
  import { navigate } from "./lib/view.ts";

  interface Props {
    id: number;
    onchanged: () => void;
    ondeleted?: (id: number) => void;
  }
  let { id, onchanged, ondeleted }: Props = $props();

  type ReaderInfo = Pick<ReaderPage, "status" | "error" | "word_count" | "extracted_at">;

  let phase = $state<"loading" | "ready" | "notfound" | "error">("loading");
  let loadError = $state("");
  let bm = $state<Bookmark | null>(null);
  let archives = $state<Archive[]>([]);
  let reader = $state<ReaderInfo | null>(null);
  let unread = $state(false);
  let actionError = $state("");
  let busy = $state(false);
  let uploading = $state(false);
  let notice = $state("");
  let readerNotice = $state("");
  let refreshing = $state(false);
  let confirmArchiveId = $state<number | null>(null);
  let confirmingDelete = $state(false);
  let fileInput = $state<HTMLInputElement>();

  const SOURCE_LABEL: Record<Archive["source"], string> = {
    extension: "Full page snapshot",
    server: "Basic capture",
    upload: "Uploaded file",
    import: "Imported snapshot",
  };

  // Moves focus to the visible inline-confirm Cancel button.
  async function focusCancel(scope: Element | null) {
    await tick();
    scope?.querySelector<HTMLElement>(".confirm-cancel")?.focus();
  }

  const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

  async function fetchReader(): Promise<ReaderInfo | null> {
    try {
      return await api.reader(id, 0, 1);
    } catch {
      return null;
    }
  }

  async function load(signal: AbortSignal) {
    phase = "loading";
    try {
      const [b, a, r] = await Promise.all([api.get(id, signal), api.archives(id), fetchReader()]);
      if (signal.aborted) return;
      bm = b;
      unread = b.unread;
      archives = a;
      reader = r;
      phase = "ready";
    } catch (e) {
      if (signal.aborted) return;
      if (e instanceof ApiError && e.status === 404) phase = "notfound";
      else {
        loadError = message(e);
        phase = "error";
      }
    }
  }

  let attempt = $state(0);
  $effect(() => {
    void attempt;
    const ctrl = new AbortController();
    void load(ctrl.signal);
    return () => ctrl.abort();
  });

  function close() {
    navigate({ view: null });
  }
  function leaveTo(patch: Parameters<typeof navigate>[0]) {
    navigate({ ...patch, view: null });
  }

  /** Runs a mutation: shows errors inline, tells the list to refresh on success. */
  async function run(fn: () => Promise<void>): Promise<boolean> {
    busy = true;
    actionError = "";
    try {
      await fn();
      onchanged();
      return true;
    } catch (e) {
      actionError = message(e);
      return false;
    } finally {
      busy = false;
    }
  }

  async function refreshArchives() {
    archives = await api.archives(id);
  }

  async function toggleUnread(v: boolean) {
    const ok = await run(async () => {
      bm = await api.update(id, { unread: v });
    });
    if (!ok) unread = !v;
  }
  async function toggleArchived() {
    if (!bm) return;
    const next = !bm.is_archived;
    await run(async () => {
      bm = await api.update(id, { is_archived: next });
      unread = bm.unread;
    });
  }
  async function deleteBookmark() {
    const ok = await run(async () => {
      await api.remove(id);
    });
    if (ok) {
      ondeleted?.(id);
      close();
    } else confirmingDelete = false;
  }

  async function deleteArchive(a: Archive) {
    const ok = await run(async () => {
      await api.deleteArchive(a.id);
      confirmArchiveId = null;
      await refreshArchives();
    });
    if (!ok) confirmArchiveId = null;
  }
  async function capture() {
    notice = "";
    const ok = await run(() => api.capture(id));
    if (ok) notice = "Capture queued";
  }
  async function extract() {
    readerNotice = "";
    const ok = await run(() => api.extract(id));
    if (ok) readerNotice = "Re-extract queued";
  }
  async function onFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    uploading = true;
    notice = "";
    await run(async () => {
      await api.uploadArchive(id, file);
      await refreshArchives();
    });
    uploading = false;
  }
  async function refreshReader() {
    refreshing = true;
    actionError = "";
    try {
      const [r, a, b] = await Promise.all([fetchReader(), api.archives(id), api.get(id)]);
      reader = r;
      archives = a;
      bm = b;
      readerNotice = "";
    } catch (e) {
      actionError = message(e);
    } finally {
      refreshing = false;
    }
  }
</script>

{#snippet archiveIcon(type: string)}
  {#if type.includes("html")}
    <FileTextIcon size="16" aria-hidden="true" />
  {:else if type.includes("pdf")}
    <FileTypeIcon size="16" aria-hidden="true" />
  {:else}
    <FileIcon size="16" aria-hidden="true" />
  {/if}
{/snippet}

<Modal
  title={bm ? displayTitle(bm) : phase === "notfound" ? "Bookmark not found" : "Bookmark"}
  ariaLabel={bm ? displayTitle(bm) : "Bookmark details"}
  width="600px"
  maxWidth="min(600px, calc(100vw - 32px))"
  onclose={close}
>
  {#if phase === "loading"}
    <div class="center"><Spinner label="Loading bookmark" /></div>
  {:else if phase === "notfound"}
    <p class="detail-empty">This bookmark does not exist or was deleted.</p>
  {:else if phase === "error"}
    <p class="notice error" role="alert">{loadError}</p>
    <Button onclick={() => attempt++}>Retry</Button>
  {:else if bm}
    <div class="details">
      {#if actionError}<p class="notice error" role="alert">{actionError}</p>{/if}

      <div class="d-url">
        <a class="mono" href={bm.url} target="_blank" rel="noopener noreferrer">{bm.url}</a>
        {#if hostOf(bm.url)}<span class="muted small">{hostOf(bm.url)}</span>{/if}
      </div>

      {#if bm.tags.length > 0}
        <section>
          <h3 class="d-label">Tags</h3>
          <div class="d-tags">
            {#each bm.tags as tag (tag)}<span class="tag-static">#{tag}</span>{/each}
          </div>
        </section>
      {/if}

      {#if bm.description}
        <section>
          <h3 class="d-label">Description</h3>
          <p class="d-text">{bm.description}</p>
        </section>
      {/if}

      <section>
        <h3 class="d-label">Archive</h3>
        {#if archives.length === 0}
          <p class="detail-empty">No archive yet</p>
        {:else}
          <ul class="d-box">
            {#each archives as a (a.id)}
              <li class="asset">
                <span class="asset-icon">{@render archiveIcon(a.content_type)}</span>
                <div class="asset-name">
                  {SOURCE_LABEL[a.source] ?? a.display_name}
                  <span class="muted small"
                    >· {formatTimestamp(a.created_at)} · {formatBytes(a.file_size)}</span
                  >
                </div>
                {#if confirmArchiveId === a.id}
                  <span class="small">Delete?</span>
                  <button
                    type="button"
                    class="lnk danger"
                    disabled={busy}
                    onclick={() => deleteArchive(a)}>Yes, delete</button
                  >
                  <button
                    type="button"
                    class="lnk confirm-cancel"
                    onclick={() => (confirmArchiveId = null)}>Cancel</button
                  >
                {:else}
                  <a class="lnk" href={a.view_url} target="_blank" rel="noopener noreferrer"
                    >Open<span class="sr-only"> {SOURCE_LABEL[a.source] ?? a.display_name}</span></a
                  >
                  <a class="lnk" href={a.download_url} download
                    >Download<span class="sr-only">
                      {SOURCE_LABEL[a.source] ?? a.display_name}</span
                    ></a
                  >
                  <button
                    type="button"
                    class="lnk danger"
                    disabled={busy}
                    onclick={(e) => {
                      const row = e.currentTarget.closest("li");
                      confirmArchiveId = a.id;
                      void focusCancel(row);
                    }}
                    >Delete<span class="sr-only">
                      {SOURCE_LABEL[a.source] ?? a.display_name}</span
                    ></button
                  >
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
        <div class="d-actions">
          <button type="button" class="lnk" disabled={busy} onclick={capture}
            >Capture again on server</button
          >
          <button type="button" class="lnk" disabled={busy} onclick={() => fileInput?.click()}>
            {#if uploading}<Spinner size={12} label="Uploading" />{/if}Upload file
          </button>
          <input type="file" hidden bind:this={fileInput} onchange={onFile} />
          {#if bm.web_archive_url}
            <a class="lnk" href={bm.web_archive_url} target="_blank" rel="noopener noreferrer"
              >Wayback Machine ↗</a
            >
          {/if}
          {#if notice}<span class="small muted" role="status">{notice}</span>{/if}
        </div>
      </section>

      <section>
        <h3 class="d-label">Reader text</h3>
        <div class="d-box reader-box">
          <div class="reader-line">
            {#if reader === null}
              <span class="muted">Status unavailable</span>
            {:else if reader.status === "ok"}
              <span
                >{reader.word_count.toLocaleString()} words · extracted
                {formatRelativeTime(reader.extracted_at)}</span
              >
            {:else if reader.status === "failed"}
              <span class="reader-fail">{reader.error || "Extraction failed"}</span>
            {:else}
              <span class="muted">Not extracted yet</span>
            {/if}
          </div>
          {#if reader?.status === "ok"}
            <button type="button" class="lnk" onclick={() => leaveTo({ read: id })}>Read</button>
          {/if}
          <button
            type="button"
            class="lnk"
            disabled={busy || archives.length === 0}
            title={archives.length === 0 ? "Needs an archive first" : undefined}
            onclick={extract}>Re-extract</button
          >
          <button type="button" class="lnk" disabled={refreshing} onclick={refreshReader}
            >Refresh</button
          >
        </div>
        {#if readerNotice}<p class="small muted" role="status">{readerNotice}</p>{/if}
      </section>

      <section>
        <h3 class="d-label">Notes</h3>
        {#if bm.notes.trim()}
          <div class="d-box notes"><Markdown source={bm.notes} /></div>
        {:else}
          <p class="detail-empty">No notes</p>
        {/if}
      </section>

      <div class="d-meta">
        <Toggle bind:checked={unread} label="Unread" disabled={busy} onchange={toggleUnread} />
        <span class="muted small"
          >Added {formatTimestamp(bm.date_added)} · modified {formatTimestamp(
            bm.date_modified,
          )}</span
        >
      </div>
    </div>
  {/if}

  {#snippet footer()}
    {#if phase === "ready" && bm}
      {#if confirmingDelete}
        <span class="small footer-note">Delete this bookmark and its archives?</span>
        <Button class="confirm-cancel" onclick={() => (confirmingDelete = false)}>Cancel</Button>
        <Button tone="danger" surface="solid" disabled={busy} onclick={deleteBookmark}>
          Delete
        </Button>
      {:else}
        <Button onclick={() => leaveTo({ edit: id })}>Edit</Button>
        <Button disabled={busy} onclick={toggleArchived}>
          {bm.is_archived ? "Unarchive" : "Archive"}
        </Button>
        <span class="grow"></span>
        <Button
          tone="danger"
          disabled={busy}
          onclick={(e) => {
            const footer =
              (e.currentTarget as HTMLElement | null)?.closest(".kit-modal-footer") ?? null;
            confirmingDelete = true;
            void focusCancel(footer);
          }}
        >
          Delete
        </Button>
      {/if}
    {:else}
      <Button onclick={close}>Close</Button>
    {/if}
  {/snippet}
</Modal>
