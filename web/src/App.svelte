<script lang="ts">
  import { onMount } from "svelte";
  import { applyUpdate, checkUpdate, getHealth, hireAgent, listAgents } from "./lib/api";
  import type { Agent, Health, PermissionMode, Provider, UpdateStatus } from "./lib/types";
  import Chat from "./pages/Chat.svelte";
  import Roadmap from "./pages/Roadmap.svelte";
  import Agents from "./pages/Agents.svelte";
  import Schedules from "./pages/Schedules.svelte";
  import Projects from "./pages/Projects.svelte";
  import Skills from "./pages/Skills.svelte";
  import Logs from "./pages/Logs.svelte";

  type Route = "chat" | "roadmap" | "projects" | "agents" | "schedules" | "skills" | "logs";

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  const NAV: { key: Route; label: string; icon: string }[] = [
    {
      key: "chat",
      label: "Chat",
      icon: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    },
    {
      key: "roadmap",
      label: "Roadmap",
      icon: '<rect x="3" y="3" width="6" height="18" rx="1.5"/><rect x="10.5" y="3" width="6" height="11" rx="1.5"/><rect x="18" y="3" width="3" height="7" rx="1.5"/>',
    },
    {
      key: "projects",
      label: "Projects",
      icon: '<path d="M3 8a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><circle cx="8.5" cy="13" r="1.4"/>',
    },
    {
      key: "agents",
      label: "Agents",
      icon: '<circle cx="9" cy="8" r="3.2"/><path d="M3.5 20a5.5 5.5 0 0 1 11 0"/><path d="M16 5.2a3.2 3.2 0 0 1 0 5.6"/><path d="M17.5 20a5.5 5.5 0 0 0-2.5-4.6"/>',
    },
    {
      key: "schedules",
      label: "Schedules",
      icon: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    },
    {
      key: "skills",
      label: "Skills",
      icon: '<rect x="3" y="3" width="7" height="7" rx="1.6"/><rect x="14" y="3" width="7" height="7" rx="1.6"/><rect x="3" y="14" width="7" height="7" rx="1.6"/><rect x="14" y="14" width="7" height="7" rx="1.6"/>',
    },
    {
      key: "logs",
      label: "Logs",
      icon: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 13h8"/><path d="M8 17h6"/>',
    },
  ];

  let route = $state<Route>("chat");
  let health = $state<Health | null>(null);
  let update = $state<UpdateStatus | null>(null);
  let updateState = $state<"idle" | "checking" | "available" | "current" | "updating" | "restarting" | "failed">("idle");
  let updateError = $state<string | null>(null);
  let chatStatus = $state<"connecting" | "live" | "offline">("connecting");
  let agents = $state<Agent[]>([]);
  let chatTarget = $state<ChatTarget | null>(null);

  // Hire modal.
  let hireOpen = $state(false);
  let hireName = $state("");
  let hireProvider = $state<Provider>("claude");
  let hirePermission = $state<PermissionMode>("approve");
  let hireError = $state<string | null>(null);

  onMount(async () => {
    try {
      health = await getHealth();
    } catch {
      health = null;
    }
    await refreshAgents();
    await refreshUpdate();
  });

  async function refreshAgents() {
    try {
      agents = await listAgents();
    } catch {
      // leave agents as-is
    }
  }

  async function refreshUpdate() {
    updateState = "checking";
    updateError = null;
    try {
      update = await checkUpdate();
      updateState = update.update_available ? "available" : "current";
    } catch (e) {
      update = null;
      updateState = "failed";
      updateError = e instanceof Error ? e.message : String(e);
    }
  }

  async function runUpdate() {
    if (!update) return;
    const warning = update.blocking_reason
      ? `${update.blocking_reason}\n\nForce update anyway? This restarts podiumd and may interrupt active turns.`
      : `Install ${update.latest_version}? This restarts podiumd and may interrupt active turns.`;
    if (!window.confirm(warning)) return;
    updateState = "updating";
    updateError = null;
    try {
      await applyUpdate(Boolean(update.blocking_reason));
      updateState = "restarting";
      await waitForRestart(update.latest_version);
      window.location.reload();
    } catch (e) {
      updateState = "failed";
      updateError = e instanceof Error ? e.message : String(e);
    }
  }

  async function waitForRestart(version: string) {
    for (let i = 0; i < 45; i++) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      try {
        const h = await getHealth();
        if (h.version === version) {
          health = h;
          return;
        }
      } catch {
        // daemon is probably between old and new process
      }
    }
  }

  function openChat(target: ChatTarget) {
    chatTarget = target;
    route = "chat";
  }

  function openHire() {
    hireName = "";
    hireProvider = "claude";
    hirePermission = "approve";
    hireError = null;
    hireOpen = true;
  }

  async function submitHire() {
    hireError = null;
    try {
      const agent = await hireAgent({
        name: hireName.trim(),
        provider: hireProvider,
        model: "",
        effort: "high",
        permission_mode: hirePermission,
      });
      agents = [agent, ...agents.filter((a) => a.Name !== agent.Name)];
      hireOpen = false;
      route = "agents";
    } catch (e) {
      hireError = e instanceof Error ? e.message : String(e);
    }
  }

  const daemonLabel = $derived(chatStatus === "live" ? "podiumd live" : `podiumd ${chatStatus}`);
  const daemonAddr = $derived(health ? `${health.version} · ${health.commit}` : "127.0.0.1:8787");

  function seg(on: boolean): string {
    return (
      "flex:1;padding:11px;border-radius:11px;cursor:pointer;font:600 13.5px 'Hanken Grotesk';" +
      (on
        ? "border:1px solid #BFE0D6;background:#E3F1EC;color:#2F6E60"
        : "border:1px solid #EAE0D4;background:#fff;color:#6F6459")
    );
  }
