<script lang="ts">
  import {
    Button,
    CopyButton,
    EmptyState,
    SettingsLayout,
    SettingsSection,
    Spinner,
    Toggle,
  } from "@kenn-io/kit-ui";
  import { api, type Settings } from "./lib/api.ts";
  import {
    clampItemsPerPage,
    mergeSettings,
    type SettingsPatch,
    DEFAULT_ITEMS_PER_PAGE,
    importSummary,
  } from "./lib/pages.ts";
  import PageHeader from "./PageHeader.svelte";

  interface Props {
    onsettings: (s: Settings) => void;
    onchanged: () => void;
  }
  let { onsettings, onchanged }: Props = $props();

  const categories = [
    { id: "general", label: "General" },
    { id: "metadata", label: "Metadata" },
    { id: "auto-tagging", label: "Auto-tagging" },
    { id: "import-export", label: "Import & export" },
    { id: "api", label: "API" },
  ];

  let settings = $state<Settings | null>(null);
  let status = $state<"loading" | "ready" | "error">("loading");
  let loadError = $state("");
  let active = $state("general");

  let pending = $state(0);
  let queue: Promise<void> = Promise.resolve();
  let saved = $state(false);
  let saveError = $state("");
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  let rules = $state("");
  let perPage = $state(String(DEFAULT_ITEMS_PER_PAGE));

  let attempt = $state(0);
  $effect(() => {
    void attempt;
    const ctrl = new AbortController();
    status = "loading";
    api
      .settings()
      .then((s) => {
        if (ctrl.signal.aborted) return;
        settings = s;
        confirmed = s;
        sync(s, true);
        status = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        loadError = e instanceof Error ? e.message : String(e);
        status = "error";
      });
    return () => {
      ctrl.abort();
      clearTimeout(savedTimer);
    };
  });

  /** Copies settings into the controls; an input being edited keeps its text unless `force`. */
  function sync(s: Settings, force = false) {
    if (force || document.activeElement?.id !== "auto-tag-rules") rules = s.auto_tag_rules;
    if (force || document.activeElement?.id !== "items-per-page") {
      const n = s.ui.items_per_page;
      perPage = String(typeof n === "number" && n > 0 ? n : DEFAULT_ITEMS_PER_PAGE);
    }
  }

  // Last settings the server confirmed. Each save applies its own patch to this
  // at run time, so a failed save can never leak into a later one.
  let confirmed: Settings | null = null;

  /** Applies `patch` optimistically; saves run one at a time. A failure resyncs everything from the server. */
  function save(patch: SettingsPatch): Promise<void> {
    if (!settings) return Promise.resolve();
    settings = mergeSettings(settings, patch);
    pending++;
    saved = false;
    saveError = "";
    clearTimeout(savedTimer);
    queue = queue.then(async () => {
      try {
        const res = await api.putSettings(mergeSettings(confirmed ?? settings!, patch));
        confirmed = res;
        if (pending === 1) {
          settings = res;
          sync(res);
        }
        onsettings(res);
        if (pending === 1) {
          saved = true;
          savedTimer = setTimeout(() => (saved = false), 2000);
        }
      } catch (e) {
        saveError = e instanceof Error ? e.message : String(e);
        try {
          confirmed = await api.settings();
        } catch {
          // Keep the last confirmed settings.
        }
        if (confirmed) {
          settings = confirmed;
          sync(confirmed, true);
        }
      } finally {
        pending--;
      }
    });
    return queue;
  }
  const setFlag = (
    key: "favicons_enabled" | "previews_enabled" | "wayback_enabled" | "default_unread",
    v: boolean,
  ) => save({ [key]: v });
  function saveRules() {
    if (settings && rules !== settings.auto_tag_rules) void save({ auto_tag_rules: rules });
  }
  function savePerPage() {
    if (!settings) return;
    const n = clampItemsPerPage(perPage);
    perPage = String(n);
    if (n !== settings.ui.items_per_page) void save({ ui: { items_per_page: n } });
  }

  // ---- import ----
  let importing = $state(false);
  let importResult = $state("");
  let importError = $state("");
  let importWarning = $state("");
  async function onFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    importing = true;
    importResult = "";
    importWarning = "";
    importError = "";
    try {
      const r = await api.importNetscape(file);
      if (r.created + r.updated + r.skipped === 0)
        importWarning = "No bookmarks found in this file";
      else importResult = importSummary(r);
      onchanged();
    } catch (err) {
      importError = err instanceof Error ? err.message : String(err);
    } finally {
      importing = false;
      input.value = "";
    }
  }

  const mcpURL = `${window.location.origin}/mcp`;
</script>

