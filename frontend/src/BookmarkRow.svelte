<script lang="ts">
  import { formatRelativeTime, formatTimestamp } from "@kenn-io/kit-ui";
  import type { Bookmark } from "./lib/api.ts";
  import { displayTitle, hostOf } from "./lib/query.ts";

  interface Props {
    bookmark: Bookmark;
    activeTags: string[];
    busy: boolean;
    selecting?: boolean;
    selected?: boolean;
    ontoggle?: () => void;
    ontag: (tag: string) => void;
    onview: () => void;
    onedit: () => void;
    onarchive: () => void;
    ondelete: (trigger: HTMLElement) => void;
  }
  let {
    bookmark: b,
    activeTags,
    busy,
    selecting = false,
    selected = false,
    ontoggle,
    ontag,
    onview,
    onedit,
    onarchive,
    ondelete,
  }: Props = $props();
</script>

<li class="bm" class:selected>
  {#if selecting}
    <input
      type="checkbox"
      class="sel"
      checked={selected}
      aria-label="Select {displayTitle(b)}"
      onchange={() => ontoggle?.()}
    />
  {/if}
  {#if b.favicon_url}
    <img class="fav" src={b.favicon_url} alt="" loading="lazy" width="16" height="16" />
  {:else}
    <span class="fav placeholder" aria-hidden="true"></span>
  {/if}
  <div class="bm-body">
    <div class="bm-title">
      {#if b.unread}<span class="dot" title="Unread" role="img" aria-label="Unread"></span>{/if}
      <a href={b.url} target="_blank" rel="noopener noreferrer" class:untitled={!b.title.trim()}>
        {displayTitle(b)}
      </a>
    </div>
    {#if b.description}<p class="bm-desc">{b.description}</p>{/if}
    <div class="meta">
      <span class="host mono">{hostOf(b.url)}</span>
      {#each b.tags as tag (tag)}
        <button
          type="button"
          class="tag"
          class:on={activeTags.includes(tag.toLowerCase())}
          aria-pressed={activeTags.includes(tag.toLowerCase())}
          onclick={() => ontag(tag)}>#{tag}</button
        >
      {/each}
      <span class="date" title={formatTimestamp(b.date_added)}
        >{formatRelativeTime(b.date_added)}</span
      >
      {#if b.archive_count > 0 && !b.basic_capture_only}<span class="badge good">archived</span
        >{/if}
      {#if b.basic_capture_only}<span class="badge">basic capture</span>{/if}
      <div class="actions">
        <button
          type="button"
          class="act"
          aria-label="View {displayTitle(b)}"
          data-view-trigger={b.id}
          onclick={onview}>View</button
        >
        <button
          type="button"
          class="act"
          aria-label="Edit {displayTitle(b)}"
          data-edit-trigger={b.id}
          onclick={onedit}>Edit</button
        >
        <button
          type="button"
          class="act"
          disabled={busy}
          aria-label="{b.is_archived ? 'Unarchive' : 'Archive'} {displayTitle(b)}"
          onclick={onarchive}
        >
          {b.is_archived ? "Unarchive" : "Archive"}
        </button>
        <button
          type="button"
          class="act danger"
          aria-label="Delete {displayTitle(b)}"
          disabled={busy}
          onclick={(e) => ondelete(e.currentTarget)}>Delete</button
        >
      </div>
    </div>
  </div>
</li>
