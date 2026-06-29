<script lang="ts">
  import { onMount } from "svelte";
  import { listSchedules, runSchedule } from "../lib/api";
  import type { RunStatus, ScheduleStatus } from "../lib/types";

  let schedules = $state<ScheduleStatus[]>([]);
  let error = $state<string | null>(null);
  let busy = $state<string>("");

  onMount(load);

  async function load() {
    try {
      schedules = await listSchedules();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function runNow(name: string) {
    busy = name;
    error = null;
    try {
      await runSchedule(name);
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = "";
    }
  }

  function timing(s: ScheduleStatus) {
    return s.every ? `every ${s.every}` : s.cron;
  }

  function nextLabel(s: ScheduleStatus) {
    if (!s.next_run) return "—";
    return new Date(s.next_run).toLocaleString([], { weekday: "short", hour: "2-digit", minute: "2-digit" });
  }

  function runDotClass(status: RunStatus) {
    return `run-dot ${status}`;
  }
</script>

<div class="page">
  <header class="page-head">
    <div>
      <h1>Schedules</h1>
      <p>Each job is a markdown file under <code>~/.podium/schedules</code>. Frontmatter sets the engine, the body is the prompt.</p>
    </div>
  </header>

  {#if error}<div class="error">{error}</div>{/if}

  <div class="stack">
    {#each schedules as s}
      <article class="card schedule-card">
        <div class="schedule-top">
          <div>
            <h3>{s.name}</h3>
            {#if s.parse_error}
              <p class="error inline">{s.parse_error}</p>
            {:else}
              <p class="muted">{s.path}</p>
            {/if}
          </div>
          {#if !s.parse_error}
            <div class="schedule-actions">
              <span class="chip">{s.run_permission}</span>
              <span class="muted">next {nextLabel(s)}</span>
              <button class="button secondary" disabled={busy === s.name} onclick={() => runNow(s.name)}>
                {busy === s.name ? "Running…" : "Run now"}
              </button>
            </div>
          {/if}
        </div>
        {#if !s.parse_error}
          <div class="chips">
            <span class="chip">{timing(s)}</span>
            <span class="chip">agent {s.agent}</span>
            {#if s.model}<span class="chip">model {s.model}</span>{/if}
            {#if s.effort}<span class="chip">effort {s.effort}</span>{/if}
            <span class="chip">{s.enabled ? "enabled" : "disabled"}</span>
          </div>
          {#if s.runs && s.runs.length > 0}
            <div class="runs">
              {#each s.runs as run}
                <span class={runDotClass(run.Status)} title={`${run.Trigger} · ${run.Status}`}></span>
              {/each}
              <span class="muted small">last {s.runs.length}</span>
            </div>
          {/if}
        {/if}
      </article>
    {/each}
    {#if schedules.length === 0}
      <p class="empty">No schedules. Drop a <code>*.md</code> file in <code>~/.podium/schedules/</code>.</p>
    {/if}
  </div>
</div>
