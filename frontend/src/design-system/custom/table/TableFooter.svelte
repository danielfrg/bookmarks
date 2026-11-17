<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";

  interface Props extends HTMLAttributes<HTMLTableSectionElement> {
    children?: Snippet;
  }

  let { children, ...rest }: Props = $props();
</script>

<tfoot data-slot="table-footer" {...rest}>
  {@render children?.()}
</tfoot>
