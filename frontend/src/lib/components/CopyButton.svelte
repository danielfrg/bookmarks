<script lang="ts">
  import { CheckIcon, CopyIcon } from "@lucide/svelte";
  import { Button } from "@ui-sdk/svelte/button";

  interface Props {
    text: string;
    label: string;
    class?: string;
  }
  let { text, label, class: className = "" }: Props = $props();

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      return;
    }
    copied = true;
    clearTimeout(timer);
    timer = setTimeout(() => (copied = false), 1500);
  }
  $effect(() => () => clearTimeout(timer));
</script>

<Button class={["copy-button", className]} data-variant="ghost" aria-label={label} onclick={copy}>
  {#if copied}<CheckIcon aria-hidden="true" />{:else}<CopyIcon aria-hidden="true" />{/if}
</Button>
