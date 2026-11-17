import { getContext, setContext } from "svelte";

export type SortDirection = "asc" | "desc";

export interface TableSort {
  key: string;
  direction: SortDirection;
}

export class TableState {
  sort = $state<TableSort | null>(null);
  #onChange: (sort: TableSort | null) => void;

  constructor(onChange: (sort: TableSort | null) => void) {
    this.#onChange = onChange;
  }

  // Cycles first direction, the other one, then off.
  toggle(key: string, first: SortDirection) {
    const other = first === "asc" ? "desc" : "asc";
    let next: TableSort | null;
    if (this.sort?.key !== key) next = { key, direction: first };
    else if (this.sort.direction === first) next = { key, direction: other };
    else next = null;
    this.sort = next;
    this.#onChange(next);
  }
}

const key = Symbol("table");

export const setTableState = (state: TableState) => setContext(key, state);
export const getTableState = () => getContext<TableState | undefined>(key);
