<script lang="ts">
  import { Button, Checkbox, FormField, Modal, Notice, Spinner } from "@kenn-io/kit-ui";
  import { formatTimestamp } from "@kenn-io/kit-ui";
  import { tick, untrack } from "svelte";
  import { api, ApiError, type Bookmark, type CheckResult, type SaveInput } from "./lib/api.ts";
  import { commitTagText } from "./lib/query.ts";
  import { navigate } from "./lib/view.ts";
  import TagInput from "./TagInput.svelte";

  interface Props {
    mode: "create" | "edit";
    /** Edit mode: the bookmark to edit. */
    id?: number;
    /** Create mode: prefill from the page URL (?new&url=&title=). */
    initialURL?: string;
    initialTitle?: string;
    onchanged: () => void;
  }
  let { mode, id, initialURL = "", initialTitle = "", onchanged }: Props = $props();

  const uid = $props.id();
  // Props that only seed the form; they do not change while the modal is open.
  const {
    mode: mode0,
    initialURL: url0,
    initialTitle: title0,
  } = untrack(() => ({ mode, initialURL, initialTitle }));
  const CHECK_DELAY_MS = 500;

  type Field = "url" | "title" | "description" | "notes" | "tags" | "unread";

  let phase = $state<"loading" | "ready" | "notfound" | "error">(
    mode0 === "edit" ? "loading" : "ready",
  );
  let loadError = $state("");
  let orig = $state<Bookmark | null>(null);

  let url = $state(url0);
  let title = $state(title0);
  let description = $state("");
  let notes = $state("");
  let tags = $state<string[]>([]);
  let tagText = $state("");
  let unread = $state(false);
  // Fields the user edited by hand; a URL check never overwrites these.
  const touched = new Set<Field>(title0 ? ["title"] : []);
  let touchedCount = $state(title0 ? 1 : 0);
  function touch(f: Field) {
    touched.add(f);
    touchedCount = touched.size;
  }

  let allTags = $state<string[]>([]);
  let existing = $state<Bookmark | null>(null);
  let checking = $state(false);
  let checkError = $state("");
  let saving = $state(false);
  let saveError = $state("");
  let confirmingDiscard = $state(false);

  const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
  const isHttpURL = (s: string) => {
    try {
      const u = new URL(s);
      return u.protocol === "http:" || u.protocol === "https:";
    } catch {
      return false;
    }
  };

  // Tag suggestions: fetched once per modal open; failure just disables autocomplete.
  $effect(() => {
    const ctrl = new AbortController();
    api
      .tags()
      .then((t) => {
        if (!ctrl.signal.aborted) allTags = t.map((x) => x.name);
      })
      .catch(() => {});
    return () => ctrl.abort();
  });

  // Create: the default for the Unread checkbox.
  $effect(() => {
    if (mode !== "create") return;
    let live = true;
    api
      .settings()
      .then((s) => {
        if (live && !touched.has("unread")) unread = s.default_unread;
      })
      .catch(() => {});
    return () => (live = false);
  });

  // Edit: load the bookmark.
  let attempt = $state(0);
  $effect(() => {
    if (mode !== "edit" || id === undefined) return;
    void attempt;
    const ctrl = new AbortController();
    phase = "loading";
    api
      .get(id, ctrl.signal)
      .then((b) => {
        orig = b;
        url = b.url;
        title = b.title;
        description = b.description;
        notes = b.notes;
        tags = [...b.tags];
        unread = b.unread;
        phase = "ready";
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        if (e instanceof ApiError && e.status === 404) phase = "notfound";
        else {
          loadError = message(e);
          phase = "error";
        }
      });
    return () => ctrl.abort();
  });

  // ---- URL check (create) ----
  let timer: ReturnType<typeof setTimeout> | undefined;
  let checkCtrl: AbortController | null = null;
  let checkPromise: Promise<void> = Promise.resolve();
  let inflightURL = "";
  let checkedURL = "";

  function applyCheck(r: CheckResult) {
    existing = r.bookmark;
    const src = r.bookmark;
    if (!touched.has("title")) title = src ? src.title : r.metadata.title;
    if (!touched.has("description")) description = src ? src.description : r.metadata.description;
    if (!touched.has("tags")) tags = src ? [...src.tags] : commitTagText([], r.auto_tags.join(" "));
    if (!touched.has("notes")) notes = src ? src.notes : "";
    if (src && !touched.has("unread")) unread = src.unread;
  }

  function runCheck(): Promise<void> {
    clearTimeout(timer);
    const u = url.trim();
    if (checking && u === inflightURL) return checkPromise;
    checkCtrl?.abort();
    checking = false;
    if (!isHttpURL(u) || u === checkedURL) return Promise.resolve();
    const ctrl = new AbortController();
    checkCtrl = ctrl;
    inflightURL = u;
    checking = true;
    checkError = "";
    checkPromise = api
      .check(u, ctrl.signal)
      .then((r) => {
        if (ctrl.signal.aborted) return;
        checkedURL = u;
        applyCheck(r);
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        checkedURL = u;
        existing = null;
        checkError = `Could not fetch page details: ${message(e)}`;
      })
      .finally(() => {
        if (checkCtrl === ctrl) checking = false;
      });
    return checkPromise;
  }

  function onURLInput(v: string) {
    url = v;
    touch("url");
    if (mode !== "create") return;
    if (v.trim() !== checkedURL) {
      existing = null;
      checkError = "";
    }
    clearTimeout(timer);
    timer = setTimeout(() => void runCheck(), CHECK_DELAY_MS);
  }
  function onURLBlur() {
    if (mode === "create") void runCheck();
  }

  // Prefilled URL from the page address.
  $effect(() => {
    untrack(() => {
      if (mode === "create" && isHttpURL(url.trim())) void runCheck();
    });
    return () => {
      clearTimeout(timer);
      checkCtrl?.abort();
    };
  });

  // ---- dirty / save ----
  function patchOf(): Partial<SaveInput> {
    const b = orig;
    if (!b) return {};
    const nextTags = commitTagText(tags, tagText);
    const p: Partial<SaveInput> = {};
    if (url.trim() !== b.url) p.url = url.trim();
    if (title !== b.title) p.title = title;
    if (description !== b.description) p.description = description;
    if (notes !== b.notes) p.notes = notes;
    if (unread !== b.unread) p.unread = unread;
    if (nextTags.join("\n") !== b.tags.join("\n")) p.tags = nextTags;
    return p;
  }
  const dirty = $derived(
    mode === "edit"
      ? Object.keys(patchOf()).length > 0
      : touchedCount > 0 || tagText.trim() !== "" || url.trim() !== initialURL.trim(),
  );
  const saveLabel = $derived(mode === "edit" || existing ? "Update" : "Save");
  const canSave = $derived(!saving && url.trim() !== "" && (mode === "create" || dirty));

  async function save() {
    if (!canSave) return;
    saveError = "";
    saving = true;
    try {
      if (mode === "create") {
        // Flush a pending check; if it reveals an existing bookmark, show the
        // notice first instead of silently updating it.
        const hadExisting = existing !== null;
        await runCheck();
        if (existing && !hadExisting) return;
        const input: SaveInput = {
          url: url.trim(),
          title,
          description,
          notes,
          tags: commitTagText(tags, tagText),
          unread,
        };
        const { bookmark } = await api.save(input, "server");
        onchanged();
        navigate({ create: null, view: bookmark.id });
      } else if (id !== undefined) {
        await api.update(id, patchOf());
        onchanged();
        navigate({ edit: null, view: id });
      }
    } catch (e) {
      saveError = message(e);
      // Inputs were disabled while saving, so focus was lost: return to the first field.
      await tick();
      document.getElementById(`${uid}-url`)?.focus();
    } finally {
      saving = false;
    }
  }

  // ---- closing ----
  async function focusKeepEditing() {
    await tick();
    document.querySelector<HTMLElement>(".kit-modal-footer .confirm-cancel")?.focus();
  }
  function close() {
    navigate(mode === "create" ? { create: null } : { edit: null });
  }
  function requestClose() {
    if (saving) return;
    if (confirmingDiscard) {
      confirmingDiscard = false;
      return;
    }
    if (dirty) {
      confirmingDiscard = true;
      void focusKeepEditing();
    } else close();
  }

  // Enter in a single-line input saves; the tag input and textareas handle it themselves.
  function onFieldsKeydown(e: KeyboardEvent) {
    if (e.key !== "Enter" || e.isComposing || e.defaultPrevented) return;
    const t = e.target as HTMLElement;
    if (t.tagName !== "INPUT" || !["text", "url"].includes((t as HTMLInputElement).type)) return;
    if (t.closest(".tag-input")) return;
    e.preventDefault();
    void save();
  }
