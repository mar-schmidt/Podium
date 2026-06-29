<script lang="ts">
  import { onMount } from "svelte";
  import {
    createTask,
    listProjects,
    listTasks,
    startTask,
    taskSession,
    updateTask,
  } from "../lib/api";
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

  const columns: { key: TaskStatus; label: string }[] = [
    { key: "backlog", label: "Backlog" },
    { key: "in_progress", label: "In Progress" },
    { key: "review", label: "Review" },
    { key: "done", label: "Done" },
  ];

  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  let projectFilter = $state("all");
  let error = $state<string | null>(null);
  let dragId = $state<string>("");

  // New-task modal state.
  let creating = $state(false);
  let ntProject = $state("");
  let ntTitle = $state("");
  let ntBody = $state("");
  let ntAgent = $state("");
  let ntPickup = $state("");

  onMount(load);

  async function load() {
    try {
      [tasks, projects] = await Promise.all([listTasks(), listProjects()]);
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
    return projects.find((p) => p.id === id)?.name ?? id;
  }

  function taskPrompt(task: Task) {
    return task.Body.trim() ? `${task.Title}\n\n${task.Body}` : task.Title;
  }

  async function assign(task: Task, agent: string) {
    try {
      await updateTask(task.ID, { assigned_agent: agent });
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
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
    try {
      const session = await startTask(task.ID);
      onOpenChat({ sessionId: session.ID, seed: taskPrompt(task) });
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function openInChat(task: Task) {
    error = null;
    try {
      const session = await taskSession(task.ID);
      if (session) {
        onOpenChat({ sessionId: session.ID });
      } else {
        await start(task);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function submitNewTask() {
    error = null;
    try {
      await createTask({
        project_id: ntProject,
        title: ntTitle.trim(),
        body: ntBody.trim(),
        assigned_agent: ntAgent,
        pickup_at: ntPickup ? new Date(ntPickup).toISOString() : "",
      });
      creating = false;
      ntProject = ntTitle = ntBody = ntAgent = ntPickup = "";
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function onDrop(status: TaskStatus) {
    const task = tasks.find((t) => t.ID === dragId);
    dragId = "";
    if (task) void move(task, status);
  }
</script>

<div class="page roadmap">
  <header class="page-head">
    <div>
      <h1>Roadmap</h1>
      <p>Plan work and assign it to a colleague. Start a card to run it on demand, or give it a pickup time to schedule it.</p>
    </div>
    <div class="head-actions">
      <select class="field compact" bind:value={projectFilter} aria-label="Project filter">
        <option value="all">all projects</option>
        {#each projects as p}<option value={p.id}>{p.name}</option>{/each}
      </select>
      <button class="button primary" onclick={() => (creating = true)}>+ New task</button>
    </div>
  </header>

  {#if error}<div class="error">{error}</div>{/if}

  <div class="board">
    {#each columns as column}
      <section
        class="column"
        role="list"
        ondragover={(e) => e.preventDefault()}
        ondrop={() => onDrop(column.key)}
      >
        <div class="column-head">
          <span>{column.label}</span>
          <span class="count">{tasksFor(column.key).length}</span>
        </div>
        <div class="column-body">
          {#each tasksFor(column.key) as task (task.ID)}
            <article
              class="task-card"
              draggable="true"
              role="listitem"
              ondragstart={() => (dragId = task.ID)}
            >
              {#if task.ProjectID}<span class="project-tag">{projectName(task.ProjectID)}</span>{/if}
              <h4>{task.Title}</h4>
              <div class="task-foot">
                <select
                  class="field compact assign"
                  value={task.AssignedAgent}
                  aria-label="Assign agent"
                  onchange={(e) => assign(task, (e.currentTarget as HTMLSelectElement).value)}
                >
                  <option value="">unassigned</option>
                  {#each agents as agent}<option value={agent.Name}>{agent.Name}</option>{/each}
                </select>
                {#if task.PickupAt}
                  <span class="chip small" title={task.PickupAt}>pickup</span>
                {:else}
                  <span class="chip small">on demand</span>
                {/if}
              </div>
              <div class="task-actions">
                {#if task.Status === "backlog"}
                  <button class="button secondary tiny" disabled={!task.AssignedAgent} onclick={() => start(task)}>Start</button>
                {:else}
                  <button class="button secondary tiny" onclick={() => openInChat(task)}>Open in chat</button>
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/each}
  </div>
</div>

{#if creating}
  <div class="modal-backdrop" role="presentation" onclick={() => (creating = false)}>
    <div class="modal" role="dialog" aria-modal="true" aria-label="New task" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <header>
        <h2>New task</h2>
        <button class="icon-button" onclick={() => (creating = false)} title="Close">×</button>
      </header>
      <div class="modal-grid">
        <label class="full">Title<input class="field" bind:value={ntTitle} placeholder="Add dark mode to the dashboard" /></label>
        <label class="full">Details<input class="field" bind:value={ntBody} placeholder="Optional context for the agent" /></label>
        <label>Project
          <select class="field" bind:value={ntProject}>
            <option value="">none</option>
            {#each projects as p}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </label>
        <label>Assign
          <select class="field" bind:value={ntAgent}>
            <option value="">unassigned</option>
            {#each agents as agent}<option value={agent.Name}>{agent.Name}</option>{/each}
          </select>
        </label>
        <label class="full">Scheduled pickup (optional)<input class="field" type="datetime-local" bind:value={ntPickup} /></label>
      </div>
      <footer>
        <button class="button secondary" onclick={() => (creating = false)}>Cancel</button>
        <button class="button primary" onclick={submitNewTask} disabled={!ntTitle.trim()}>Create task</button>
      </footer>
    </div>
  </div>
{/if}
