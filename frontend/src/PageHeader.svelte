<script lang="ts">
  import type { Snippet } from "svelte";
  import ArrowLeftIcon from "@lucide/svelte/icons/arrow-left";
  import { navigate } from "./lib/view.ts";

  interface Props {
    title: string;
    children?: Snippet;
  }
  let { title, children }: Props = $props();
</script>

<div class="page-head">
  <a
    class="back-link"
    href="/"
    onclick={(e) => {
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      e.preventDefault();
      navigate({ pageName: "list" });
    }}><ArrowLeftIcon size={14} aria-hidden="true" /> Bookmarks</a
  >
  <h2>{title}</h2>
  <span class="grow"></span>
  {@render children?.()}
</div>
