<script lang="ts">
  // The popup's compact notice: a one-line title and an optional one-line
  // message, marked by a tone-colored edge (styled in app.css).
  interface Props {
    tone: "info" | "success" | "warning" | "error";
    title: string;
    message?: string;
  }
  let { tone, title, message = "" }: Props = $props();
</script>

<div
  class="popup-notice-box"
  data-tone={tone}
  role={tone === "error" ? "alert" : "status"}
>
  <strong>{title}</strong>
  {#if message}<span>{message}</span>{/if}
</div>