<main class="main page settings-page">
  <PageHeader title="Settings">
    <span class="save-status small" role="status" aria-live="polite">
      {#if saved}Saved{/if}
    </span>
  </PageHeader>
  {#if status === "loading"}
    <div class="center"><Spinner label="Loading settings" /></div>
  {:else if status === "error" || !settings}
    <EmptyState title="Could not load settings" description={loadError}>
      <Button onclick={() => attempt++}>Retry</Button>
    </EmptyState>
  {:else}
    {#if saveError}<p class="notice error" role="alert">{saveError}</p>{/if}
    <SettingsLayout {categories} bind:active title="">
      {#snippet panel(id)}
        {#if id === "general"}
          <SettingsSection title="General">
            <div class="setting">
              <Toggle
                label="New bookmarks are unread by default"
                checked={settings!.default_unread}
                onchange={(v) => setFlag("default_unread", v)}
              />
            </div>
            <div class="setting">
              <label for="items-per-page">Items per page</label>
              <input
                id="items-per-page"
                class="num-input"
                type="number"
                min="10"
                max="100"
                step="1"
                value={perPage}
                oninput={(e) => (perPage = e.currentTarget.value)}
                onblur={savePerPage}
                onkeydown={(e) => e.key === "Enter" && savePerPage()}
              />
              <span class="small muted">Between 10 and 100.</span>
            </div>
          </SettingsSection>
        {:else if id === "metadata"}
          <SettingsSection
            title="Metadata"
            description="What the server fetches for each bookmark."
          >
            <div class="setting">
              <Toggle
                label="Download favicons"
                checked={settings!.favicons_enabled}
                onchange={(v) => setFlag("favicons_enabled", v)}
              />
            </div>
            <div class="setting">
              <Toggle
                label="Download preview images"
                checked={settings!.previews_enabled}
                onchange={(v) => setFlag("previews_enabled", v)}
              />
            </div>
            <div class="setting">
              <Toggle
                label="Look up Internet Archive (Wayback) snapshots"
                checked={settings!.wayback_enabled}
                onchange={(v) => setFlag("wayback_enabled", v)}
              />
            </div>
          </SettingsSection>
        {:else if id === "auto-tagging"}
          <SettingsSection
            title="Auto-tagging"
            description="Add tags to bookmarks automatically based on their URL."
          >
            <div class="setting stacked">
              <label for="auto-tag-rules">Rules</label>
              <textarea
                id="auto-tag-rules"
                class="field-area mono"
                rows="8"
                spellcheck="false"
                placeholder={"youtube.com video\nreddit.com/r/Music music reddit"}
                bind:value={rules}
                onblur={saveRules}></textarea>
              <p class="small muted">
                One rule per line: a URL pattern (host, optional path, query or #fragment) followed
                by the tags to add, for example <code class="mono"
                  >reddit.com/r/Music music reddit</code
                >. A host without a subdomain matches all subdomains; a path matches itself and its
                subpaths. No wildcards.
              </p>
              <div>
                <Button
                  tone="info"
                  surface="solid"
                  disabled={rules === settings?.auto_tag_rules}
                  onclick={saveRules}>Save rules</Button
                >
              </div>
            </div>
          </SettingsSection>
        {:else if id === "import-export"}
          <SettingsSection
            title="Import & export"
            description="Move bookmarks in or out as a Netscape HTML file."
          >
            <div class="setting stacked">
              <label for="import-file">Import</label>
              <input
                id="import-file"
                type="file"
                accept=".html,.htm"
                disabled={importing}
                onchange={onFile}
              />
              {#if importing}<span class="small muted" role="status"
                  ><Spinner size={12} label="Importing" /> Importing…</span
                >{/if}
              {#if importResult}<p class="small" role="status">{importResult}</p>{/if}
              {#if importWarning}<p class="small warn-note" role="status">{importWarning}</p>{/if}
              {#if importError}<p class="notice error" role="alert">{importError}</p>{/if}
            </div>
            <div class="setting stacked">
              <span class="label">Export</span>
              <p class="small">
                <a href={api.exportURL} download="bookmarks.html">Download all bookmarks (.html)</a>
              </p>
            </div>
          </SettingsSection>
        {:else if id === "api"}
          <SettingsSection title="API">
            <div class="setting stacked">
              <p class="small">
                The API token is set with <code class="mono">BOOKMARKS_API_TOKEN</code> on the server.
              </p>
              <p class="small">
                API docs: <a href="/api/docs" target="_blank" rel="noreferrer">/api/docs</a>
              </p>
              <p class="small">
                MCP: <a class="mono" href="/mcp">{mcpURL}</a>
                <CopyButton text={mcpURL} ariaLabel="Copy MCP URL" title="Copy MCP URL" />
              </p>
            </div>
          </SettingsSection>
        {/if}
      {/snippet}
    </SettingsLayout>
  {/if}
</main>
