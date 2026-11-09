<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    title: string;
    children?: Snippet;
  }
  let { title, children }: Props = $props();
</script>

<div class="page-head">
  <h2>{title}</h2>
  <span class="grow"></span>
  {@render children?.()}
</div>
