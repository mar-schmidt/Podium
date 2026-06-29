<script lang="ts">
  import { onMount } from "svelte";
  import {
    createProject,
    createTask,
    listProjects,
    listTasks,
    startTask,
    taskSession,
    updateTask,
  } from "../lib/api";
  import { agentGradient, avatarStyle, initial, projectColor } from "../lib/theme";
  import type { Agent, Project, Task, TaskStatus } from "../lib/types";

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  let {
    agents = [],
    onOpenChat = (_t: ChatTarget) => {},
  }: { agents?: Agent[]; onOpenChat?: (t: ChatTarget) => void } = $props();

  const COLUMNS: { key: TaskStatus; label: string; dot: string }[] = [
    { key: "backlog", label: "Backlog", dot: "#C9BBAA" },
    { key: "in_progress", label: "In Progress", dot: "#3F8F7E" },
    { key: "review", label: "Review", dot: "#6E86C9" },
    { key: "done", label: "Done", dot: "#4F9E78" },
  ];

  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  let projectFilter = $state("all");
  let error = $state<string | null>(null);
  let dragId = $state<string>("");
  let dragging = $state(false);
  let openCard = $state<Task | null>(null);
  let busy = $state(false);

  // New-task modal.
  let creating = $state(false);
  let ntProject = $state("");
  let ntTitle = $state("");
  let ntBody = $state("");
  let ntAgent = $state("");
  let ntScheduled = $state(false);
  let ntPickup = $state("");
  let newProjName = $state("");
  let newProjOpen = $state(false);

  onMount(load);

  async function load() {
    try {
      [tasks, projects] = await Promise.all([listTasks(), listProjects()]);
      if (!ntAgent && agents.length) ntAgent = agents[0].Name;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  const visibleTasks = $derived(
    projectFilter === "all" ? tasks : tasks.filter((t) => t.ProjectID === projectFilter),
  );

  function tasksFor(status: TaskStatus) {
    return visibleTasks.filter((t) => t.Status === status);
  }

  function projectName(id: string) {
    return projects.find((p) => p.id === id)?.name ?? id ?? "no project";
  }

  function taskPrompt(task: Task) {
    return task.Body.trim() ? `${task.Title}\n\n${task.Body}` : task.Title;
  }

  function hasSession(task: Task) {
    return task.Status !== "backlog";
  }

  async function move(task: Task, status: TaskStatus) {
    if (task.Status === status) return;
    try {
      await updateTask(task.ID, { status });
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function start(task: Task) {
    error = null;
    busy = true;
    try {
      const session = await startTask(task.ID);
      onOpenChat({ sessionId: session.ID, seed: taskPrompt(task) });
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
      openCard = null;
    }
  }

  async function openInChat(task: Task) {
    error = null;
    try {
      const session = await taskSession(task.ID);
      if (session) onOpenChat({ sessionId: session.ID });
      else await start(task);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
    openCard = null;
  }

  function onDrop(status: TaskStatus) {
    const task = tasks.find((t) => t.ID === dragId);
    dragId = "";
    dragging = false;
    if (!task) return;
    if (status === "in_progress" && task.Status === "backlog") void start(task);
    else void move(task, status);
  }

  function chip(task: Task): { label: string; bg: string; fg: string } {
    if (task.Status === "done") return { label: "✓ done", bg: "#E3F1EC", fg: "#3F7A5F" };
    if (task.Status === "in_progress") return { label: "● working", bg: "#E3F1EC", fg: "#2F6E60" };
    if (task.Status === "review") return { label: "awaiting review", bg: "#EEEAFB", fg: "#5847B8" };
    if (task.PickupAt) return { label: "⟳ scheduled", bg: "#FBF1DD", fg: "#9A6E1E" };
    return { label: "on demand", bg: "#F1EADF", fg: "#8A7560" };
  }

  function chipStyle(t: Task): string {
    const c = chip(t);
    return `padding:3px 9px;border-radius:999px;background:${c.bg};color:${c.fg};font:600 10px 'JetBrains Mono',monospace;white-space:nowrap`;
  }

  function cardStyle(t: Task): string {
    const done = t.Status === "done";
    const bg = done ? "#F7F4EF" : "#FFFDFB";
    const border = done ? "#E7DFD4" : "#EDE4D9";
    const extra = done ? ";opacity:.62;filter:saturate(.7)" : "";
    return `background:${bg};border:1px solid ${border};border-radius:14px;padding:13px 14px;cursor:grab;box-shadow:0 1px 2px rgba(43,37,32,.04),0 8px 20px -16px rgba(43,37,32,.3)${extra}`;
  }

  async function submitNewTask() {
    error = null;
    try {
      await createTask({
        project_id: ntProject,
        title: ntTitle.trim(),
        body: ntBody.trim(),
        assigned_agent: ntAgent,
        pickup_at: ntScheduled && ntPickup ? new Date(ntPickup).toISOString() : "",
      });
      creating = false;
      ntTitle = ntBody = "";
      ntScheduled = false;
      ntPickup = "";
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function addInlineProject() {
    const name = newProjName.trim();
    if (!name) return;
    const id = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    try {
      await createProject({ id, name, description: "", stack: [], notes: "" });
      await load();
      ntProject = id;
      newProjName = "";
      newProjOpen = false;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function projChipStyle(id: string, sel: boolean): string {
    void id;
    return (
      "display:inline-flex;align-items:center;gap:7px;padding:8px 13px;border-radius:11px;cursor:pointer;font:600 12.5px 'Hanken Grotesk';" +
      (sel ? "border:1px solid #BFE0D6;background:#E3F1EC;color:#2F6E60" : "border:1px solid #EAE0D4;background:#fff;color:#6F6459")
    );
  }

  function agentChipStyle(sel: boolean): string {
    return (
      "display:inline-flex;align-items:center;gap:8px;padding:5px 13px 5px 5px;border-radius:11px;cursor:pointer;font:600 12.5px 'Hanken Grotesk';" +
      (sel ? "border:1px solid #BFE0D6;background:#E3F1EC;color:#2F6E60" : "border:1px solid #EAE0D4;background:#fff;color:#6F6459")
    );
  }
</script>

<div class="page roadmap-page">
  <header class="page-head">
    <div>
      <h1>Roadmap</h1>
      <p>Plan work and assign it to a colleague. Give it a pickup time to schedule it, or drag a card onto <b style="color:#2F6E60">Start</b> to run it on demand.</p>
    </div>
    <span class="spacer"></span>
    <div class="dd-wrap">
      <select class="mini-select" bind:value={projectFilter} aria-label="Project filter">
        <option value="all">all projects</option>
        {#each projects as p}<option value={p.id}>{p.name}</option>{/each}
      </select>
    </div>
    <button class="head-cta" onclick={() => (creating = true)}>+ New task</button>
  </header>

  {#if error}<div class="error-banner" style="margin-bottom:14px">{error}</div>{/if}

  <div class="board">
    {#each COLUMNS as col}
      {@const isStart = col.key === "in_progress" && dragging}
      <div class="col" role="list" ondragover={(e) => e.preventDefault()} ondrop={() => onDrop(col.key)}>
        <div class="col-head">
          <span class="col-dot" style="background:{isStart ? '#2E8E78' : col.dot}"></span>
          <span class="col-label" style="color:{isStart ? '#2A7A68' : '#2B2520'}">{isStart ? "Start" : col.label}</span>
          <span class="col-count mono">{tasksFor(col.key).length}</span>
        </div>
        <div class="col-zone" class:hot={isStart} class:donecol={col.key === "done"}>
          {#each tasksFor(col.key) as task (task.ID)}
            <div
              class="task-card"
              role="button"
              tabindex="0"
              draggable="true"
              style={cardStyle(task)}
              ondragstart={() => { dragId = task.ID; dragging = true; }}
              ondragend={() => { dragging = false; }}
              onclick={() => (openCard = task)}
              onkeydown={(e) => { if (e.key === "Enter") openCard = task; }}
            >
              <div class="tc-proj">
                <span class="proj-dot" style="background:{projectColor(task.ProjectID)}"></span>
                <span class="tc-proj-name mono">{projectName(task.ProjectID)}</span>
              </div>
              <div class="tc-title">{task.Title}</div>
              <div class="tc-foot">
                <span style={avatarStyle(agentGradient(task.AssignedAgent || "?"), 22, 7, 10)}>{initial(task.AssignedAgent || "?")}</span>
                <span class="tc-agent">{task.AssignedAgent || "unassigned"}</span>
                <span style={chipStyle(task)}>{chip(task).label}</span>
              </div>
              {#if hasSession(task)}
                <button class="tc-openchat" onclick={(e) => { e.stopPropagation(); openInChat(task); }}>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>
                  Open in chat
                </button>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</div>

<!-- ===== Card detail modal ===== -->
{#if openCard}
  {@const c = chip(openCard)}
  <div class="modal-backdrop" role="presentation" onclick={() => (openCard = null)}>
    <div class="modal-card card-modal" role="dialog" aria-modal="true" aria-label="Task" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="cm-head">
        <div class="cm-proj">
          <span class="proj-dot" style="background:{projectColor(openCard.ProjectID)}"></span>
          <span class="mono cm-proj-name">{projectName(openCard.ProjectID)}</span>
          <span class="spacer"></span>
          <span style="padding:3px 9px;border-radius:999px;background:{c.bg};color:{c.fg};font:600 10px 'JetBrains Mono',monospace">{c.label}</span>
        </div>
        <div class="cm-title">{openCard.Title}</div>
      </div>
      <div class="cm-body">
        {#if openCard.Body}
          <div class="label-mono" style="margin-bottom:7px">prompt</div>
          <div class="cm-prompt">{openCard.Body}</div>
        {/if}
        <div class="cm-assignee">
          <span style={avatarStyle(agentGradient(openCard.AssignedAgent || "?"), 34, 11, 14)}>{initial(openCard.AssignedAgent || "?")}</span>
          <div style="flex:1">
            <div class="cm-agent-name">{openCard.AssignedAgent || "unassigned"}</div>
            <div class="mono cm-agent-sub">assignee · {openCard.Status.replace("_", " ")}</div>
          </div>
        </div>
        <div class="cm-actions">
          {#if hasSession(openCard)}
            <button class="cm-primary" onclick={() => openInChat(openCard!)}>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>
              Open in chat
            </button>
          {:else}
            <button class="cm-primary" disabled={busy || !openCard.AssignedAgent} onclick={() => start(openCard!)}>Start now →</button>
          {/if}
        </div>
        {#if hasSession(openCard) && openCard.Status !== "done"}
          <button class="cm-done" onclick={() => move(openCard!, "done")}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg>
            Mark as done
          </button>
        {/if}
        {#if openCard.Status === "done"}
          <button class="cm-reopen" onclick={() => move(openCard!, "review")}>Reopen task</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<!-- ===== New task modal ===== -->
{#if creating}
  <div class="modal-backdrop" role="presentation" onclick={() => (creating = false)}>
    <div class="modal-card nt-modal" role="dialog" aria-modal="true" aria-label="New task" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="modal-head">
        <div class="modal-title">New task</div>
        <div class="modal-sub">Assign work to a colleague. Give it a pickup time to schedule it, or leave it on demand and drag it onto Start when you're ready.</div>
      </div>
      <div class="modal-body" style="max-height:74vh;overflow-y:auto">
        <div class="label-mono" style="margin-bottom:8px">title</div>
        <input class="field-input" bind:value={ntTitle} placeholder="e.g. Add a settings page" />

        <div class="label-mono" style="margin:18px 0 8px">prompt for the agent <span style="color:#B6AA9C;text-transform:none;font-weight:400">— the full instructions to run</span></div>
        <textarea class="field-area" rows="6" bind:value={ntBody} placeholder="Describe the task in detail. This is sent to the agent verbatim when the task starts — paste a spec, acceptance criteria, file paths, anything." style="min-height:120px"></textarea>

        <div class="label-mono" style="margin:18px 0 8px">project</div>
        <div class="chip-wrap">
          {#each projects as p}
            <button style={projChipStyle(p.id, ntProject === p.id)} onclick={() => (ntProject = p.id)}>
              <span class="proj-dot" style="background:{projectColor(p.id)}"></span>{p.name}
            </button>
          {/each}
          <button class="new-proj-chip" onclick={() => (newProjOpen = true)}><span style="font-size:15px;line-height:1">+</span> New project</button>
        </div>
        {#if newProjOpen}
          <div style="display:flex;gap:8px;margin-top:9px">
            <input class="field-input" style="flex:1;border-color:#BCDCCF;background:#F1F7F4" bind:value={newProjName} placeholder="New project name…" onkeydown={(e) => { if (e.key === "Enter") { e.preventDefault(); addInlineProject(); } }} />
            <button class="head-cta" style="padding:0 17px" onclick={addInlineProject}>Create</button>
          </div>
        {/if}

        <div class="label-mono" style="margin:18px 0 8px">assignee</div>
        <div class="chip-wrap">
          {#each agents as a}
            <button style={agentChipStyle(ntAgent === a.Name)} onclick={() => (ntAgent = a.Name)}>
              <span style={avatarStyle(agentGradient(a.Name), 20, 6, 9)}>{initial(a.Name)}</span>{a.Name}
            </button>
          {/each}
        </div>

        <div class="label-mono" style="margin:18px 0 8px">when</div>
        <div style="display:flex;gap:9px">
          <button style={projChipStyle("", !ntScheduled)} onclick={() => (ntScheduled = false)}>On demand</button>
          <button style={projChipStyle("", ntScheduled)} onclick={() => (ntScheduled = true)}>Scheduled</button>
        </div>
        {#if ntScheduled}
          <input class="field-input" type="datetime-local" style="margin-top:9px" bind:value={ntPickup} />
          <div style="font:400 11.5px/1.5 'Hanken Grotesk';color:#9A8E80;margin-top:7px">Scheduled tasks sit in <b>Backlog</b> with a pickup time and start automatically when due.</div>
        {/if}

        <button class="modal-cta" disabled={!ntTitle.trim()} onclick={submitNewTask}>Add to roadmap</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .mini-select {
    padding: 7px 13px;
    border-radius: 999px;
    background: #fff;
    border: 1px solid var(--field-line);
    font: 500 12.5px "Hanken Grotesk";
    color: var(--muted);
    cursor: pointer;
    outline: none;
  }

  .board {
    flex: 1;
    display: flex;
    gap: 16px;
    overflow-x: auto;
    min-height: 0;
    padding-bottom: 6px;
  }

  .col {
    width: 288px;
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .col-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 4px 6px 12px;
  }

  .col-dot {
    width: 9px;
    height: 9px;
    border-radius: 3px;
    flex: none;
  }

  .col-label {
    font: 700 13px "Hanken Grotesk";
  }

  .col-count {
    font: 600 12px "JetBrains Mono", monospace;
    color: var(--faint);
  }

  .col-zone {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 11px;
    padding: 2px 4px 4px;
    border-radius: 14px;
    background: rgba(255, 253, 251, 0.45);
    border: 2px solid transparent;
    transition: all 0.15s;
    min-height: 80px;
  }

  .col-zone.donecol {
    background: rgba(79, 158, 120, 0.07);
  }

  .col-zone.hot {
    background: rgba(63, 143, 126, 0.13);
    border: 2px dashed #7fc3b2;
    padding: 8px 5px;
  }

  .tc-proj {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 8px;
  }

  .proj-dot {
    width: 9px;
    height: 9px;
    border-radius: 99px;
    flex: none;
  }

  .tc-proj-name {
    font: 500 11px "JetBrains Mono", monospace;
    color: #9a8e80;
  }

  .tc-title {
    font: 600 14px/1.35 "Hanken Grotesk";
    color: var(--ink);
  }

  .tc-foot {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
  }

  .tc-agent {
    font: 500 12px "Hanken Grotesk";
    color: var(--muted);
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tc-openchat {
    margin-top: 11px;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    border: 1px solid #cfe3d8;
    background: #eaf3ef;
    color: var(--teal-deep);
    border-radius: 9px;
    padding: 7px;
    font: 600 12px "Hanken Grotesk";
    cursor: pointer;
  }

  /* card modal */
  .card-modal {
    width: 440px;
    max-width: 92vw;
  }

  .cm-head {
    padding: 20px 22px 16px;
    border-bottom: 1px solid #f1eae0;
  }

  .cm-proj {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }

  .cm-proj-name {
    font: 500 11px "JetBrains Mono", monospace;
    color: #9a8e80;
  }

  .cm-title {
    font: 800 19px/1.3 "Hanken Grotesk";
    color: var(--ink);
  }

  .cm-body {
    padding: 18px 22px;
  }

  .cm-prompt {
    font: 400 13.5px/1.65 "Hanken Grotesk";
    color: #5a5048;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
    background: var(--surface-3);
    border: 1px solid var(--line-3);
    border-radius: 12px;
    padding: 13px 15px;
  }

  .cm-assignee {
    display: flex;
    align-items: center;
    gap: 11px;
    margin-top: 14px;
    padding: 13px;
    background: var(--surface-3);
    border: 1px solid var(--line-3);
    border-radius: 13px;
  }

  .cm-agent-name {
    font: 600 14px "Hanken Grotesk";
  }

  .cm-agent-sub {
    font: 400 11px "JetBrains Mono", monospace;
    color: #9a8e80;
  }

  .cm-actions {
    display: flex;
    gap: 9px;
    margin-top: 18px;
  }

  .cm-primary {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    border: none;
    border-radius: 11px;
    padding: 11px;
    background: var(--teal);
    color: #fff;
    font: 600 14px "Hanken Grotesk";
    cursor: pointer;
    box-shadow: 0 6px 14px -6px rgba(63, 143, 126, 0.7);
  }

  .cm-done {
    width: 100%;
    margin-top: 9px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    border: 1px solid #cfe3d8;
    border-radius: 11px;
    padding: 10px;
    background: #eff6f1;
    color: #3f7a5f;
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
  }

  .cm-reopen {
    width: 100%;
    margin-top: 9px;
    border: 1px solid var(--field-line);
    border-radius: 11px;
    padding: 10px;
    background: #fff;
    color: var(--muted-2);
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
  }

  /* new task modal */
  .nt-modal {
    width: 486px;
    max-width: 94vw;
  }

  .chip-wrap {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .new-proj-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 13px;
    border-radius: 11px;
    cursor: pointer;
    font: 600 12.5px "Hanken Grotesk";
    border: 1.5px dashed #c9b89f;
    background: transparent;
    color: #a8825e;
  }
</style>
