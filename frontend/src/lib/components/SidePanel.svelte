<script lang="ts">
  import type { SavedSearch, Tag } from "../api.ts";
  import { activeTagsOf, groupTags } from "../query.ts";
  import { Button } from "@ui-sdk/svelte/button";
  import TagButton from "$lib/components/TagButton.svelte";
  import { navigate, type View } from "../view.ts";

  interface Props {
    view: View;
    tags: Tag[];
    savedSearches: SavedSearch[];
    status: "loading" | "ready" | "error";
    open: boolean;
    ontag: (tag: string) => void;
  }
  let { view, tags, savedSearches, status, open, ontag }: Props = $props();

  const groups = $derived(groupTags(tags));
  const activeTags = $derived(activeTagsOf(view.q));
</script>

<aside class="side" class:open aria-label="Saved searches and tags">
  <h4>SAVED SEARCHES</h4>
  <ul class="saved-list">
    <li>
      <Button
        class="saved-item"
        data-variant={view.saved === null ? "secondary" : "ghost"}
        onclick={() => navigate({ saved: null })}>All</Button
      >
    </li>
    {#each savedSearches as b (b.id)}
      <li>
        <Button
          class="saved-item"
          data-variant={view.saved === b.id ? "secondary" : "ghost"}
          onclick={() => navigate({ saved: b.id })}>{b.name}</Button
        >
      </li>
    {/each}
  </ul>
  <h4>TAGS</h4>
  {#if status === "error"}
    <p class="muted small" role="alert">Could not load tags and saved searches.</p>
  {:else if status === "loading" && groups.length === 0}
    <p class="muted small">Loading…</p>
  {:else if groups.length === 0}
    <p class="muted small">No tags yet.</p>
  {/if}
  {#each groups as g (g.letter)}
    <div class="group">
      <b>{g.letter}</b>
      {#each g.tags as t (t.id)}
        <TagButton
          name={t.name}
          on={activeTags.includes(t.name.toLowerCase())}
          title="{t.count} bookmark{t.count === 1 ? '' : 's'}"
          onclick={() => ontag(t.name)}
        />
      {/each}
    </div>
  {/each}
</aside>
