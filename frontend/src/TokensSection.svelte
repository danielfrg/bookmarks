<script lang="ts">
  // Settings → API → Tokens: list, create (shown once) and revoke API tokens.
  import { EllipsisIcon, KeyIcon, TrashIcon } from "@lucide/svelte";
  import { Button } from "@ui-sdk/svelte/button";
  import { Field, FieldLabel } from "@ui-sdk/svelte/field";
  import { Input } from "@ui-sdk/svelte/input";
  import { Menu, MenuContent, MenuItem, MenuTrigger } from "@ui-sdk/svelte/menu";
  import CopyButton from "$lib/components/CopyButton.svelte";
  import Modal from "$lib/components/Modal.svelte";
  import SettingsSection from "$lib/components/SettingsSection.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import { formatRelativeTime, formatTimestamp } from "./lib/format.ts";
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
    void focusSel(".token-dialog input");
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
  <div class="tokens-head">
    <span class:muted={!tokens?.length}>{tokens === null && !loadError ? "" : countLabel}</span>
    <Button data-variant="ghost" class="new-token-trigger" onclick={openCreate}>New token</Button>
  </div>
  {#if loadError}
    <div class="tokens-note">
      <p class="notice error" role="alert">{loadError}</p>
      <Button onclick={() => void load()}>Retry</Button>
    </div>
  {:else if tokens === null}
    <div class="tokens-note"><Spinner size={12} label="Loading tokens" /></div>
  {:else}
    {#each tokens as t (t.id)}
      <div class="token-row">
        <span class="token-tile" aria-hidden="true"><KeyIcon /></span>
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
        <Menu>
          <MenuTrigger data-variant="ghost" aria-label="Actions for {t.name}">
            <EllipsisIcon aria-hidden="true" />
          </MenuTrigger>
          <MenuContent aria-label="Actions for {t.name}">
            <MenuItem value="revoke" onSelect={() => startRevoke(t)}>
              <TrashIcon aria-hidden="true" /> Revoke token
            </MenuItem>
          </MenuContent>
        </Menu>
      </div>
    {/each}
  {/if}
</SettingsSection>

{#if dialog === "form"}
  <Modal title="Create token" size="sm" onclose={closeDialog}>
    <form
      id="create-token"
      class="token-dialog"
      onsubmit={(e) => {
        e.preventDefault();
        void create();
      }}
    >
      <Field name="token-name">
        <FieldLabel>Token name</FieldLabel>
        <Input
          bind:value={name}
          placeholder="A descriptive name, e.g. Chrome extension"
          autocomplete="off"
          disabled={busy}
        />
      </Field>
      {#if createError}<p class="notice error" role="alert">{createError}</p>{/if}
    </form>
    {#snippet footer()}
      <Button disabled={busy} onclick={closeDialog}>Cancel</Button>
      <Button data-variant="solid" disabled={busy || !name.trim()} onclick={() => void create()}
        >Create</Button
      >
    {/snippet}
  </Modal>
{:else if dialog === "created" && created}
  <!-- Closes only with Done (not Escape or the backdrop), so the one-time token isn't lost. -->
  <Modal title={created.name} size="sm" closable={false} onclose={closeDialog}>
    <div class="token-dialog">
      <span class="token-label">Token</span>
      <div class="token-value">
        <code class="mono">{created.token}</code>
        <CopyButton class="created-token-copy" text={created.token} label="Copy token" />
      </div>
      <p class="token-hint">This token is shown only once. Copy it now.</p>
    </div>
    {#snippet footer()}
      <Button data-variant="solid" onclick={closeDialog}>Done</Button>
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
