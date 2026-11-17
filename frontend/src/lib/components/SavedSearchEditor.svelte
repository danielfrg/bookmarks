<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";
  import { Field, FieldLabel } from "@ui-sdk/svelte/field";
  import { Input } from "@ui-sdk/svelte/input";
  import {
    SegmentedControl,
    SegmentedControlIndicator,
    SegmentedControlItem,
  } from "@ui-sdk/svelte/segmented-control";
  import Modal from "$lib/components/Modal.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import { untrack } from "svelte";
  import { api, type SavedSearch } from "../api.ts";
  import { commitTagText } from "../query.ts";
  import TagInput from "./TagInput.svelte";

  interface Props {
    /** The saved search to edit; null creates a new one. */
    savedSearch: SavedSearch | null;
    suggestions: string[];
    onsaved: () => void;
    onclose: () => void;
  }
  let { savedSearch, suggestions, onsaved, onclose }: Props = $props();

  const uid = $props.id();
  const b0 = untrack(() => savedSearch);
  let name = $state(b0?.name ?? "");
  let query = $state(b0?.query ?? "");
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
      await api.saveSavedSearch({
        id: b0?.id,
        name: name.trim(),
        query: query.trim(),
        any_tags: commitTagText(anyTags, anyText),
        all_tags: commitTagText(allTags, allText),
        excluded_tags: commitTagText(excludedTags, exclText),
        filter_unread: unread as SavedSearch["filter_unread"],
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
  title={b0 ? "Edit saved search" : "New saved search"}
  size="md"
  onclose={() => !saving && onclose()}
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="edit-form" onkeydown={onKeydown}>
    {#if error}<p class="notice error" role="alert">{error}</p>{/if}
    <Field name="name" invalid={nameInvalid ? true : undefined}>
      <FieldLabel>Name</FieldLabel>
      <Input bind:value={name} required disabled={saving} oninput={() => (nameInvalid = false)} />
    </Field>
    <Field name="search">
      <FieldLabel>Search</FieldLabel>
      <Input class="mono" bind:value={query} disabled={saving} placeholder="Search terms" />
    </Field>
    <div class="field">
      <label data-slot="field-label" for="{uid}-any">Any of these tags</label>
      <TagInput
        id="{uid}-any"
        bind:tags={anyTags}
        bind:text={anyText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <div class="field">
      <label data-slot="field-label" for="{uid}-all">All of these tags</label>
      <TagInput
        id="{uid}-all"
        bind:tags={allTags}
        bind:text={allText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <div class="field">
      <label data-slot="field-label" for="{uid}-excl">Excluded tags</label>
      <TagInput
        id="{uid}-excl"
        bind:tags={excludedTags}
        bind:text={exclText}
        {suggestions}
        disabled={saving}
      />
    </div>
    <Field name="unread">
      <FieldLabel>Unread</FieldLabel>
      <SegmentedControl value={unread} disabled={saving} onValueChange={(v) => v && (unread = v)}>
        <SegmentedControlIndicator />
        <SegmentedControlItem value="off">Off</SegmentedControlItem>
        <SegmentedControlItem value="yes">Yes</SegmentedControlItem>
        <SegmentedControlItem value="no">No</SegmentedControlItem>
      </SegmentedControl>
    </Field>
  </div>
  {#snippet footer()}
    <Button disabled={saving} onclick={onclose}>Cancel</Button>
    <span class="grow"></span>
    <Button data-variant="solid" disabled={saving} onclick={save}>
      {#if saving}<Spinner size={12} label="Saving" />{/if}
      Save
    </Button>
  {/snippet}
</Modal>
