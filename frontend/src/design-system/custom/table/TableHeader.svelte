<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";

  interface Props extends HTMLAttributes<HTMLTableSectionElement> {
    children?: Snippet;
  }

  let { children, ...rest }: Props = $props();
</script>

<thead data-slot="table-header" {...rest}>
  {@render children?.()}
</thead>