</script>

<Modal
  title={mode === "create"
    ? "Add bookmark"
    : phase === "notfound"
      ? "Bookmark not found"
      : "Edit bookmark"}
  ariaLabel={mode === "create" ? "Add bookmark" : "Edit bookmark"}
  width="560px"
  maxWidth="min(560px, calc(100vw - 32px))"
  onclose={requestClose}
>
  {#if phase === "loading"}
    <div class="center"><Spinner label="Loading bookmark" /></div>
  {:else if phase === "notfound"}
    <p class="detail-empty">This bookmark does not exist or was deleted.</p>
  {:else if phase === "error"}
    <p class="notice error" role="alert">{loadError}</p>
    <Button onclick={() => attempt++}>Retry</Button>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="edit-form" onkeydown={onFieldsKeydown}>
      {#if existing}
        <Notice
          tone="warning"
          title="Already saved"
          message="Saved on {formatTimestamp(existing.date_added)}. Saving updates it."
        />
      {/if}
      {#if saveError}<p class="notice error form-error" role="alert">{saveError}</p>{/if}

      <div class="url-field">
        <FormField
          field={{ id: `${uid}-url`, label: "URL", value: url, disabled: saving }}
          type="url"
          placeholder="https://"
          autocomplete="off"
          oninput={onURLInput}
          onblur={onURLBlur}
        />
        {#if checking}
          <span class="small muted check-note" role="status"
            ><Spinner size={12} label="Checking URL" /> Checking…</span
          >
        {:else if checkError}
          <span class="small muted check-note" role="status">{checkError}</span>
        {/if}
      </div>

      <FormField
        field={{ id: `${uid}-title`, label: "Title", value: title, disabled: saving }}
        placeholder="Page title"
        oninput={(v) => {
          title = v;
          touch("title");
        }}
      />

      <div class="field">
        <label for="{uid}-tags">Tags</label>
        <TagInput
          id="{uid}-tags"
          bind:tags
          bind:text={tagText}
          suggestions={allTags}
          disabled={saving}
          onchange={() => touch("tags")}
        />
        <span class="small muted">Press Enter, space or comma to add a tag.</span>
      </div>

      <div class="field">
        <label for="{uid}-desc">Description</label>
        <textarea
          id="{uid}-desc"
          class="field-area"
          rows="3"
          disabled={saving}
          value={description}
          oninput={(e) => {
            description = e.currentTarget.value;
            touch("description");
          }}></textarea>
      </div>

      <div class="field">
        <label for="{uid}-notes">Notes</label>
        <textarea
          id="{uid}-notes"
          class="field-area"
          rows="4"
          disabled={saving}
          placeholder="Add notes…"
          value={notes}
          oninput={(e) => {
            notes = e.currentTarget.value;
            touch("notes");
          }}></textarea>
        <span class="small muted">Markdown is supported.</span>
      </div>

      <Checkbox
        label="Mark as unread"
        checked={unread}
        disabled={saving}
        onchange={(v) => {
          unread = v;
          touch("unread");
        }}
      />
    </div>
  {/if}

  {#snippet footer()}
    {#if phase !== "ready"}
      <Button onclick={close}>Close</Button>
    {:else if confirmingDiscard}
      <span class="small footer-note">Discard changes?</span>
      <Button class="confirm-cancel" onclick={() => (confirmingDiscard = false)}>
        Keep editing
      </Button>
      <Button tone="danger" surface="solid" onclick={close}>Discard</Button>
    {:else}
      <Button disabled={saving} onclick={requestClose}>Cancel</Button>
      <span class="grow"></span>
      <Button tone="info" surface="solid" disabled={!canSave} onclick={save}>
        {#if saving}<Spinner size={12} label="Saving" />{/if}
        {saveLabel}
      </Button>
    {/if}
  {/snippet}
</Modal>
