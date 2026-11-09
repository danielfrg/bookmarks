<script lang="ts">
  import { Button, Modal, SegmentedControl, Spinner, TextInput } from "@kenn-io/kit-ui";
  import { untrack } from "svelte";
  import { api, type Bundle } from "./lib/api.ts";
  import { commitTagText } from "./lib/query.ts";
  import TagInput from "./TagInput.svelte";

  interface Props {
    /** The bundle to edit; null creates a new one. */
    bundle: Bundle | null;
    suggestions: string[];
    onsaved: () => void;
    onclose: () => void;
  }
  let { bundle, suggestions, onsaved, onclose }: Props = $props();

  const uid = $props.id();
  const b0 = untrack(() => bundle);
  let name = $state(b0?.name ?? "");
  let search = $state(b0?.search ?? "");
  let anyTags = $state<string[]>([...(b0?.any_tags ?? [])]);
  let allTags = $state<string[]>([...(b0?.all_tags ?? [])]);
  let excludedTags = $state<string[]>([...(b0?.excluded_tags ?? [])]);
  let anyText = $state("");
  let allText = $state("");
  let exclText = $state("");
  let unread = $state<string>(b0?.filter_unread ?? "off");
  let saving = $state(false);
  let error = $state("");
  let nameInvalid = $state(false);

  async function save() {
    if (saving) return;
    if (!name.trim()) {
      nameInvalid = true;
      error = "Name is required.";
      return;
    }
    saving = true;
    error = "";
    try {
      await api.saveBundle({
        id: b0?.id,
        name: name.trim(),
        search: search.trim(),
        any_tags: commitTagText(anyTags, anyText),
        all_tags: commitTagText(allTags, allText),
        excluded_tags: commitTagText(excludedTags, exclText),
        filter_unread: unread as Bundle["filter_unread"],
      });
      onsaved();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== "Enter" || e.isComposing || e.defaultPrevented) return;
    const t = e.target as HTMLElement;
    if (t.tagName !== "INPUT" || t.closest(".tag-input")) return;
    e.preventDefault();
    void save();
  }
</script>

<Modal
  title={b0 ? "Edit bundle" : "New bundle"}
  ariaLabel={b0 ? "Edit bundle" : "New bundle"}
  width="560px"
  maxWidth="min(560px, calc(100vw - 32px))"
  onclose={() => !saving && onclose()}
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="edit-form" onkeydown={onKeydown}>
    {#if error}<p class="notice error form-error" role="alert">{error}</p>{/if}
    <div class="field">
      <label for="{uid}-name">Name</label>
      <TextInput
        id="{uid}-name"
        block
        autofocus
        required
        invalid={nameInvalid}
        disabled={saving}
        bind:value={name}
        oninput={() => (nameInvalid = false)}
      />
    </div>
    <div class="field">
      <label for="{uid}-search">Search</label>
      <TextInput
        id="{uid}-search"
        block
        class="mono"
        disabled={saving}
        bind:value={search}
        placeholder="Search terms"
      />
    </div>
    <div class="field">
      <label for="{uid}-any">Any of these tags</label>
      <TagInput
        id="{uid}-any"
        bind:tags={anyTags}
        bind:text={anyText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <div class="field">
      <label for="{uid}-all">All of these tags</label>
      <TagInput
        id="{uid}-all"
        bind:tags={allTags}
        bind:text={allText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <div class="field">
      <label for="{uid}-excl">Excluded tags</label>
      <TagInput
        id="{uid}-excl"
        bind:tags={excludedTags}
        bind:text={exclText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <div class="field">
      <span class="field-label" id="{uid}-unread">Unread</span>
      <SegmentedControl
        ariaLabel="Unread filter"
        value={unread}
        disabled={saving}
        options={[
          { value: "off", label: "Off" },
          { value: "yes", label: "Yes" },
          { value: "no", label: "No" },
        ]}
        onchange={(v) => (unread = v)}
      />
    </div>
  </div>
  {#snippet footer()}
    <Button disabled={saving} onclick={onclose}>Cancel</Button>
    <span class="grow"></span>
    <Button tone="info" surface="solid" disabled={saving} onclick={save}>
      {#if saving}<Spinner size={12} label="Saving" />{/if}
      Save
    </Button>
  {/snippet}
</Modal>
