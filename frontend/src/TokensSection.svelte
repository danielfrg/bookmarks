<script lang="ts">
  // Settings → API → Tokens: list, create (shown once) and revoke API tokens.
  import {
    Button,
    CopyButton,
    Menu,
    MenuContent,
    MenuItem,
    MenuTrigger,
    Modal,
    SettingsSection,
    Spinner,
    TextInput,
    formatRelativeTime,
    formatTimestamp,
  } from "@kenn-io/kit-ui";
  import EllipsisIcon from "@lucide/svelte/icons/ellipsis";
  import KeyIcon from "@lucide/svelte/icons/key";
  import TrashIcon from "@lucide/svelte/icons/trash";
  import { onMount, tick } from "svelte";
  import { api, type ApiToken, type CreatedApiToken } from "./lib/api.ts";
  import ConfirmDialog from "./ConfirmDialog.svelte";

  const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e));

  let tokens = $state<ApiToken[] | null>(null);
  let loadError = $state("");
  let busy = $state(false);

  // Create dialog: "form" asks for a name, "created" shows the token once and
  // closes only with Done (not Escape or the overlay), so it isn't lost by accident.
  let dialog = $state<"form" | "created" | null>(null);
  let name = $state("");
  let createError = $state("");
  let created = $state<CreatedApiToken | null>(null);

  let revoking = $state<ApiToken | null>(null);
  let revokeError = $state("");

  // Only the newest list request may update the list, so a slow first load
  // can't overwrite the list fetched after a create.
  let seq = 0;
  async function load() {
    const mine = ++seq;
    try {
      const list = await api.tokens();
      if (mine === seq) {
        tokens = list;
        loadError = "";
      }
    } catch (e) {
      if (mine === seq) loadError = errorText(e);
    }
  }
  onMount(() => void load());

  async function focusSel(sel: string) {
    await tick();
    document.querySelector<HTMLElement>(sel)?.focus();
  }

  function openCreate() {
    name = "";
    createError = "";
    created = null;
    dialog = "form";
    void focusSel("#token-name");
  }
  function closeDialog() {
    if (busy) return;
    dialog = null;
    created = null;
    void focusSel(".new-token-trigger");
  }
  async function create() {
    const n = name.trim();
    if (!n || busy) return;
    busy = true;
    createError = "";
    try {
      created = await api.createToken(n);
      dialog = "created";
      void focusSel(".created-token-copy");
      await load();
    } catch (e) {
      createError = errorText(e);
    } finally {
      busy = false;
    }
  }

  function startRevoke(t: ApiToken) {
    revokeError = "";
    revoking = t;
  }
  async function revoke() {
    const t = revoking;
    if (!t || busy) return;
    busy = true;
    revokeError = "";
    try {
      await api.deleteToken(t.id);
      revoking = null;
      tokens = (tokens ?? []).filter((x) => x.id !== t.id);
      void focusSel(".new-token-trigger");
    } catch (e) {
      revokeError = errorText(e);
    } finally {
      busy = false;
    }
  }

  const countLabel = $derived(
    tokens === null || tokens.length === 0
      ? "No tokens created"
      : `${tokens.length} ${tokens.length === 1 ? "token" : "tokens"}`,
  );
</script>

<SettingsSection
  title="Tokens"
  description="Let the browser extension, scripts and agents use the API and MCP."
