<script lang="ts">
  import {
    Button,
    CopyButton,
    EmptyState,
    SettingsLayout,
    SettingsSection,
    Spinner,
    TextInput,
    Toggle,
    formatRelativeTime,
    formatTimestamp,
  } from "@kenn-io/kit-ui";
  import { tick } from "svelte";
  import { api, type ApiToken, type CreatedApiToken, type Settings } from "./lib/api.ts";
  import {
    clampItemsPerPage,
    mergeSettings,
    type SettingsPatch,
    DEFAULT_ITEMS_PER_PAGE,
    importSummary,
  } from "./lib/pages.ts";
  import { navigate, type SettingsSection as SectionId } from "./lib/view.ts";
  import PageHeader from "./PageHeader.svelte";

  interface Props {
    /** The section in the URL (/settings/<section>). */
    section: SectionId;
    onsettings: (s: Settings) => void;
    onchanged: () => void;
  }
  let { section, onsettings, onchanged }: Props = $props();

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

  // ---- API tokens ----
  const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e));
  let tokens = $state<ApiToken[] | null>(null);
  let tokensError = $state("");
  let tokenName = $state("");
  let tokenBusy = $state(false);
  let tokenActionError = $state("");
  let newToken = $state<CreatedApiToken | null>(null);
  let deletingToken = $state<number | null>(null);

  // Only the newest list request may update the list, so a slow first load
  // can't overwrite the list fetched after a create.
  let tokensSeq = 0;
  async function loadTokens() {
    const seq = ++tokensSeq;
    try {
      const list = await api.tokens();
      if (seq === tokensSeq) {
        tokens = list;
        tokensError = "";
      }
    } catch (e) {
      if (seq === tokensSeq) tokensError = errorText(e);
    }
  }
  // Load the tokens the first time the API category is shown.
  let tokensRequested = false;
  $effect(() => {
    if (section !== "api" || tokensRequested) return;
    tokensRequested = true;
    void loadTokens();
  });
  function retryTokens() {
    tokensError = "";
    tokens = null;
    void loadTokens();
  }

  async function focusSel(sel: string) {
    await tick();
    document.querySelector<HTMLElement>(sel)?.focus();
  }

  async function createToken() {
    const name = tokenName.trim();
    if (!name || tokenBusy) return;
    tokenBusy = true;
    tokenActionError = "";
    deletingToken = null;
    try {
      const created = await api.createToken(name);
      newToken = created;
      tokenName = "";
      void focusSel(".new-token-copy");
      await loadTokens();
    } catch (e) {
      tokenActionError = errorText(e);
    } finally {
      tokenBusy = false;
    }
  }
  function dismissNewToken() {
    newToken = null;
    void focusSel("#token-name");
  }
  function startDeleteToken(t: ApiToken) {
    tokenActionError = "";
    deletingToken = t.id;
    void focusSel(".token-confirm-cancel");
  }
  function cancelDeleteToken() {
    const id = deletingToken;
    deletingToken = null;
    if (id !== null) void focusSel(`.token-delete-${id}`);
  }
  async function deleteToken(t: ApiToken) {
    if (tokenBusy) return;
    tokenBusy = true;
    tokenActionError = "";
    try {
      await api.deleteToken(t.id);
      deletingToken = null;
      tokens = (tokens ?? []).filter((x) => x.id !== t.id);
      if (newToken?.id === t.id) newToken = null;
      void focusSel("#token-name");
    } catch (e) {
      tokenActionError = errorText(e);
    } finally {
      tokenBusy = false;
    }
  }
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
    <SettingsLayout
      {categories}
      bind:active={() => section, (id) => navigate({ section: id as SectionId })}
      title=""
    >
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
                API docs: <a href="/api/docs" target="_blank" rel="noreferrer">/api/docs</a>
              </p>
              <p class="small with-copy">
                MCP: <a class="mono" href="/mcp">{mcpURL}</a>
                <CopyButton text={mcpURL} ariaLabel="Copy MCP URL" title="Copy MCP URL" />
              </p>
            </div>

            {#if newToken}
              <div class="setting stacked new-token">
                <span class="label">New token “{newToken.name}”</span>
                <div class="new-token-value">
                  <code class="mono">{newToken.token}</code>
                  <CopyButton
                    class="new-token-copy"
                    text={newToken.token}
                    ariaLabel="Copy token"
                    title="Copy token"
                  />
                </div>
                <p class="small" role="status">Copy this token now — it won't be shown again.</p>
                <div>
                  <Button size="sm" onclick={dismissNewToken}>Done</Button>
                </div>
              </div>
            {/if}

            <div class="setting stacked">
              <label for="token-name">Create token</label>
              <form
                class="token-create"
                onsubmit={(e) => {
                  e.preventDefault();
                  void createToken();
                }}
              >
                <TextInput
                  id="token-name"
                  class="token-name-input"
                  placeholder="Name, e.g. Chrome extension"
                  bind:value={tokenName}
                  disabled={tokenBusy}
                  autocomplete="off"
                />
                <Button
                  type="submit"
                  tone="info"
                  surface="solid"
                  disabled={tokenBusy || !tokenName.trim()}>Create token</Button
                >
              </form>
              {#if tokenActionError}<p class="notice error" role="alert">{tokenActionError}</p>{/if}
            </div>

            <div class="setting stacked">
              <span class="label">Tokens</span>
              {#if tokensError}
                <p class="notice error" role="alert">{tokensError}</p>
                <div><Button size="sm" onclick={retryTokens}>Retry</Button></div>
              {:else if tokens === null}
                <span class="small muted"
                  ><Spinner size={12} label="Loading tokens" /> Loading…</span
                >
              {:else if tokens.length === 0}
                <p class="small muted">No tokens yet.</p>
              {:else}
                <ul class="saved-rows token-rows">
                  {#each tokens as t (t.id)}
                    <li class="saved-row">
                      <div class="saved-main">
                        <span class="token-name">{t.name}</span>
                        <span class="small muted">
                          Created
                          <span title={formatTimestamp(t.created_at)}
                            >{formatRelativeTime(t.created_at)}</span
                          >
                          · last used
                          {#if t.last_used_at}<span title={formatTimestamp(t.last_used_at)}
                              >{formatRelativeTime(t.last_used_at)}</span
                            >{:else}never{/if}
                        </span>
                      </div>
                      <span class="row-actions">
                        {#if deletingToken === t.id}
                          <span class="small confirm-inline"
                            >Delete {t.name}? Apps using it stop working.</span
                          >
                          <Button
                            class="token-confirm-cancel"
                            size="sm"
                            disabled={tokenBusy}
                            onclick={cancelDeleteToken}>Cancel</Button
                          >
                          <Button
                            size="sm"
                            tone="danger"
                            surface="solid"
                            disabled={tokenBusy}
                            onclick={() => deleteToken(t)}>Delete</Button
                          >
                        {:else}
                          <Button
                            size="sm"
                            tone="danger"
                            class="token-delete-{t.id}"
                            disabled={tokenBusy}
                            onclick={() => startDeleteToken(t)}>Delete</Button
                          >
                        {/if}
                      </span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </SettingsSection>
        {/if}
      {/snippet}
    </SettingsLayout>
  {/if}
</main>
