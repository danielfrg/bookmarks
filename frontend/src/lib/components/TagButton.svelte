<script lang="ts">
  import { Button } from "@ui-sdk/svelte/button";

  interface Props {
    name: string;
    /** Shown pressed while the tag is part of the search. */
    on?: boolean;
    title?: string;
    onclick: () => void;
  }
  let { name, on, title, onclick }: Props = $props();
</script>

<Button class="tag" data-variant="ghost" data-size="sm" aria-pressed={on} {title} {onclick}
  >#{name}</Button
>
