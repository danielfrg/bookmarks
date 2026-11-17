<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    title: string;
    description?: string;
    children: Snippet;
  }
  let { title, description = "", children }: Props = $props();
</script>

<section class="settings-section">
  <header>
    <h3>{title}</h3>
    {#if description}<p>{description}</p>{/if}
  </header>
  <div class="settings-section-body">{@render children()}</div>
</section>
