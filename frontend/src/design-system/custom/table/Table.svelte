<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLTableAttributes } from "svelte/elements";
  import { setTableState, TableState, type TableSort } from "./context.svelte";

  interface Props extends HTMLTableAttributes {
    sort?: TableSort | null;
    onSortChange?: (sort: TableSort | null) => void;
    children?: Snippet;
  }

  let { sort = $bindable(null), onSortChange, children, ...rest }: Props = $props();

  const state = new TableState((next) => {
    sort = next;
    onSortChange?.(next);
  });
  setTableState(state);

  $effect.pre(() => {
    state.sort = sort;
  });
</script>

<div data-slot="table-container">
  <table data-slot="table" {...rest}>
    {@render children?.()}
  </table>
</div>
