<script lang="ts">
  import type { Provider } from "./types";

  let { provider, size = 16 }: { provider: Provider | string; size?: number } = $props();
</script>

{#if provider === "codex"}
  <svg
    class="provider-logo codex-logo"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    aria-hidden="true"
    fill="none">
    <path d="M12 3.4c2.1 0 3.8 1.7 3.8 3.8v1.1l-3.8 2.2-3.8-2.2V7.2c0-2.1 1.7-3.8 3.8-3.8Z" />
    <path d="M19.4 8c1.1 1.8.5 4.1-1.3 5.2l-1 .6-3.8-2.2V7.2l1-.6c1.8-1 4.1-.4 5.1 1.4Z" />
    <path d="M19.4 16c-1.1 1.8-3.4 2.4-5.2 1.3l-1-.6v-4.4l3.8-2.2 1 .6c1.9 1.1 2.5 3.4 1.4 5.3Z" />
    <path d="M12 20.6a3.8 3.8 0 0 1-3.8-3.8v-1.1l3.8-2.2 3.8 2.2v1.1c0 2.1-1.7 3.8-3.8 3.8Z" />
    <path d="M4.6 16c-1.1-1.8-.5-4.1 1.3-5.2l1-.6 3.8 2.2v4.4l-1 .6c-1.8 1-4.1.4-5.1-1.4Z" />
    <path d="M4.6 8c1.1-1.8 3.4-2.4 5.2-1.3l1 .6v4.4L7 13.8l-1-.6C4.1 12.1 3.5 9.8 4.6 8Z" />
  </svg>
{:else}
  <svg
    class="provider-logo claude-logo"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    aria-hidden="true"
    fill="none">
    <path d="M12 3.2v17.6" />
    <path d="M4.2 7.4 19.8 16.6" />
    <path d="M19.8 7.4 4.2 16.6" />
    <path d="M6.8 4.6 17.2 19.4" />
    <path d="M17.2 4.6 6.8 19.4" />
    <circle cx="12" cy="12" r="2.2" />
  </svg>
{/if}
