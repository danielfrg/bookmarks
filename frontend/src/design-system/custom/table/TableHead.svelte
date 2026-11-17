<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLThAttributes } from "svelte/elements";
  import { getTableState, type SortDirection } from "./context.svelte";

  interface Props extends Omit<HTMLThAttributes, "align"> {
    align?: "start" | "center" | "end";
    // Makes the column sortable: the key passed to sortRows().
    sortKey?: string;
    // The first direction a press picks: "asc" for text, "desc" often reads better for numbers and dates.
    firstDirection?: SortDirection;
    children?: Snippet;
  }

  let { align, sortKey, firstDirection = "asc", children, ...rest }: Props = $props();

  const table = getTableState();
  const direction = $derived(sortKey && table?.sort?.key === sortKey ? table.sort.direction : null);
  const ariaSort = $derived(
    sortKey
      ? direction === "asc"
        ? "ascending"
        : direction === "desc"
          ? "descending"
          : "none"
      : undefined,
  );
</script>

<th data-slot="table-head" data-align={align} scope="col" aria-sort={ariaSort} {...rest}>
  {#if sortKey && table}
    <button
      type="button"
      data-slot="table-sort"
      data-direction={direction ?? undefined}
      onclick={() => table.toggle(sortKey, firstDirection)}
    >
      {@render children?.()}
      <svg viewBox="0 0 24 24" aria-hidden="true" data-slot="table-sort-icon">
        {#if direction === "asc"}
          <path d="m5 12 7-7 7 7M12 19V5" />
        {:else if direction === "desc"}
          <path d="M12 5v14M19 12l-7 7-7-7" />
        {:else}
          <path d="m21 16-4 4-4-4M17 20V4M3 8l4-4 4 4M7 4v16" />
        {/if}
      </svg>
    </button>
  {:else}
    {@render children?.()}
  {/if}
</th>
