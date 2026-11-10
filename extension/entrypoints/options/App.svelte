<script lang="ts">
  import { onMount } from "svelte";
  import { Button, TextInput, ThemeToggle, Toggle } from "@kenn-io/kit-ui";
  import {
    CloudflareLoginError,
    TokenError,
    UnreachableError,
    client,
  } from "../../lib/api";
  import { loadSettings, normalizeServerURL, saveSettings } from "../../lib/settings";

  type Status = { tone: "ok" | "error"; text: string; link?: string };

  let loaded = $state(false);
  let serverURL = $state("");
  let token = $state("");
  let unreadByDefault = $state(false);
  let showToken = $state(false);
  let status = $state<Status | null>(null);
  let testing = $state(false);
  let defaultsError = $state("");
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  onMount(async () => {
    const s = await loadSettings();
    serverURL = s.serverURL;
    token = s.token;
    unreadByDefault = s.unreadByDefault;
    loaded = true;
  });

  function flash(next: Status, ms?: number) {
    clearTimeout(savedTimer);
    status = next;
    if (ms) savedTimer = setTimeout(() => (status = null), ms);
  }

  async function save(event: Event) {
    event.preventDefault();
    const url = normalizeServerURL(serverURL);
    if (!url) {
      flash({ tone: "error", text: "Enter an http(s) URL like https://bookmarks.example.com" });
      return;
    }
    if (!token.trim()) {
      flash({ tone: "error", text: "Enter your API token" });
      return;
    }
    try {
      await saveSettings({
        serverURL: url,
        token: token.trim(),
        unreadByDefault,
      });
    } catch (err) {
      flash({ tone: "error", text: err instanceof Error ? err.message : "Could not save" });
      return;
    }
    serverURL = url;
    token = token.trim();
    flash({ tone: "ok", text: "Saved" }, 2000);
  }

  async function test() {
    const url = normalizeServerURL(serverURL);
    if (!url) {
      flash({ tone: "error", text: "Enter an http(s) URL like https://bookmarks.example.com" });
      return;
    }
    testing = true;
    clearTimeout(savedTimer);
    status = null;
    try {
      await client({ serverURL: url, token: token.trim() }).ping();
      flash({ tone: "ok", text: "Connected" });
    } catch (err) {
      if (err instanceof TokenError) flash({ tone: "error", text: "Token rejected" });
      else if (err instanceof UnreachableError)
        flash({ tone: "error", text: "Can't reach the server" });
      else if (err instanceof CloudflareLoginError)
        flash({ tone: "error", text: "Log in to Cloudflare first", link: url });
      else flash({ tone: "error", text: err instanceof Error ? err.message : String(err) });
    } finally {
      testing = false;
    }
  }

  // Defaults save on change. Only the toggle is written so unsaved
  // server/token edits are not persisted by accident.
  async function saveDefaults() {
    defaultsError = "";
    try {
      const stored = await loadSettings();
      await saveSettings({ ...stored, unreadByDefault });
    } catch (err) {
      defaultsError = err instanceof Error ? err.message : "Could not save";
    }
  }
</script>

<main class="page">
  <header class="page-header">
    <h1>Bookmarks extension</h1>
    <ThemeToggle />
  </header>

  <form class="card" onsubmit={save} aria-labelledby="server-heading">
    <h2 id="server-heading">Server</h2>
    <div class="field">
      <label for="server-url">Server URL</label>
      <TextInput
        id="server-url"
        type="url"
        block
        autocomplete="off"
        placeholder="https://bookmarks.example.com"
        disabled={!loaded}
        bind:value={serverURL}
      />
    </div>
    <div class="field">
      <label for="api-token">API token</label>
      <div class="token-row">
        <TextInput
          id="api-token"
          type={showToken ? "text" : "password"}
          block
          autocomplete="off"
          disabled={!loaded}
          bind:value={token}
        />
        <Button onclick={() => (showToken = !showToken)} ariaLabel={showToken ? "Hide token" : "Show token"}>
          {showToken ? "Hide" : "Show"}
        </Button>
      </div>
    </div>
    <div class="actions">
      <Button type="submit" tone="info" surface="solid" disabled={!loaded}>Save</Button>
      <Button onclick={test} disabled={!loaded || testing}>Test connection</Button>
      <span class="status" data-tone={status?.tone} role="status" aria-live="polite">
        {#if status}
          {status.text}
          {#if status.link}
            <a href={status.link} target="_blank" rel="noreferrer">Open {status.link}</a>
          {/if}
        {/if}
      </span>
    </div>
  </form>

  <section class="card" aria-labelledby="defaults-heading">
    <h2 id="defaults-heading">Defaults</h2>
    <div class="setting">
      <Toggle
        label="Mark new bookmarks unread by default"
        disabled={!loaded}
        bind:checked={unreadByDefault}
        onchange={saveDefaults}
      />
    </div>
    {#if defaultsError}
      <p class="status" data-tone="error" role="alert">{defaultsError}</p>
    {/if}
  </section>
</main>
