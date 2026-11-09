<script lang="ts">
  import type { Bundle, Tag } from "./lib/api.ts";
  import { activeTagsOf, groupTags } from "./lib/query.ts";
  import { navigate, type View } from "./lib/view.ts";

  interface Props {
    view: View;
    tags: Tag[];
    bundles: Bundle[];
    open: boolean;
    ontag: (tag: string) => void;
  }
  let { view, tags, bundles, open, ontag }: Props = $props();

  const groups = $derived(groupTags(tags));
  const activeTags = $derived(activeTagsOf(view.q));
</script>

<aside class="side" class:open aria-label="Bundles and tags">
  <h4>BUNDLES</h4>
  <ul class="bundle-list">
    <li>
      <button
        type="button"
        class:on={view.bundle === null}
        onclick={() => navigate({ bundle: null })}
      >
        All
      </button>
    </li>
    {#each bundles as b (b.id)}
      <li>
        <button
          type="button"
          class:on={view.bundle === b.id}
          onclick={() => navigate({ bundle: b.id })}
        >
          {b.name}
        </button>
      </li>
    {/each}
  </ul>
  <h4>TAGS</h4>
  {#if groups.length === 0}
    <p class="muted small">No tags yet.</p>
  {/if}
  {#each groups as g (g.letter)}
    <div class="group">
      <b>{g.letter}</b>
      {#each g.tags as t (t.id)}
        <button
          type="button"
          class="tag"
          class:on={activeTags.includes(t.name.toLowerCase())}
          aria-pressed={activeTags.includes(t.name.toLowerCase())}
          title="{t.count} bookmark{t.count === 1 ? '' : 's'}"
          onclick={() => ontag(t.name)}>{t.name}</button
        >
      {/each}
    </div>
  {/each}
</aside>