>
  <!-- Direct children of the section body: it pads each and draws the dividers. -->
  <div class="tokens-head">
    <span class:muted={!tokens?.length}>{tokens === null && !loadError ? "" : countLabel}</span>
    <Button size="sm" class="ghost new-token-trigger" onclick={openCreate}>New token</Button>
  </div>
  {#if loadError}
    <div class="tokens-note">
      <p class="notice error" role="alert">{loadError}</p>
      <Button size="sm" onclick={() => void load()}>Retry</Button>
    </div>
  {:else if tokens === null}
    <div class="tokens-note"><Spinner size={12} label="Loading tokens" /></div>
  {:else}
    {#each tokens as t (t.id)}
      <div class="token-row">
        <span class="token-tile" aria-hidden="true"><KeyIcon size={15} /></span>
        <div class="token-main">
          <span class="token-title">{t.name}</span>
          <span class="token-meta">
            Created <span title={formatTimestamp(t.created_at)}
              >{formatRelativeTime(t.created_at)}</span
            >
            ·
            {#if t.last_used_at}Last used <span title={formatTimestamp(t.last_used_at)}
                >{formatRelativeTime(t.last_used_at)}</span
              >{:else}Never used{/if}
          </span>
        </div>
        <Menu align="end">
          <MenuTrigger ariaLabel="Actions for {t.name}" class="ghost token-menu-trigger">
            <EllipsisIcon size={16} aria-hidden="true" />
          </MenuTrigger>
          <MenuContent ariaLabel="Actions for {t.name}">
            <MenuItem tone="danger" onselect={() => startRevoke(t)}>
              {#snippet icon()}<TrashIcon size={14} />{/snippet}
              Revoke token
            </MenuItem>
          </MenuContent>
        </Menu>
      </div>
    {/each}
  {/if}
</SettingsSection>

{#if dialog}
  <Modal
    ariaLabel={dialog === "form" ? "Create token" : "Token created"}
    closable={false}
    maxWidth="32rem"
    closeOnOverlayClick={dialog === "form"}
    onclose={() => dialog === "form" && closeDialog()}
  >
    {#if dialog === "form"}
      <form
        class="token-dialog"
        onsubmit={(e) => {
          e.preventDefault();
          void create();
        }}
      >
        <h2 class="token-dialog-title">Create token</h2>
        <label class="token-field">
          <span>Token name</span>
          <TextInput
            id="token-name"
            block
            placeholder="A descriptive name, e.g. Chrome extension"
            autocomplete="off"
            disabled={busy}
            bind:value={name}
          />
        </label>
        {#if createError}<p class="notice error" role="alert">{createError}</p>{/if}
      </form>
    {:else if created}
      <div class="token-dialog">
        <h2 class="token-dialog-title">{created.name}</h2>
        <div class="token-field">
          <span>Token</span>
          <div class="token-value">
            <code class="mono">{created.token}</code>
            <CopyButton
              class="created-token-copy"
              text={created.token}
              ariaLabel="Copy token"
              title="Copy token"
            />
          </div>
          <p class="token-hint">This token is shown only once. Copy it now.</p>
        </div>
      </div>
    {/if}
    {#snippet footer()}
      {#if dialog === "form"}
        <Button disabled={busy} onclick={closeDialog}>Cancel</Button>
        <Button
          tone="info"
          surface="solid"
          disabled={busy || !name.trim()}
          onclick={() => void create()}>Create</Button
        >
      {:else}
        <Button tone="info" surface="solid" onclick={closeDialog}>Done</Button>
      {/if}
    {/snippet}
  </Modal>
{/if}

{#if revoking}
  <ConfirmDialog
    title="Revoke “{revoking.name}”?"
    message="Apps using this token lose access right away. This can't be undone."
    confirmLabel="Revoke"
    {busy}
    error={revokeError}
    onconfirm={revoke}
    oncancel={() => {
      const t = revoking;
      revoking = null;
      if (t) void focusSel(".new-token-trigger");
    }}
  />
{/if}

<style>
  .tokens-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .tokens-note {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  .token-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  /* The row's "…" (a ghost MenuTrigger) is a round icon button. */
  .token-row :global(.token-menu-trigger) {
    width: 2rem;
    height: 2rem;
    min-height: 0;
    padding: 0;
    border-radius: 999px;
    color: var(--text-muted);
  }
  .token-tile {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 2rem;
    height: 2rem;
    border-radius: var(--radius-md, 8px);
    background: var(--bg-inset);
    color: var(--text-muted);
  }
  .token-main {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }
  .token-title {
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }
  .token-meta {
    color: var(--text-muted);
    font-size: var(--font-size-sm, 0.8125rem);
  }
  .muted {
    color: var(--text-muted);
  }
  .token-dialog {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .token-dialog-title {
    margin: 0;
    font-size: var(--font-size-md, 0.9375rem);
    font-weight: 600;
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }
  .token-field {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }
  .token-field > span {
    font-size: var(--font-size-sm, 0.8125rem);
    color: var(--text-secondary);
  }
  .token-value {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .token-value code {
    flex: 1;
    min-width: 0;
    color: var(--accent-green);
    overflow-wrap: anywhere;
    user-select: all;
  }
  .token-hint {
    margin: 0;
    color: var(--text-muted);
    font-size: var(--font-size-sm, 0.8125rem);
  }
</style>
