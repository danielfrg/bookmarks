<script lang="ts">
  import XIcon from "@lucide/svelte/icons/x";
  import { commitTagText } from "../tags.ts";

  interface Props {
    /** Committed tags (bindable). */
    tags: string[];
    /** Typed text that is not committed yet (bindable); the parent commits it on save. */
    text?: string;
    /** Every existing tag name, used for autocomplete. */
    suggestions?: string[];
    disabled?: boolean;
    id?: string;
    placeholder?: string;
    /** Called on every user change: commit, chip removal, Backspace, typing. */
    onchange?: () => void;
  }
  let {
    tags = $bindable(),
    text = $bindable(""),
    suggestions = [],
    disabled = false,
    id,
    placeholder = "Add tags",
    onchange,
  }: Props = $props();

  const uid = $props.id();
  const listId = `${uid}-list`;
  let focused = $state(false);
  let dismissed = $state(false);
  let active = $state(-1);
  let inputEl = $state<HTMLInputElement>();

  const matches = $derived.by(() => {
    const prefix = text.trim().replace(/^#+/, "").toLowerCase();
    if (!prefix) return [];
    return suggestions
      .filter((n) => n.toLowerCase().startsWith(prefix) && !tags.includes(n.toLowerCase()))
      .slice(0, 8);
  });
  const open = $derived(focused && !dismissed && matches.length > 0);

  function commit(value: string) {
    tags = commitTagText(tags, value);
    onchange?.();
    text = "";
    active = -1;
    dismissed = false;
  }
  function onInput(e: Event) {
    onchange?.();
    if ((e as InputEvent).isComposing) return;
    dismissed = false;
    active = -1;
    // Space and comma commit (typed or pasted); the trailing part stays editable.
    if (/[\s,]/.test(text)) {
      const parts = text.split(/[\s,]+/);
      const last = parts.pop() ?? "";
      commit(parts.join(" "));
      text = last;
    }
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.isComposing) return;
    if (e.key === "Enter") {
      // Enter never submits the form from here.
      e.preventDefault();
      const pick = open && active >= 0 ? matches[active] : undefined;
      if (pick !== undefined) commit(pick);
      else if (text.trim()) commit(text);
    } else if (e.key === "ArrowDown" && matches.length > 0) {
      e.preventDefault();
      dismissed = false;
      active = (active + 1) % matches.length;
    } else if (e.key === "ArrowUp" && matches.length > 0) {
      e.preventDefault();
      active = active <= 0 ? matches.length - 1 : active - 1;
    } else if (e.key === "Escape" && open) {
      // Closes the list only, not the modal.
      e.preventDefault();
      dismissed = true;
      active = -1;
    } else if (e.key === "Backspace" && text === "" && tags.length > 0) {
      tags = tags.slice(0, -1);
      onchange?.();
    }
  }
  function onBlur() {
    focused = false;
    if (text.trim()) commit(text);
  }
  function remove(tag: string) {
    tags = tags.filter((t) => t !== tag);
    onchange?.();
    inputEl?.focus();
  }
</script>

<div class="tag-input" class:disabled>
  {#each tags as tag (tag)}
    <span class="chip">
      #{tag}
      <button
        type="button"
        class="chip-x"
        aria-label="Remove tag {tag}"
        {disabled}
        onclick={() => remove(tag)}><XIcon size={12} aria-hidden="true" /></button
      >
    </span>
  {/each}
  <input
    {id}
    bind:this={inputEl}
    bind:value={text}
    class="tag-text"
    type="text"
    role="combobox"
    aria-expanded={open}
    aria-controls={open ? listId : undefined}
    aria-autocomplete="list"
    aria-activedescendant={open && active >= 0 ? `${uid}-opt-${active}` : undefined}
    autocomplete="off"
    autocapitalize="off"
    spellcheck="false"
    {placeholder}
    {disabled}
    oninput={onInput}
    onkeydown={onKeydown}
    onfocus={() => (focused = true)}
    onblur={onBlur}
  />
  {#if open}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <ul
      id={listId}
      class="tag-suggest"
      role="listbox"
      aria-label="Tag suggestions"
      onmousedown={(e) => e.preventDefault()}
    >
      {#each matches as name, i (name)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id="{uid}-opt-{i}"
          role="option"
          aria-selected={i === active}
          class:on={i === active}
          onclick={() => commit(name)}
        >
          {name}
        </li>
      {/each}
    </ul>
  {/if}
</div>
