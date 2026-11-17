<script lang="ts">
  import { untrack } from "svelte";
  import { CheckIcon, ChevronDownIcon, PlusIcon, SearchIcon, TagIcon, XIcon } from "@lucide/svelte";
  import { Button } from "@ui-sdk/svelte/button";
  import { Input, InputClear, InputGroup } from "@ui-sdk/svelte/input";
  import {
    SegmentedControl,
    SegmentedControlIndicator,
    SegmentedControlItem,
  } from "@ui-sdk/svelte/segmented-control";
  import {
    Select,
    SelectContent,
    SelectItem,
    SelectItemIndicator,
    SelectLabel,
    SelectTrigger,
    SelectValue,
  } from "@ui-sdk/svelte/select";
  import { Toggle } from "@ui-sdk/svelte/toggle";
  import { ThemeToggle } from "../../design-system/custom/theme-toggle";
  import { colorScheme } from "$lib/color-scheme";
  import { syncSearchText } from "../query.ts";
  import { navigate, type Filter, type View } from "../view.ts";

  interface Props {
    view: View;
    panelOpen: boolean;
    /** Bulk select mode on the list. */
    selecting: boolean;
    ontogglepanel: () => void;
    onselect: () => void;
    onhelp: () => void;
  }
  let { view, panelOpen, selecting, ontogglepanel, onselect, onhelp }: Props = $props();

  let text = $state("");
  let timer: ReturnType<typeof setTimeout> | undefined;

  // Follow external changes (tag clicks, back/forward) without fighting typing.
  let lastQ = "";
  $effect(() => {
    const q = view.q;
    // `text` is read untracked: typing must not re-run this effect.
    const next = untrack(() => syncSearchText(lastQ, q, text));
    if (next !== untrack(() => text)) {
      clearTimeout(timer);
      text = next;
    }
    lastQ = q;
  });

  function submit(value: string) {
    clearTimeout(timer);
    // Searching from another page (or the reader) goes to the list.
    if (value.trim() !== view.q)
      navigate({ q: value.trim(), page: 1, pageName: "list", read: null });
  }
  function onInput(value: string, reason: string) {
    clearTimeout(timer);
    if (reason === "clear-press") submit("");
    else timer = setTimeout(() => submit(value), 300);
  }

  const filters = [
    { value: "all", label: "All" },
    { value: "unread", label: "Unread" },
  ];
  const sorts = [
    { value: "added_desc", label: "Newest" },
    { value: "added_asc", label: "Oldest" },
    { value: "title_asc", label: "Title A–Z" },
    { value: "title_desc", label: "Title Z–A" },
  ];
  const onList = $derived(view.pageName === "list" && view.read === null);
  const links = [
    { name: "tags", label: "Tags" },
    { name: "saved", label: "Saved searches" },
    { name: "settings", label: "Settings" },
  ] as const;
</script>

<header class="topbar">
  <div class="topbar-inner">
    <div class="topbar-row">
      <Button
        class="wordmark"
        data-variant="ghost"
        onclick={() =>
          navigate({ pageName: "list", read: null, q: "", filter: "all", saved: null, page: 1 })}
        >Bookmarks</Button
      >
      <span class="grow"></span>
      <nav class="toplinks" aria-label="Pages">
        {#each links as l (l.name)}
          <Button
            data-variant={view.pageName === l.name ? "secondary" : "ghost"}
            aria-current={view.pageName === l.name ? "page" : undefined}
            onclick={() => navigate({ pageName: l.name })}>{l.label}</Button
          >
        {/each}
      </nav>
      <ThemeToggle {colorScheme} />
    </div>
    <div class="topbar-row controls">
      <InputGroup class="search-wrap">
        <SearchIcon aria-hidden="true" />
        <Input
          type="search"
          bind:value={text}
          placeholder={'Search… #tag !unread "phrase"'}
          aria-label="Search bookmarks"
          autocomplete="off"
          onValueChange={(value, details) => onInput(value, details.reason)}
          onkeydown={(e) => e.key === "Enter" && submit(text)}
        />
        <InputClear aria-label="Clear search"><XIcon aria-hidden="true" /></InputClear>
      </InputGroup>
      {#if onList}
        <SegmentedControl
          aria-label="Filter"
          value={view.filter}
          onValueChange={(v) => v && navigate({ filter: v as Filter, pageName: "list", page: 1 })}
        >
          <SegmentedControlIndicator />
          {#each filters as f (f.value)}
            <SegmentedControlItem value={f.value}>{f.label}</SegmentedControlItem>
          {/each}
        </SegmentedControl>
      {/if}
      <span class="grow"></span>
      {#if onList}
        <span class="panel-toggle">
          <Button aria-expanded={panelOpen} onclick={ontogglepanel}
            ><TagIcon aria-hidden="true" /> Tags</Button
          >
        </span>
      {/if}
      <Button
        class="add-trigger"
        data-variant="solid"
        onclick={() => navigate({ create: { url: "", title: "" } })}
        ><PlusIcon aria-hidden="true" /> Add</Button
      >
      {#if onList}
        <Toggle
          class="select-trigger"
          pressed={selecting}
          onPressedChange={() => onselect()}
          title="Select bookmarks for bulk actions">Select</Toggle
        >
        <Select
          items={sorts}
          value={view.sort}
          onValueChange={(v) => v && navigate({ sort: v, pageName: "list", page: 1 })}
        >
          <SelectLabel class="sr-only">Sort</SelectLabel>
          <SelectTrigger>
            <SelectValue />
            <ChevronDownIcon aria-hidden="true" />
          </SelectTrigger>
          <SelectContent>
            {#each sorts as item (item.value)}
              <SelectItem value={item.value}>
                {item.label}
                <SelectItemIndicator><CheckIcon aria-hidden="true" /></SelectItemIndicator>
              </SelectItem>
            {/each}
          </SelectContent>
        </Select>
      {/if}
      <Button
        class="help-trigger"
        aria-label="Help"
        title="Shortcuts and search help (?)"
        onclick={onhelp}>?</Button
      >
    </div>
  </div>
</header>
