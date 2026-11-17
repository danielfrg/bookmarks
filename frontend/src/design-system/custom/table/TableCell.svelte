<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLTdAttributes } from "svelte/elements";

  interface Props extends Omit<HTMLTdAttributes, "align"> {
    align?: "start" | "center" | "end";
    children?: Snippet;
  }

  let { align, children, ...rest }: Props = $props();
</script>

<td data-slot="table-cell" data-align={align} {...rest}>
  {@render children?.()}
</td>
