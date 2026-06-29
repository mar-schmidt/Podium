<script lang="ts">
  import { onMount } from "svelte";
  import type { Health } from "./lib/types";

  let health = $state<Health | null>(null);
  let error = $state<string | null>(null);

  onMount(async () => {
    try {
      const res = await fetch("/healthz");
      if (!res.ok) throw new Error(`status ${res.status}`);
      health = (await res.json()) as Health;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  });
</script>

<main class="mx-auto max-w-2xl p-10">
  <header class="mb-8 flex items-center gap-3">
    <div class="grid h-10 w-10 place-items-center rounded-xl bg-[var(--color-accent)] text-white">
      ◆
    </div>
    <div>
      <h1 class="text-2xl font-bold tracking-tight">Podium</h1>
      <p class="text-sm text-[var(--color-muted)]">conductor</p>
    </div>
  </header>

  <div class="rounded-2xl bg-[var(--color-surface)] p-6 shadow-sm">
    {#if health}
      <div class="flex items-center gap-2">
        <span class="h-2.5 w-2.5 rounded-full bg-emerald-500"></span>
        <span class="font-medium">podiumd live</span>
      </div>
      <dl class="mt-4 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <dt class="text-[var(--color-muted)]">status</dt>
        <dd>{health.status}</dd>
        <dt class="text-[var(--color-muted)]">version</dt>
        <dd>{health.version} ({health.commit})</dd>
        <dt class="text-[var(--color-muted)]">uptime</dt>
        <dd>{health.uptime_ms}ms</dd>
      </dl>
    {:else if error}
      <div class="flex items-center gap-2 text-[var(--color-accent)]">
        <span class="h-2.5 w-2.5 rounded-full bg-[var(--color-accent)]"></span>
        <span class="font-medium">cannot reach podiumd</span>
      </div>
      <p class="mt-2 text-sm text-[var(--color-muted)]">{error}</p>
    {:else}
      <p class="text-sm text-[var(--color-muted)]">connecting…</p>
    {/if}
  </div>

  <p class="mt-6 text-sm text-[var(--color-muted)]">
    Phase 0 skeleton. The full chat UI ships in Phase 4.
  </p>
</main>
