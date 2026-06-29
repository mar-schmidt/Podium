<script lang="ts">
  import { onMount } from "svelte";
  import { getHealth, hireAgent, listAgents } from "./lib/api";
  import type { Agent, Health, PermissionMode, Provider } from "./lib/types";
  import Chat from "./pages/Chat.svelte";
  import Roadmap from "./pages/Roadmap.svelte";
  import Agents from "./pages/Agents.svelte";
  import Schedules from "./pages/Schedules.svelte";
  import Projects from "./pages/Projects.svelte";

  type Route = "chat" | "roadmap" | "agents" | "schedules" | "projects";

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  const nav: { key: Route; label: string; icon: string }[] = [
    { key: "chat", label: "Chat", icon: "💬" },
    { key: "roadmap", label: "Roadmap", icon: "🗂" },
    { key: "agents", label: "Agents", icon: "👥" },
    { key: "schedules", label: "Schedules", icon: "⏰" },
    { key: "projects", label: "Projects", icon: "📁" },
  ];

  let route = $state<Route>("chat");
  let health = $state<Health | null>(null);
  let chatStatus = $state<"connecting" | "live" | "offline">("connecting");
  let agents = $state<Agent[]>([]);
  let chatTarget = $state<ChatTarget | null>(null);

  // Hire modal.
  let hireOpen = $state(false);
  let hireName = $state("jared");
  let hireProvider = $state<Provider>("claude");
  let hirePermission = $state<PermissionMode>("approve");
  let hireModel = $state("");
  let hireEffort = $state("medium");
  let hireError = $state<string | null>(null);

  onMount(async () => {
    try {
      health = await getHealth();
    } catch {
      health = null;
    }
    await refreshAgents();
  });

  async function refreshAgents() {
    try {
      agents = await listAgents();
    } catch {
      // leave agents as-is
    }
  }

  function openChat(target: ChatTarget) {
    chatTarget = target;
    route = "chat";
  }

  async function submitHire() {
    hireError = null;
    try {
      const agent = await hireAgent({
        name: hireName.trim(),
        provider: hireProvider,
        model: hireModel.trim(),
        effort: hireEffort,
        permission_mode: hirePermission,
      });
      agents = [agent, ...agents.filter((a) => a.Name !== agent.Name)];
      hireOpen = false;
    } catch (e) {
      hireError = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<main class="app-shell">
  <aside class="nav">
    <div class="brand">
      <div class="logo">◆</div>
      <div>
        <h1>Podium</h1>
        <p>conductor</p>
      </div>
    </div>

    <nav class="nav-links">
      {#each nav as item}
        <button class:active={route === item.key} class="nav-link" onclick={() => (route = item.key)}>
          <span class="nav-icon">{item.icon}</span>
          {item.label}
        </button>
      {/each}
    </nav>

    <div class="nav-foot">
      <div class="daemon-status">
        <span class:live={chatStatus === "live"} class="status-dot"></span>
        <div>
          <strong>podiumd {chatStatus === "live" ? "live" : chatStatus}</strong>
          <small>{health ? `${health.version} (${health.commit})` : "—"}</small>
        </div>
      </div>
      <button class="button ghost" onclick={() => (hireOpen = true)}>+ Hire agent</button>
    </div>
  </aside>

  <section class="view">
    {#if route === "chat"}
      <Chat
        {agents}
        target={chatTarget}
        onConsumeTarget={() => (chatTarget = null)}
        onStatus={(s) => (chatStatus = s)}
      />
    {:else if route === "roadmap"}
      <Roadmap {agents} onOpenChat={openChat} />
    {:else if route === "agents"}
      <Agents {agents} onHire={() => (hireOpen = true)} />
    {:else if route === "schedules"}
      <Schedules />
    {:else if route === "projects"}
      <Projects />
    {/if}
  </section>

  {#if hireOpen}
    <div class="modal-backdrop" role="presentation" onclick={() => (hireOpen = false)}>
      <div class="modal" role="dialog" aria-modal="true" aria-label="Hire agent" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
        <header>
          <h2>Hire an agent</h2>
          <button class="icon-button" onclick={() => (hireOpen = false)} title="Close">×</button>
        </header>
        {#if hireError}<div class="error">{hireError}</div>{/if}
        <div class="modal-grid">
          <label class="full">Name<input class="field" bind:value={hireName} placeholder="e.g. atlas" /></label>
          <label>Backend
            <select class="field" bind:value={hireProvider}>
              <option value="claude">claude</option>
              <option value="codex">codex</option>
            </select>
          </label>
          <label>Permission
            <select class="field" bind:value={hirePermission}>
              <option value="approve">approve · safe</option>
              <option value="yolo">yolo · full access</option>
            </select>
          </label>
          <label>Effort
            <select class="field" bind:value={hireEffort}>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="xhigh">xhigh</option>
              <option value="max">max</option>
            </select>
          </label>
          <label>Model<input class="field" bind:value={hireModel} placeholder="provider default" /></label>
        </div>
        <footer>
          <button class="button secondary" onclick={() => (hireOpen = false)}>Cancel</button>
          <button class="button primary" onclick={submitHire} disabled={!hireName.trim()}>Create agent</button>
        </footer>
      </div>
    </div>
  {/if}
</main>
