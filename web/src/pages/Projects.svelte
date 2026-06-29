<script lang="ts">
  import { onMount } from "svelte";
  import { createProject, listProjects } from "../lib/api";
  import type { Project } from "../lib/types";

  let projects = $state<Project[]>([]);
  let error = $state<string | null>(null);
  let creating = $state(false);
  let newId = $state("");
  let newName = $state("");
  let newDescription = $state("");
  let newStack = $state("");
  let newNotes = $state("");

  onMount(load);

  async function load() {
    try {
      projects = await listProjects();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function submit() {
    error = null;
    try {
      await createProject({
        id: newId.trim(),
        name: newName.trim(),
        description: newDescription.trim(),
        stack: newStack.split(",").map((s) => s.trim()).filter(Boolean),
        notes: newNotes.trim(),
      });
      creating = false;
      newId = newName = newDescription = newStack = newNotes = "";
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<div class="page">
  <header class="page-head">
    <div>
      <h1>Projects</h1>
      <p>Shared, agent-independent work tracked in <code>~/.podium/projects/projects.yaml</code>. Any agent can pick up any project.</p>
    </div>
    <button class="button primary" onclick={() => (creating = true)}>+ New project</button>
  </header>

  {#if error}<div class="error">{error}</div>{/if}

  <div class="card-grid">
    {#each projects as project}
      <article class="card project-card">
        <div class="card-body">
          <div class="title-row">
            <h3>{project.name}</h3>
            <span class="chip">{project.status}</span>
          </div>
          <p class="muted">{project.description || "No description yet."}</p>
          {#if project.stack && project.stack.length > 0}
            <div class="chips">
              {#each project.stack as tech}<span class="chip">{tech}</span>{/each}
            </div>
          {/if}
          {#if project.notes}<p class="notes">{project.notes}</p>{/if}
          <div class="meta-row">
            <span class="muted small">{project.path}</span>
            {#if project.backlog?.length}<span class="muted small">{project.backlog.length} backlog</span>{/if}
            {#if project.roadmap?.length}<span class="muted small">{project.roadmap.length} roadmap</span>{/if}
          </div>
        </div>
      </article>
    {/each}
    {#if projects.length === 0}
      <p class="empty">No projects yet. Create one, or let an agent add it to the ledger.</p>
    {/if}
  </div>
</div>

{#if creating}
  <div class="modal-backdrop" role="presentation" onclick={() => (creating = false)}>
    <div class="modal" role="dialog" aria-modal="true" aria-label="New project" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <header>
        <h2>New project</h2>
        <button class="icon-button" onclick={() => (creating = false)} title="Close">×</button>
      </header>
      <div class="modal-grid">
        <label>ID<input class="field" bind:value={newId} placeholder="mission-control" /></label>
        <label>Name<input class="field" bind:value={newName} placeholder="Mission Control" /></label>
        <label class="full">Description<input class="field" bind:value={newDescription} placeholder="What is this?" /></label>
        <label class="full">Stack (comma-separated)<input class="field" bind:value={newStack} placeholder="Next.js, TypeScript" /></label>
        <label class="full">Notes<input class="field" bind:value={newNotes} /></label>
      </div>
      <footer>
        <button class="button secondary" onclick={() => (creating = false)}>Cancel</button>
        <button class="button primary" onclick={submit} disabled={!newId.trim()}>Create</button>
      </footer>
    </div>
  </div>
{/if}
