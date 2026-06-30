<script lang="ts">
  import { onMount } from "svelte";
  import {
    createProject,
    describeProject,
    listProjects,
    listTasks,
    updateProject,
  } from "../lib/api";
  import { PROJECT_COLORS, projectColor } from "../lib/theme";
  import type { Agent, Project, Task } from "../lib/types";

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  let {
    agents = [],
    onOpenChat = (_t: ChatTarget) => {},
  }: { agents?: Agent[]; onOpenChat?: (t: ChatTarget) => void } = $props();

  let projects = $state<Project[]>([]);
  let tasks = $state<Task[]>([]);
  let error = $state<string | null>(null);
  // Per-project description drafts (edited locally, saved on demand).
  let drafts = $state<Record<string, string>>({});
  let busyDescribe = $state<string>("");
  let savingDesc = $state<string>("");
  // Which agent's engine drafts descriptions.
  let writerAgent = $state("");

  // New-project modal.
  let creating = $state(false);
  let npName = $state("");
  let npDescription = $state("");
  let npStack = $state("");
  let npNotes = $state("");

  onMount(load);

  async function load() {
    try {
      [projects, tasks] = await Promise.all([listProjects(), listTasks()]);
      drafts = Object.fromEntries(projects.map((p) => [p.id, p.description]));
      if (!writerAgent && agents.length) writerAgent = agents[0].Name;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function color(p: Project): string {
    return p.color || projectColor(p.id);
  }

  function taskCount(id: string): number {
    return tasks.filter((t) => t.ProjectID === id).length;
  }

  function meta(p: Project): string {
    const t = taskCount(p.id);
    const b = p.backlog?.length ?? 0;
    return `${t} task${t === 1 ? "" : "s"} · ${b} in backlog`;
  }

  function dirty(p: Project): boolean {
    return (drafts[p.id] ?? "") !== p.description;
  }

  async function setColor(p: Project, c: string) {
    try {
      const updated = await updateProject(p.id, { color: c });
      projects = projects.map((x) => (x.id === p.id ? updated : x));
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function saveDesc(p: Project) {
    savingDesc = p.id;
    error = null;
    try {
      const updated = await updateProject(p.id, { description: drafts[p.id] ?? "" });
      projects = projects.map((x) => (x.id === p.id ? updated : x));
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      savingDesc = "";
    }
  }

  async function describe(p: Project) {
    if (!writerAgent) {
      error = "Hire an agent first — descriptions are drafted by an agent's engine.";
      return;
    }
    busyDescribe = p.id;
    error = null;
    try {
      const text = await describeProject(p.id, writerAgent);
      drafts = { ...drafts, [p.id]: text };
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busyDescribe = "";
    }
  }

  async function submit() {
    error = null;
    const name = npName.trim();
    if (!name) return;
    const id = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    try {
      await createProject({
        id,
        name,
        description: npDescription.trim(),
        stack: npStack.split(",").map((s) => s.trim()).filter(Boolean),
        notes: npNotes.trim(),
      });
      creating = false;
      npName = npDescription = npStack = npNotes = "";
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
      <p>Name your projects, give each a colour, and write a short description. Colours show up everywhere the project appears.</p>
    </div>
    <span class="spacer"></span>
    {#if agents.length > 1}
      <label class="writer-pick mono">
        ✦ writer
        <select bind:value={writerAgent}>
          {#each agents as a}<option value={a.Name}>{a.Name}</option>{/each}
        </select>
      </label>
    {/if}
    <button class="head-cta" onclick={() => (creating = true)}>+ New project</button>
  </header>

  {#if error}<div class="error-banner" style="margin-bottom:14px">{error}</div>{/if}

  <div class="proj-grid">
    {#each projects as p (p.id)}
      <article class="proj-card">
        <div class="pc-head">
          <span class="pc-bigdot" style="background:{color(p)}"></span>
          <span class="pc-name">{p.name}</span>
        </div>
        <div class="pc-id mono">{p.id}</div>

        <div class="label-mono" style="margin:16px 0 8px">colour</div>
        <div class="pc-swatches">
          {#each PROJECT_COLORS as c}
            <button
              class="swatch"
              style="background:{c};box-shadow:{c === color(p) ? '0 0 0 2px #FFFDFB,0 0 0 4px ' + c : 'inset 0 0 0 1px rgba(0,0,0,.06)'}"
              aria-label="Set colour"
              onclick={() => setColor(p, c)}
            ></button>
          {/each}
        </div>

        <div class="pc-desc-head">
          <span class="label-mono" style="flex:1">description</span>
          <button class="ai-btn" disabled={busyDescribe === p.id} onclick={() => describe(p)}>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l1.9 4.6L18.5 9.5 13.9 11.4 12 16l-1.9-4.6L5.5 9.5l4.6-1.9z" /></svg>
            {busyDescribe === p.id ? "Writing…" : "Help me write"}
          </button>
        </div>
        <textarea
          class="field-area"
          rows="3"
          value={drafts[p.id] ?? ""}
          oninput={(e) => (drafts = { ...drafts, [p.id]: (e.currentTarget as HTMLTextAreaElement).value })}
          placeholder="What is this project? One or two sentences."
          style="min-height:74px"
        ></textarea>
        {#if dirty(p)}
          <div class="pc-save-row">
            <button class="pc-cancel" onclick={() => (drafts = { ...drafts, [p.id]: p.description })}>Reset</button>
            <button class="pc-save" disabled={savingDesc === p.id} onclick={() => saveDesc(p)}>{savingDesc === p.id ? "Saving…" : "Save description"}</button>
          </div>
        {/if}

        {#if p.stack && p.stack.length}
          <div class="pc-chips">
            {#each p.stack as tech}<span class="pc-tech mono">{tech}</span>{/each}
          </div>
        {/if}

        <div class="pc-foot">
          <span class="pc-meta mono">{meta(p)}</span>
          <button class="pc-view" onclick={() => onOpenChat({})}>View sessions →</button>
        </div>
      </article>
    {/each}
    {#if projects.length === 0}
      <p class="empty-note">No projects yet. Create one, or let an agent add it to the ledger.</p>
    {/if}
  </div>
</div>

{#if creating}
  <div class="modal-backdrop" role="presentation" onclick={() => (creating = false)}>
    <div class="modal-card np-modal" role="dialog" aria-modal="true" aria-label="New project" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="modal-head">
        <div class="modal-title">New project</div>
        <div class="modal-sub">Tracked in <span class="mono">~/.podium/projects/projects.yaml</span>. Any agent can pick it up.</div>
      </div>
      <div class="modal-body">
        <div class="label-mono" style="margin-bottom:8px">name</div>
        <input class="field-input" bind:value={npName} placeholder="Mission Control" />

        <div class="label-mono" style="margin:18px 0 8px">description</div>
        <textarea class="field-area" rows="2" bind:value={npDescription} placeholder="What is this project? One or two sentences." style="min-height:60px"></textarea>

        <div class="label-mono" style="margin:18px 0 8px">stack (comma-separated)</div>
        <input class="field-input" bind:value={npStack} placeholder="Next.js, TypeScript, Tailwind" />

        <div class="label-mono" style="margin:18px 0 8px">notes</div>
        <textarea class="field-area" rows="2" bind:value={npNotes} placeholder="Anything agents should know." style="min-height:56px"></textarea>

        <button class="modal-cta" disabled={!npName.trim()} onclick={submit}>Create project</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .writer-pick {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font: 500 11.5px "JetBrains Mono", monospace;
    color: #6b53a8;
    background: #f4eff8;
    border: 1px solid #e2d7e9;
    border-radius: 999px;
    padding: 5px 6px 5px 12px;
  }

  .writer-pick select {
    border: none;
    background: #fff;
    border-radius: 999px;
    padding: 4px 8px;
    font: 500 11.5px "JetBrains Mono", monospace;
    color: #6b53a8;
    outline: none;
    cursor: pointer;
  }

  .proj-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 380px), 1fr));
    gap: 18px;
    max-width: 1180px;
  }

  .proj-card {
    background: var(--surface);
    border: 1px solid var(--line-2);
    border-radius: 20px;
    padding: 20px;
    box-shadow: 0 1px 2px rgba(43, 37, 32, 0.04), 0 16px 40px -28px rgba(43, 37, 32, 0.22);
  }

  .pc-head {
    display: flex;
    align-items: center;
    gap: 11px;
  }

  .pc-bigdot {
    width: 14px;
    height: 14px;
    border-radius: 99px;
    flex: none;
  }

  .pc-name {
    font: 800 19px "Hanken Grotesk";
    color: var(--ink);
  }

  .pc-id {
    font: 500 11px "JetBrains Mono", monospace;
    color: var(--faint);
    margin: 2px 0 0 25px;
  }

  .pc-swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .swatch {
    width: 24px;
    height: 24px;
    border-radius: 8px;
    cursor: pointer;
    border: none;
  }

  .pc-desc-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 16px 0 8px;
  }

  .ai-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border: 1px solid #e2d7e9;
    background: #f4eff8;
    color: #6b53a8;
    border-radius: 9px;
    padding: 5px 10px;
    font: 600 11.5px "Hanken Grotesk";
    cursor: pointer;
  }

  .pc-save-row {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 9px;
  }

  .pc-cancel {
    border: 1px solid var(--field-line);
    background: #fff;
    border-radius: 9px;
    padding: 6px 12px;
    font: 600 12px "Hanken Grotesk";
    color: var(--muted-2);
    cursor: pointer;
  }

  .pc-save {
    border: none;
    background: var(--teal);
    color: #fff;
    border-radius: 9px;
    padding: 6px 14px;
    font: 600 12px "Hanken Grotesk";
    cursor: pointer;
    box-shadow: 0 6px 14px -6px rgba(63, 143, 126, 0.7);
  }

  .pc-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 12px;
  }

  .pc-tech {
    padding: 3px 9px;
    border-radius: 999px;
    background: var(--surface-3);
    border: 1px solid var(--line-3);
    font: 500 11px "JetBrains Mono", monospace;
    color: #8a7560;
  }

  .pc-foot {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 14px;
    padding-top: 13px;
    border-top: 1px solid #f1eae0;
  }

  .pc-meta {
    font: 500 11.5px "JetBrains Mono", monospace;
    color: #9a8e80;
    flex: 1;
  }

  .pc-view {
    border: 1px solid var(--field-line);
    background: #fff;
    border-radius: 9px;
    padding: 6px 11px;
    font: 600 11.5px "Hanken Grotesk";
    color: var(--teal-deep);
    cursor: pointer;
  }

  .np-modal {
    width: 460px;
    max-width: 92vw;
  }

  @media (max-width: 768px) {
    .writer-pick {
      align-self: stretch;
      justify-content: space-between;
    }

    .writer-pick select {
      min-width: 0;
      max-width: 58vw;
    }

    .proj-card {
      padding: 16px;
    }

    .pc-head {
      align-items: flex-start;
    }

    .pc-name {
      min-width: 0;
      overflow-wrap: anywhere;
    }

    .pc-id {
      margin-left: 25px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .pc-desc-head,
    .pc-foot,
    .pc-save-row {
      align-items: stretch;
      flex-direction: column;
    }

    .ai-btn,
    .pc-view,
    .pc-save,
    .pc-cancel {
      justify-content: center;
      width: 100%;
    }
  }
</style>
