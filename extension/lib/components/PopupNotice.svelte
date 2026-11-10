<script lang="ts">
  // The popup's compact notice: a one-line title and an optional one-line
  // message, marked by a tone-colored edge (kit-ui's Notice is too tall here).
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

<style>
  .popup-notice-box {
    display: flex;
    flex-direction: column;
    gap: 0.0625rem;
    padding: 0.375rem 0.625rem;
    border-left: 3px solid var(--tone);
    border-radius: var(--radius-sm, 4px);
    background: color-mix(in srgb, var(--tone) 10%, transparent);
    font-size: var(--font-size-xs, 0.75rem);
    line-height: 1.35;
    min-width: 0;
    width: 100%;
    box-sizing: border-box;
  }
  .popup-notice-box[data-tone="info"] {
    --tone: var(--accent-blue);
  }
  .popup-notice-box[data-tone="success"] {
    --tone: var(--accent-green);
  }
  .popup-notice-box[data-tone="warning"] {
    --tone: var(--accent-amber, var(--accent-yellow, #d4a72c));
  }
  .popup-notice-box[data-tone="error"] {
    --tone: var(--accent-red);
  }
  strong {
    font-weight: 600;
    color: var(--text-primary);
    font-size: var(--font-size-sm, 0.8125rem);
  }
  span {
    color: var(--text-secondary, var(--muted-foreground));
    overflow-wrap: anywhere;
  }
</style>
