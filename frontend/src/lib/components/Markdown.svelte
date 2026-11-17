<script lang="ts">
  import DOMPurify from "dompurify";
  import { marked } from "marked";

  interface Props {
    source: string;
  }
  let { source }: Props = $props();

  const html = $derived(
    DOMPurify.sanitize(marked.parse(source, { async: false, gfm: true }), {
      ADD_ATTR: ["target"],
    }),
  );
</script>

<div class="markdown">{@html html}</div>
