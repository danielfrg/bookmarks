<script lang="ts">
  import { Button, Markdown, Spinner, formatRelativeTime } from "@kenn-io/kit-ui";
  import ArrowLeftIcon from "@lucide/svelte/icons/arrow-left";
  import { onMount, tick } from "svelte";
  import { api, ApiError, type Archive, type Bookmark, type ReaderPage } from "./lib/api.ts";
  import { displayTitle, hostOf } from "./lib/query.ts";
  import { READER_PAGE_CHARS, joinChunks } from "./lib/reader.ts";
  import { navigate, readView, writeView } from "./lib/view.ts";

  interface Props {
    id: number;
  }
  let { id }: Props = $props();

  let phase = $state<"loading" | "ready" | "notfound" | "error">("loading");
  let loadError = $state("");
  let bm = $state<Bookmark | null>(null);
  let archives = $state<Archive[]>([]);
  let archivesError = $state(false);
  let page = $state<ReaderPage | null>(null);
  let text = $state("");
  let more = $state(false);
  let moreError = $state("");
  let nextOffset = $state<number | null>(null);
  let extractQueued = $state(false);
  let extractError = $state("");
  let busy = $state(false);
  let attempt = $state(0);
  let root = $state<HTMLElement>();
  let body = $state<HTMLElement>();

  const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
  // Archives come newest first; the newest is the one worth linking.
  const archive = $derived(archives.length ? archives[0] : null);

  async function load(signal: AbortSignal) {
    phase = "loading";
    extractQueued = false;
    extractError = "";
    try {
      let failed = false;
      const [b, a, r] = await Promise.all([
        api.get(id, signal),
        api.archives(id).catch(() => {
          failed = true;
          return [] as Archive[];
        }),
        api.reader(id, 0, READER_PAGE_CHARS),
      ]);
      if (signal.aborted) return;
      bm = b;
      archives = a;
      archivesError = failed;
      page = r;
      text = r.markdown;
      nextOffset = r.next_offset;
      moreError = "";
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
  $effect(() => {
    void attempt;
    const ctrl = new AbortController();
    void load(ctrl.signal);
    return () => ctrl.abort();
  });

  // Continue reading: aborted if the page goes away mid-request.
  let moreCtrl: AbortController | null = null;
  async function continueReading() {
    if (nextOffset === null || more) return;
    more = true;
    moreError = "";
    moreCtrl = new AbortController();
    const ctrl = moreCtrl;
    try {
      const r = await api.reader(id, nextOffset, READER_PAGE_CHARS);
      if (ctrl.signal.aborted) return;
      text = joinChunks(text, r.markdown);
      nextOffset = r.next_offset;
    } catch (e) {
      if (ctrl.signal.aborted) return;
      moreError = message(e);
    } finally {
      if (!ctrl.signal.aborted) more = false;
    }
  }

  async function reextract() {
    busy = true;
    extractError = "";
    try {
      await api.extract(id);
      extractQueued = true;
    } catch (e) {
      extractError = message(e);
    } finally {
      busy = false;
    }
  }

  // Document title while reading.
  $effect(() => {
    document.title = bm ? `${displayTitle(bm)} · Bookmarks` : "Bookmarks";
  });
  // Links in the article open in a new tab. The Markdown component renders
  // asynchronously, so watch for (re)rendered content.
  $effect(() => {
    const el = body;
    if (!el) return;
    const fix = () => {
      for (const a of el.querySelectorAll<HTMLAnchorElement>("a[href]")) {
        a.target = "_blank";
        a.rel = "noopener noreferrer";
      }
    };
    fix();
    const mo = new MutationObserver(fix);
    mo.observe(el, { childList: true, subtree: true });
    return () => mo.disconnect();
  });

  onMount(() => {
    void tick().then(() => root?.focus());
    return () => {
      document.title = "Bookmarks";
      moreCtrl?.abort();
    };
  });

  // The list view (query, filter, page) the reader was opened from.
  const backHref = $derived.by(() => {
    const v = readView(new URL(window.location.href));
    return window.location.pathname + writeView({ ...v, read: null });
  });
  const back = (e: MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    navigate({ read: null });
  };
</script>

<main class="main reader-page" tabindex="-1" bind:this={root}>
  <div class="reader-col">
    <a class="reader-back" href={backHref} onclick={back}>
      <ArrowLeftIcon size={14} aria-hidden="true" /> Bookmarks
    </a>

    {#if phase === "loading"}
      <div class="center" role="status" aria-label="Loading"><Spinner /></div>
    {:else if phase === "notfound"}
      <div class="reader-state">
        <h2>Bookmark not found</h2>
        <p class="muted">It may have been deleted.</p>
      </div>
    {:else if phase === "error" || !bm || !page}
      <div class="reader-state">
        <p class="notice error" role="alert">{loadError}</p>
        <Button onclick={() => attempt++}>Retry</Button>
      </div>
    {:else}
      <header class="reader-head">
        <h1>{displayTitle(bm)}</h1>
        <p class="reader-meta">
          <a class="mono" href={bm.url} target="_blank" rel="noopener noreferrer">
            {hostOf(bm.url) || bm.url}
          </a>
          <a href={bm.url} target="_blank" rel="noopener noreferrer">Original ↗</a>
          {#if archive}
            <a href={archive.view_url} target="_blank" rel="noopener noreferrer">Archive ↗</a>
          {/if}
          {#if page.status === "ok"}
            <span class="muted">
              {page.word_count.toLocaleString()} words
              {#if page.extracted_at}· extracted {formatRelativeTime(page.extracted_at)}{/if}
            </span>
          {/if}
        </p>
      </header>

      {#if page.status === "ok"}
        <article class="reader" bind:this={body}>
          <Markdown source={text} />
        </article>
        {#if nextOffset !== null}
          <div class="reader-more">
            {#if moreError}<p class="notice error" role="alert">{moreError}</p>{/if}
            <Button onclick={continueReading} disabled={more}>
              {more ? "Loading…" : "Continue reading"}
            </Button>
          </div>
        {/if}
      {:else}
        <div class="reader-state">
          {#if page.status === "failed"}
            <h2>Extraction failed</h2>
            {#if page.error}<p class="notice error">{page.error}</p>{/if}
          {:else}
            <h2>No reader text yet</h2>
          {/if}
          {#if extractQueued}
            <p role="status">Re-extract queued</p>
            <Button onclick={() => attempt++}>Refresh</Button>
          {:else}
            <Button onclick={reextract} disabled={busy || (!archive && !archivesError)}
              >Re-extract</Button
            >
            {#if archivesError}
              <p class="muted">Could not check for an archive; Re-extract may still work.</p>
            {:else if !archive}
              <p class="muted">Reader text is extracted from an archive; this bookmark has none.</p>
            {/if}
            {#if extractError}<p class="notice error" role="alert">{extractError}</p>{/if}
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</main>