</script>

<div class="app-root">
  <!-- ============ SIDEBAR ============ -->
  <aside class="sidebar">
    <div class="brand">
      <div class="brand-logo">
        <svg width="20" height="20" viewBox="0 0 48 48" aria-hidden="true">
          <path d="M8 31 Q18 6 30 16" fill="none" stroke="#fff" stroke-width="3.8" stroke-linecap="round" />
          <circle cx="30" cy="16" r="4.8" fill="#fff" />
          <circle cx="36" cy="23" r="2.9" fill="#fff" opacity=".72" />
          <circle cx="41" cy="29" r="1.7" fill="#fff" opacity=".45" />
        </svg>
      </div>
      <div>
        <div class="brand-name">Podium</div>
        <div class="brand-tag mono">conductor</div>
      </div>
    </div>

    <nav class="nav-links">
      {#each NAV as item}
        <button class="nav-link" class:active={route === item.key} onclick={() => (route = item.key)}>
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round">{@html item.icon}</svg
          >
          {item.label}
        </button>
      {/each}
    </nav>

    <div class="nav-foot">
      <div class="daemon">
        <span class="daemon-dot" class:live={chatStatus === "live"}></span>
        <div class="daemon-text mono">
          {daemonLabel}<br /><span class="daemon-addr">{daemonAddr}</span>
        </div>
      </div>
      <div class="update-box">
        <div class="update-line mono">
          {#if updateState === "checking"}
            checking updates
          {:else if updateState === "available" && update}
            update {update.latest_version}
          {:else if updateState === "updating"}
            updating
          {:else if updateState === "restarting"}
            restarting
          {:else if updateState === "failed"}
            update check failed
          {:else}
            up to date
          {/if}
        </div>
        {#if update?.blocking_reason}
          <div class="update-note">{update.blocking_reason}</div>
        {:else if updateError}
          <div class="update-note">{updateError}</div>
        {/if}
        <div class="update-actions">
          <button class="update-btn" disabled={updateState === "checking" || updateState === "updating" || updateState === "restarting"} onclick={refreshUpdate}>Check</button>
          {#if updateState === "available" || update?.blocking_reason}
            <button class="update-btn primary" disabled={updateState === "updating" || updateState === "restarting"} onclick={runUpdate}>Update</button>
          {/if}
        </div>
      </div>
      <button class="hire-btn" onclick={openHire}><span class="hire-plus">+</span> Hire agent</button>
    </div>
  </aside>

  <!-- ============ MAIN ============ -->
  <div class="main">
    {#if route === "chat"}
      <Chat {agents} target={chatTarget} onConsumeTarget={() => (chatTarget = null)} onStatus={(s) => (chatStatus = s)} />
    {:else if route === "roadmap"}
      <Roadmap {agents} onOpenChat={openChat} />
    {:else if route === "projects"}
      <Projects {agents} onOpenChat={openChat} />
    {:else if route === "agents"}
      <Agents {agents} onHire={openHire} onOpenChat={openChat} onChanged={refreshAgents} />
    {:else if route === "schedules"}
      <Schedules {agents} onOpenChat={openChat} />
    {:else if route === "skills"}
      <Skills />
    {:else if route === "logs"}
      <Logs />
    {/if}
  </div>

  <!-- ============ HIRE MODAL ============ -->
  {#if hireOpen}
    <div class="modal-backdrop" role="presentation" onclick={() => (hireOpen = false)}>
      <div class="modal-card hire-modal" role="dialog" aria-modal="true" aria-label="Hire agent" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
        <div class="modal-head">
          <div class="modal-title">Hire an agent</div>
          <div class="modal-sub">Give your new colleague a name and a backend. They'll get a workspace, a SOUL.md, and a seat on the bench.</div>
        </div>
        <div class="modal-body">
          {#if hireError}<div class="error-banner" style="margin-bottom:14px">{hireError}</div>{/if}
          <div class="label-mono" style="margin-bottom:8px">name</div>
          <input class="field-input" bind:value={hireName} placeholder="e.g. atlas" />

          <div class="label-mono" style="margin:18px 0 8px">backend</div>
          <div style="display:flex;gap:9px">
            <button style={seg(hireProvider === "claude")} onclick={() => (hireProvider = "claude")}>Claude</button>
            <button style={seg(hireProvider === "codex")} onclick={() => (hireProvider = "codex")}>Codex</button>
          </div>

          <div class="label-mono" style="margin:18px 0 8px">permission mode</div>
          <div style="display:flex;gap:9px">
            <button style={seg(hirePermission === "approve")} onclick={() => (hirePermission = "approve")}>approve · safe</button>
            <button style={seg(hirePermission === "yolo")} onclick={() => (hirePermission = "yolo")}>yolo · full access</button>
          </div>

          <button class="modal-cta" disabled={!hireName.trim()} onclick={submitHire}>Create agent</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .sidebar {
    width: 236px;
    flex: none;
    background: var(--surface);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    padding: 20px 16px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 4px 8px 18px;
  }

  .brand-logo {
    width: 34px;
    height: 34px;
    border-radius: 11px;
    background: linear-gradient(150deg, #46a08c, #2f6e60);
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 6px 14px -6px rgba(47, 110, 96, 0.6);
  }

  .brand-name {
    font: 800 18px "Hanken Grotesk";
    letter-spacing: -0.02em;
    line-height: 1;
  }

  .brand-tag {
    font-size: 10px;
    font-weight: 500;
    color: var(--faint);
    letter-spacing: 0.08em;
  }

  .nav-links {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .nav-link {
    display: flex;
    align-items: center;
    gap: 11px;
    border: none;
    cursor: pointer;
    text-align: left;
    padding: 10px 12px;
    border-radius: 12px;
    font: 600 14px "Hanken Grotesk";
    background: transparent;
    color: var(--muted);
  }

  .nav-link:hover {
    background: #f6efe6;
  }

  .nav-link.active {
    background: #e3f1ec;
    color: var(--teal-deep);
  }

  .nav-foot {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .daemon {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-radius: 12px;
    background: var(--surface-3);
    border: 1px solid var(--line-3);
  }

  .daemon-dot {
    width: 8px;
    height: 8px;
    border-radius: 99px;
    flex: none;
    background: #c0492a;
    box-shadow: 0 0 0 3px rgba(192, 73, 42, 0.18);
  }

  .daemon-dot.live {
    background: #4f9e78;
    box-shadow: 0 0 0 3px rgba(79, 158, 120, 0.18);
  }

  .daemon-text {
    font-size: 11px;
    font-weight: 500;
    color: var(--muted);
    line-height: 1.3;
  }

  .daemon-addr {
    color: var(--faint);
  }

  .update-box {
    padding: 10px 12px;
    border-radius: 12px;
    background: var(--surface-3);
    border: 1px solid var(--line-3);
  }

  .update-line {
    font-size: 10.5px;
    color: var(--muted);
  }

  .update-note {
    margin-top: 5px;
    color: var(--faint);
    font: 400 11px/1.35 "Hanken Grotesk";
  }

  .update-actions {
    display: flex;
    gap: 7px;
    margin-top: 8px;
  }

  .update-btn {
    flex: 1;
    border: 1px solid var(--line-3);
    background: #fff;
    border-radius: 9px;
    padding: 7px 8px;
    cursor: pointer;
    font: 700 11px "Hanken Grotesk";
    color: var(--muted);
  }

  .update-btn.primary {
    background: var(--teal);
    border-color: var(--teal);
    color: #fff;
  }

  .hire-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    border: 1.5px dashed #decfbe;
    background: rgba(250, 246, 240, 0.6);
    cursor: pointer;
    padding: 10px;
    border-radius: 12px;
    font: 600 13px "Hanken Grotesk";
    color: #a8825e;
  }

  .hire-plus {
    font-size: 16px;
    line-height: 1;
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }

  /* Hire modal */
  .hire-modal {
    width: 460px;
    max-width: 92vw;
  }

  .modal-head {
    padding: 26px 26px 0;
  }

  .modal-title {
    font: 800 22px "Hanken Grotesk";
    letter-spacing: -0.01em;
  }

  .modal-sub {
    font: 400 13.5px/1.5 "Hanken Grotesk";
    color: var(--muted-2);
    margin-top: 5px;
  }

  .modal-body {
    padding: 22px 26px 26px;
  }

  .modal-cta {
    width: 100%;
    margin-top: 24px;
    border: none;
    border-radius: 13px;
    padding: 13px;
    background: var(--teal);
    color: #fff;
    font: 700 15px "Hanken Grotesk";
    cursor: pointer;
    box-shadow: 0 10px 22px -8px rgba(63, 143, 126, 0.7);
  }

  @media (max-width: 768px) {
    .sidebar {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 50;
      width: auto;
      height: 72px;
      padding: 8px 10px calc(8px + env(safe-area-inset-bottom));
      border-right: none;
      border-top: 1px solid var(--line);
      box-shadow: 0 -14px 34px -28px rgba(43, 37, 32, 0.42);
    }

    .brand,
    .nav-foot {
      display: none;
    }

    .nav-links {
      flex: 1;
      flex-direction: row;
      gap: 4px;
      min-width: 0;
    }

    .nav-link {
      flex: 1;
      min-width: 0;
      flex-direction: column;
      justify-content: center;
      gap: 4px;
      padding: 7px 4px;
      border-radius: 11px;
      text-align: center;
      font-size: 11px;
      line-height: 1.1;
    }

    .nav-link svg {
      width: 18px;
      height: 18px;
    }

    .main {
      width: 100%;
      min-height: 0;
      padding-bottom: calc(72px + env(safe-area-inset-bottom));
    }
  }
</style>
