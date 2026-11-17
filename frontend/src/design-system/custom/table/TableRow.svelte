<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";

  interface Props extends HTMLAttributes<HTMLTableRowElement> {
    children?: Snippet;
  }

  let { children, ...rest }: Props = $props();
</script>

<tr data-slot="table-row" {...rest}>
  {@render children?.()}
</tr>
