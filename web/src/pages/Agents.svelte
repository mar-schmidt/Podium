<script lang="ts">
  import type { Agent } from "../lib/types";

  let {
    agents = [],
    onHire = () => {},
  }: { agents?: Agent[]; onHire?: () => void } = $props();

  function initial(name: string) {
    return name.slice(0, 1).toUpperCase();
  }
</script>

<div class="page">
  <header class="page-head">
    <div>
      <h1>Agents</h1>
      <p>Your named colleagues. Each has a workspace, a SOUL.md, and a default backend.</p>
    </div>
    <button class="button primary" onclick={onHire}>+ Hire agent</button>
  </header>

  <div class="card-grid">
    {#each agents as agent}
      <article class="card agent-card">
        <div class="avatar">{initial(agent.Name)}</div>
        <div class="card-body">
          <h3>{agent.Name}</h3>
          <p class="muted">{agent.Provider}{agent.Model ? ` · ${agent.Model}` : ""}</p>
          <div class="chips">
            <span class="chip">{agent.PermissionMode}</span>
            <span class="chip">effort {agent.Effort || "medium"}</span>
            {#if agent.Profile}<span class="chip">{agent.Profile}</span>{/if}
          </div>
        </div>
      </article>
    {/each}
    {#if agents.length === 0}
      <p class="empty">No agents yet. Hire your first colleague.</p>
    {/if}
  </div>
</div>
