<script lang="ts">
  import { untrack } from "svelte";
  import {
    Button,
    SearchInput,
    SegmentedControl,
    SelectDropdown,
    ThemeToggle,
  } from "@kenn-io/kit-ui";
  import TagIcon from "@lucide/svelte/icons/tag";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import { syncSearchText } from "./lib/query.ts";
  import { navigate, type Filter, type View } from "./lib/view.ts";

  interface Props {
    view: View;
    panelOpen: boolean;
    ontogglepanel: () => void;
  }
  let { view, panelOpen, ontogglepanel }: Props = $props();

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
    if (value.trim() !== view.q) navigate({ q: value.trim(), page: 1 });
  }
  function onInput(value: string) {
    clearTimeout(timer);
    timer = setTimeout(() => submit(value), 300);
  }

  const filters = [
    { value: "active", label: "Active" },
    { value: "unread", label: "Unread" },
    { value: "archived", label: "Archived" },
  ];
  const sorts = [
    { value: "added_desc", label: "Newest" },
    { value: "added_asc", label: "Oldest" },
    { value: "title_asc", label: "Title A–Z" },
    { value: "title_desc", label: "Title Z–A" },
  ];
  const links = [
    { name: "tags", label: "Tags" },
    { name: "bundles", label: "Bundles" },
    { name: "settings", label: "Settings" },
  ] as const;
</script>

<header class="topbar">
  <div class="topbar-row">
    <button
      class="wordmark"
      type="button"
      onclick={() => navigate({ pageName: "list", q: "", filter: "active", bundle: null, page: 1 })}
    >
      Bookmarks
    </button>
    <div class="search-wrap">
      <SearchInput
        bind:value={text}
        block
        size="sm"
        placeholder={'Search… #tag !unread "phrase"'}
        ariaLabel="Search bookmarks"
        oninput={onInput}
        onkeydown={(e) => e.key === "Enter" && submit(text)}
        onclear={() => submit("")}
      />
    </div>
    <nav class="toplinks" aria-label="Pages">
      {#each links as l (l.name)}
        <button
          type="button"
          class="toplink"
          class:on={view.pageName === l.name}
          onclick={() => navigate({ pageName: l.name })}>{l.label}</button
        >
      {/each}
    </nav>
    <ThemeToggle />
  </div>
  <div class="topbar-row controls">
    <SegmentedControl
      ariaLabel="Filter"
      options={filters}
      value={view.filter}
      onchange={(v) => navigate({ filter: v as Filter, pageName: "list", page: 1 })}
    />
    <SelectDropdown
      title="Sort"
      options={sorts}
      value={view.sort}
      onchange={(v) => navigate({ sort: v, pageName: "list", page: 1 })}
    />
    <span class="grow"></span>
    <span class="panel-toggle">
      <Button size="sm" ariaExpanded={panelOpen} onclick={ontogglepanel}>
        <TagIcon size={14} /> Tags
      </Button>
    </span>
    <Button
      tone="info"
      surface="solid"
      size="sm"
      class="add-trigger"
      onclick={() => navigate({ create: { url: "", title: "" } })}
    >
      <PlusIcon size={14} /> Add
    </Button>
  </div>
</header>
