<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";
  import TagButton from "$lib/components/TagButton.svelte";
  import CheckField from "$lib/components/CheckField.svelte";
  import { formatRelativeTime, formatTimestamp } from "./lib/format.ts";
  import type { Bookmark } from "./lib/api.ts";
  import { displayTitle, hostOf } from "./lib/query.ts";

  interface Props {
    bookmark: Bookmark;
    activeTags: string[];
    busy: boolean;
    selecting?: boolean;
    selected?: boolean;
    active?: boolean;
    ontoggle?: () => void;
    ontag: (tag: string) => void;
    onview: () => void;
    onedit: () => void;
    onmarkread: () => void;
    ondelete: (trigger: HTMLElement) => void;
  }
  let {
    bookmark: b,
    activeTags,
    busy,
    selecting = false,
    selected = false,
    active = false,
    ontoggle,
    ontag,
    onview,
    onedit,
    onmarkread,
    ondelete,
  }: Props = $props();
</script>

<li
  class="bm"
  class:selected
  class:row-active={active}
  aria-current={active ? "true" : undefined}
  data-bm={b.id}
>
  {#if selecting}
    <span class="sel">
      <CheckField
        checked={selected}
        ariaLabel="Select {displayTitle(b)}"
        onchange={() => ontoggle?.()}
      />
    </span>
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
        <TagButton
          name={tag}
          on={activeTags.includes(tag.toLowerCase())}
          onclick={() => ontag(tag)}
        />
      {/each}
      <span class="date" title={formatTimestamp(b.date_added)}
        >{formatRelativeTime(b.date_added)}</span
      >
      {#if b.archive_count > 0 && !b.basic_capture_only}<span class="badge good">archived</span
        >{/if}
      {#if b.basic_capture_only}<span class="badge">basic capture</span>{/if}
      <div class="actions">
        {#if b.latest_archive_url}
          <Button
            data-variant="ghost"
            data-size="sm"
            aria-label="View the archived page of {displayTitle(b)}"
          >
            {#snippet render({ props })}<a
                {...props}
                href={b.latest_archive_url}
                target="_blank"
                rel="noopener">View</a
              >{/snippet}
          </Button>
        {/if}
        <Button
          data-variant="ghost"
          data-size="sm"
          aria-label="View {displayTitle(b)}"
          data-view-trigger={b.id}
          onclick={onview}>Details</Button
        >
        <Button
          data-variant="ghost"
          data-size="sm"
          aria-label="Edit {displayTitle(b)}"
          data-edit-trigger={b.id}
          onclick={onedit}>Edit</Button
        >
        {#if b.unread}
          <Button
            data-variant="ghost"
            data-size="sm"
            disabled={busy}
            aria-label="Mark {displayTitle(b)} as read"
            onclick={onmarkread}>Mark read</Button
          >
        {/if}
        <Button
          data-variant="destructive"
          data-size="sm"
          aria-label="Delete {displayTitle(b)}"
          disabled={busy}
          onclick={(e) => ondelete(e.currentTarget as HTMLElement)}>Delete</Button
        >
      </div>
    </div>
  </div>
</li>
