<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";

  interface Props extends HTMLAttributes<HTMLElement> {
    children?: Snippet;
  }

  let { children, ...rest }: Props = $props();
</script>

<caption data-slot="table-caption" {...rest}>
  {@render children?.()}
</caption>
