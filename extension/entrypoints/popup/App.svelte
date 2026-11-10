<script lang="ts">
  import { onMount } from "svelte";
  import SettingsIcon from "@lucide/svelte/icons/settings";
  import { Button, Checkbox, IconButton, Spinner, TextInput, formatTimestamp } from "@kenn-io/kit-ui";
  import { client, type CheckResult } from "../../lib/api";
  import { capturability, type Capturability } from "../../lib/capturability";
  import PopupNotice from "../../lib/components/PopupNotice.svelte";
  import TagInput from "../../lib/components/TagInput.svelte";
  import {
    SAVE_PORT,
    type SaveEvent,
    type SavePortIn,
    type SavePortOut,
    type SaveRequest,
  } from "../../lib/messages";
  import {
    checkFailure,
    doneHandling,
    initialForm,
    isBookmarkableURL,
    progressText,
    saveErrorText,
    viewFromStatus,
    type CheckFailure,
    type FormNotice,
    type SaveActions,
  } from "../../lib/popup-state";
  import { isConfigured, loadSettings, type ExtSettings } from "../../lib/settings";
  import { commitTagText } from "../../lib/tags";

  type Screen = "loading" | "setup" | "unbookmarkable" | "check-error" | "form";
  type Stage = "saving" | "capturing" | "uploading";

  const CLOSE_AFTER_DONE_MS = 900;
  const emptyCheck: CheckResult = {
    bookmark: null,
    basic_capture_only: false,
    metadata: { title: "", description: "", preview_image_url: "" },
    auto_tags: [],
  };

  let screen = $state<Screen>("loading");
  let settings = $state<ExtSettings | null>(null);
  let tab = $state<{ id: number; url: string } | null>(null);
  let failure = $state<CheckFailure | null>(null);
  let checkWarning = $state(""); // the failure, shown under "Couldn't check this page"

  let title = $state("");
  let description = $state("");
  let notes = $state("");
  let tags = $state<string[]>([]);
  let tagText = $state("");
  let unread = $state(false);
  let actions = $state<SaveActions>({
    saveLabel: "Save",
    saveArchives: false,
    archiveLabel: null,
  });
  let archiveDisabledReason = $state<string | null>(null);
  let existing = $state(false);
  let notices = $state<FormNotice[]>([]);
  let existingDate = $state("");
  let allTags = $state<string[]>([]);

  let stage = $state<Stage | null>(null);
  let done = $state(false);
  let errorNotice = $state<{ title: string; message: string } | null>(null);
  let resumedSuccess = $state(false);

  let port: chrome.runtime.Port | undefined;
  let gotFirstEvent = false;
  let closeTimer: ReturnType<typeof setTimeout> | undefined;

  const saving = $derived(stage !== null);

  onMount(() => {
    void load();
    return () => {
      clearTimeout(closeTimer);
      port?.disconnect();
    };
  });

  function applyEvent(event: SaveEvent | null) {
    const first = !gotFirstEvent;
    gotFirstEvent = true;
    const v = viewFromStatus(event);
    switch (v.view) {
      case "form":
        return;
      case "progress":
        stage = v.stage;
        done = false;
        errorNotice = null;
        return;
      case "done":
        stage = null;
        errorNotice = null;
        if (doneHandling(first) === "notice") {
          // Already waiting when the popup opened: show it, keep the form usable.
          done = false;
          resumedSuccess = true;
        } else {
          done = true;
          closeTimer = setTimeout(() => window.close(), CLOSE_AFTER_DONE_MS);
        }
        return;
      case "error":
        stage = null;
        done = false;
        errorNotice = saveErrorText(
          v.error ?? { kind: "other", message: event?.message ?? "Save failed" },
          v.bookmarkId,
        );
        return;
    }
  }

  function connect(tabId: number) {
    port = chrome.runtime.connect({ name: SAVE_PORT });
    port.onMessage.addListener((m: SavePortOut) => {
      if (m.type === "event" && m.tabId === tabId) applyEvent(m.event);
    });
    const status: SavePortIn = { type: "status", tabId };
    port.postMessage(status);
  }

  async function load() {
    screen = "loading";
    failure = null;
    checkWarning = "";
    const s = await loadSettings();
    settings = s;
    if (!isConfigured(s)) {
      screen = "setup";
      return;
    }
    const [active] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (active?.id === undefined || !isBookmarkableURL(active.url)) {
      screen = "unbookmarkable";
      return;
    }
    tab = { id: active.id, url: active.url };
    if (!port) connect(active.id);

    const api = client(s);
    if (allTags.length === 0) {
      api.tags().then(
        (names) => (allTags = names),
        () => {},
      );
    }

    let check = emptyCheck;
    try {
      check = await api.check(active.url);
    } catch (err) {
      const f = checkFailure(err);
      if (f.blocking) {
        failure = f;
        screen = "check-error";
        return;
      }
      checkWarning = f.title;
    }

    let capture: Capturability;
    try {
      capture = capturability(active.url, {
        fileAccess: await chrome.extension.isAllowedFileSchemeAccess(),
      });
    } catch {
      capture = capturability(active.url);
    }
    const init = initialForm({ settings: s, check, tabTitle: active.title, capture });
    title = init.form.title;
    description = init.form.description;
    notes = init.form.notes;
    tags = init.form.tags;
    unread = init.form.unread;
    actions = init.actions;
    archiveDisabledReason = init.archiveDisabledReason;
    existing = init.existing !== null;
    existingDate = init.existing ? formatTimestamp(init.existing.date_added) : "";
    notices = init.notices;
    screen = "form";
  }

  /** Saves the form; `archive` also captures the page and uploads it. */
  function submit(archive: boolean) {
    if (screen !== "form" || saving || !tab || !port) return;
    const request: SaveRequest = {
      tabId: tab.id,
      url: tab.url,
      title: title.trim(),
      description,
      notes,
      tags: commitTagText(tags, tagText),
      unread,
      archive,
    };
    tags = request.tags;
    tagText = "";
    errorNotice = null;
    resumedSuccess = false;
    done = false;
    stage = "saving";
    const msg: SavePortIn = { type: "save", request };
    port.postMessage(msg);
  }

  function openOptions() {
    void chrome.runtime.openOptionsPage();
    window.close();
  }

  function openServer() {
    if (settings) void chrome.tabs.create({ url: settings.serverURL });
    window.close();
  }

  function onKeydown(e: KeyboardEvent) {
    // TagInput consumes Escape while its suggestions are open.
    if (e.key === "Escape" && !e.defaultPrevented) window.close();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="popup">
  <header class="popup-head">
    <span class="popup-title">Bookmarks</span>
    <IconButton ariaLabel="Open settings" size="sm" onclick={openOptions}>
      <SettingsIcon size={14} aria-hidden="true" />
    </IconButton>
  </header>

  {#if screen === "setup"}
    <div class="popup-state">
      <PopupNotice
        tone="info"
        title="Set up the extension"
        message="Add your server URL and API token."
      />
      <Button tone="info" surface="solid" onclick={openOptions}>Open settings</Button>
    </div>
  {:else if screen === "unbookmarkable"}
    <div class="popup-state">
      <PopupNotice
        tone="warning"
        title="Can't save this page"
        message="Only http and https pages can be bookmarked."
      />
    </div>
  {:else if screen === "check-error" && failure}
    <div class="popup-state">
      {#if failure.kind === "cloudflare"}
        <PopupNotice
          tone="error"
          title={failure.title}
          message="Log in, then reopen this popup."
        />
        <Button tone="info" surface="solid" onclick={openServer}>Log in to Cloudflare</Button>
      {:else if failure.kind === "token"}
        <PopupNotice tone="error" title={failure.title} message="The server rejected the API token." />
        <Button tone="info" surface="solid" onclick={openOptions}>Open settings</Button>
      {:else}
        <PopupNotice tone="error" title={failure.title} message="Check the server URL and your connection." />
        <Button onclick={() => void load()}>Retry</Button>
      {/if}
    </div>
  {:else if screen !== "form" && stage !== null}
    <div class="popup-state" role="status">
      <span class="progress-text">{progressText(stage)}</span>
      <div class="progress" aria-hidden="true"><i></i></div>
    </div>
  {:else if screen === "loading"}
    <div class="popup-state popup-loading" role="status">
      <Spinner size={14} label="Loading" />
    </div>
  {:else}
    <form
      class="popup-form"
      onsubmit={(e) => {
        e.preventDefault();
        submit(actions.saveArchives);
      }}
    >
      {#each notices as n (n.kind)}
        <div class="popup-notice">
          {#if n.kind === "already-saved"}
            <PopupNotice tone="warning" title="Already saved" message="Saved on {existingDate}." />
          {:else}
            <PopupNotice tone="warning" title="Basic capture only" message="Archive to save the full page." />
          {/if}
        </div>
      {/each}
      {#if checkWarning}
        <div class="popup-notice">
          <PopupNotice tone="warning" title="Couldn't check this page" message={checkWarning} />
        </div>
      {/if}
      {#if resumedSuccess}
        <div class="popup-notice"><PopupNotice tone="success" title="Saved ✓" /></div>
      {/if}
      {#if errorNotice}
        <div class="popup-notice">
          <PopupNotice tone="error" title={errorNotice.title} message={errorNotice.message} />
        </div>
      {/if}

      <div class="popup-body">
        <div class="field">
          <label for="p-url">URL</label>
          <TextInput id="p-url" class="url-input" block readonly value={tab?.url ?? ""} />
        </div>
        <div class="field">
          <label for="p-title">Title</label>
          <TextInput
            id="p-title"
            block
            autocomplete="off"
            disabled={saving}
            bind:value={title}
          />
        </div>
        <div class="field">
          <label for="p-tags">Tags</label>
          <TagInput
            id="p-tags"
            bind:tags
            bind:text={tagText}
            suggestions={allTags}
            disabled={saving}
          />
        </div>
        <div class="field">
          <label for="p-desc">Description</label>
          <textarea
            id="p-desc"
            class="field-area"
            rows="3"
            disabled={saving}
            bind:value={description}
          ></textarea>
        </div>
        <div class="field">
          <label for="p-notes">Notes</label>
          <textarea
            id="p-notes"
            class="field-area"
            rows="3"
            placeholder="Add notes…"
            disabled={saving}
            bind:value={notes}
          ></textarea>
        </div>
        <div class="checks">
          <Checkbox label="Mark unread" bind:checked={unread} disabled={saving} />
          {#if archiveDisabledReason}
            <p class="hint">{archiveDisabledReason}</p>
          {/if}
        </div>
      </div>

      <footer class="popup-foot">
        <div class="foot-status" role="status" aria-live="polite">
          {#if stage}
            <span class="progress-text">{progressText(stage)}</span>
            <div class="progress" aria-hidden="true"><i></i></div>
          {:else if done}
            <span class="progress-text ok">Saved ✓</span>
          {/if}
        </div>
        <div class="foot-actions">
          {#if actions.archiveLabel}
            <Button disabled={saving || done} onclick={() => submit(true)}>
              {actions.archiveLabel}
            </Button>
          {/if}
          <Button type="submit" tone="info" surface="solid" disabled={saving || done}>
            {actions.saveLabel}
          </Button>
        </div>
      </footer>
    </form>
  {/if}
</div>
