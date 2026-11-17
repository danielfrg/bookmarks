<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";
  import { Field, FieldDescription, FieldLabel } from "@ui-sdk/svelte/field";
  import { FileInput, FileInputTrigger } from "@ui-sdk/svelte/file-input";
  import {
    NumberInput,
    NumberInputControl,
    NumberInputDecrement,
    NumberInputGroup,
    NumberInputIncrement,
  } from "@ui-sdk/svelte/number-input";
  import { MinusIcon, PlusIcon } from "@lucide/svelte";
  import { Tabs, TabsList, TabsPanel, TabsTab } from "@ui-sdk/svelte/tabs";
  import { Textarea } from "@ui-sdk/svelte/textarea";
  import CopyButton from "$lib/components/CopyButton.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import SettingsSection from "$lib/components/SettingsSection.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import SwitchField from "$lib/components/SwitchField.svelte";
  import { api, type Settings } from "../api.ts";
  import {
    clampItemsPerPage,
    mergeSettings,
    type SettingsPatch,
    DEFAULT_ITEMS_PER_PAGE,
    importSummary,
  } from "../pages.ts";
  import { navigate, type SettingsSection as SectionId } from "../view.ts";
  import TokensSection from "./TokensSection.svelte";
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
    const editing = (selector: string) => document.activeElement?.closest(selector) != null;
    if (force || !editing(".rules-field")) rules = s.auto_tag_rules;
    if (force || !editing(".per-page")) {
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
  async function onFile(files: File[]) {
    const file = files[0];
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
    <Tabs
      class="settings-layout"
      orientation="vertical"
      bind:value={() => section, (id) => id && navigate({ section: id as SectionId })}
    >
      <TabsList aria-label="Settings sections">
        {#each categories as c (c.id)}
          <TabsTab value={c.id}>{c.label}</TabsTab>
        {/each}
      </TabsList>
      <TabsPanel value="general" class="settings-panel">
        <SettingsSection title="General">
          <div class="setting">
            <SwitchField
              label="New bookmarks are unread by default"
              checked={settings.default_unread}
              onchange={(v) => setFlag("default_unread", v)}
            />
          </div>
          <div class="setting">
            <Field name="items-per-page" class="per-page">
              <FieldLabel>Items per page</FieldLabel>
              <NumberInput
                min={10}
                max={100}
                step={1}
                value={Number(perPage) || null}
                onValueChange={(v) => (perPage = v === null ? "" : String(v))}
                onValueCommit={savePerPage}
              >
                <NumberInputGroup>
                  <NumberInputDecrement aria-label="Fewer items"
                    ><MinusIcon aria-hidden="true" /></NumberInputDecrement
                  >
                  <NumberInputControl />
                  <NumberInputIncrement aria-label="More items"
                    ><PlusIcon aria-hidden="true" /></NumberInputIncrement
                  >
                </NumberInputGroup>
              </NumberInput>
              <FieldDescription>Between 10 and 100.</FieldDescription>
            </Field>
          </div>
        </SettingsSection>
      </TabsPanel>
      <TabsPanel value="metadata" class="settings-panel">
        <SettingsSection title="Metadata" description="What the server fetches for each bookmark.">
          <div class="setting">
            <SwitchField
              label="Download favicons"
              checked={settings.favicons_enabled}
              onchange={(v) => setFlag("favicons_enabled", v)}
            />
          </div>
          <div class="setting">
            <SwitchField
              label="Download preview images"
              checked={settings.previews_enabled}
              onchange={(v) => setFlag("previews_enabled", v)}
            />
          </div>
          <div class="setting">
            <SwitchField
              label="Look up Internet Archive (Wayback) snapshots"
              checked={settings.wayback_enabled}
              onchange={(v) => setFlag("wayback_enabled", v)}
            />
          </div>
        </SettingsSection>
      </TabsPanel>
      <TabsPanel value="auto-tagging" class="settings-panel">
        <SettingsSection
          title="Auto-tagging"
          description="Add tags to bookmarks automatically based on their URL."
        >
          <div class="setting stacked rules-field">
            <Field name="auto-tag-rules">
              <FieldLabel>Rules</FieldLabel>
              <Textarea
                class="mono"
                rows={8}
                spellcheck="false"
                placeholder={"youtube.com video\nreddit.com/r/Music music reddit"}
                bind:value={rules}
                onblur={saveRules}
              />
            </Field>
            <p class="small muted">
              One rule per line: a URL pattern (host, optional path, query or #fragment) followed by
              the tags to add, for example <code class="mono">reddit.com/r/Music music reddit</code
              >. A host without a subdomain matches all subdomains; a path matches itself and its
              subpaths. No wildcards.
            </p>
            <div>
              <Button
                data-variant="solid"
                disabled={rules === settings.auto_tag_rules}
                onclick={saveRules}>Save rules</Button
              >
            </div>
          </div>
        </SettingsSection>
      </TabsPanel>
      <TabsPanel value="import-export" class="settings-panel">
        <SettingsSection
          title="Import & export"
          description="Move bookmarks in or out as a Netscape HTML file."
        >
          <div class="setting stacked">
            <span class="label">Import</span>
            <FileInput value={[]} accept=".html,.htm" disabled={importing} onValueChange={onFile}>
              <FileInputTrigger>Choose an HTML file</FileInputTrigger>
            </FileInput>
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
      </TabsPanel>
      <TabsPanel value="api" class="settings-panel">
        <SettingsSection title="API">
          <div class="setting stacked">
            <p class="small">
              API docs: <a href="/api/docs" target="_blank" rel="noreferrer">/api/docs</a>
            </p>
            <p class="small with-copy">
              MCP: <a href="/mcp">{mcpURL}</a>
              <CopyButton text={mcpURL} label="Copy MCP URL" />
            </p>
          </div>
        </SettingsSection>
        <TokensSection />
      </TabsPanel>
    </Tabs>
  {/if}
</main>
