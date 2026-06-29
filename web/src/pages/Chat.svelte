<script lang="ts">
  import { onMount } from "svelte";
  import { getSession, listProjects } from "../lib/api";
  import {
    agentGradient,
    avatarStyle,
    initial,
    modeChip,
    originLabel,
    originStyle,
    providerChip,
  } from "../lib/theme";
  import type {
    Agent,
    ClientMessage,
    Message,
    PermissionRequest,
    Project,
    ServerMessage,
    Session,
    SessionOrigin,
  } from "../lib/types";

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  let {
    agents = [],
    target = null,
    onConsumeTarget = () => {},
    onStatus = (_s: "connecting" | "live" | "offline") => {},
  }: {
    agents?: Agent[];
    target?: ChatTarget | null;
    onConsumeTarget?: () => void;
    onStatus?: (s: "connecting" | "live" | "offline") => void;
  } = $props();

  let status = $state<"connecting" | "live" | "offline">("connecting");
  let sessions = $state<Session[]>([]);
  let activeSession = $state<Session | null>(null);
  let projectName = $state<string>("");
  let messages = $state<Message[]>([]);
  let pendingAssistant = $state("");
  let pendingPermission = $state<PermissionRequest | null>(null);
  let permissionRemaining = $state(0);
  let messageText = $state("");
  let selectedAgent = $state("");
  let originFilter = $state<SessionOrigin | "all">("all");
  let agentFilter = $state("all");
  let projectFilter = $state("all");
  let projects = $state<Project[]>([]);
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let sending = $state(false);
  let ws: WebSocket | null = null;
  let poll: number | undefined;
  let countdown: number | undefined;
  let pendingSeed: string | null = null;

  // Layout / UI state.
  let sessOpen = $state(true);
  let ctxOpen = $state(true);
  let openDropdown = $state<string | null>(null);
  let newSessionOpen = $state(false);

  const SLASH_CMDS = [
    { cmd: "/model", desc: "set the model for this session" },
    { cmd: "/effort", desc: "low · medium · high · xhigh · max" },
    { cmd: "/profile", desc: "switch auth context — replays history" },
    { cmd: "/permission", desc: "approve or yolo for this session" },
    { cmd: "/name", desc: "rename the session" },
    { cmd: "/help", desc: "list every command" },
  ];

  const EFFORTS = ["low", "medium", "high", "xhigh", "max"];

  const activeAgent = $derived(
    agents.find((a) => a.Name === activeSession?.AgentName || a.Name === selectedAgent),
  );
  const activeGrad = $derived(agentGradient(activeAgent?.Name ?? selectedAgent ?? "?"));
  const activeMono = $derived(initial(activeAgent?.Name ?? selectedAgent ?? "?"));
  const filteredSessions = $derived(
    sessions.filter((s) => {
      if (originFilter !== "all" && s.Origin !== originFilter) return false;
      if (agentFilter !== "all" && s.AgentName !== agentFilter) return false;
      if (projectFilter !== "all" && s.ProjectID !== projectFilter) return false;
      return true;
    }),
  );
  function projectLabel(id: string): string {
    return projects.find((p) => p.id === id)?.name ?? id;
  }
  const sessionTitle = $derived(
    activeSession ? activeSession.Name || activeSession.AgentName : selectedAgent || "New session",
  );
  const modelOptions = $derived(
    activeAgent?.Provider === "codex" ? ["gpt-5.1", "gpt-5.1-mini", "o4"] : ["sonnet", "opus", "haiku"],
  );
  const curModel = $derived(activeSession?.Model || activeAgent?.Model || "—");
  const curEffort = $derived(activeSession?.Effort || activeAgent?.Effort || "medium");
  const curMode = $derived(activeSession?.PermissionMode || activeAgent?.PermissionMode || "approve");
  const showSlash = $derived(messageText.startsWith("/"));

  function sessionSub(s: Session): string {
    return `${s.AgentName} · ${s.Provider}${s.Model ? " " + s.Model : ""}`;
  }

  onMount(() => {
    connect();
    listProjects().then((p) => (projects = p)).catch(() => {});
    poll = window.setInterval(() => send({ type: "list" }), 4000);
    countdown = window.setInterval(updatePermissionRemaining, 1000);
    return () => {
      if (poll) window.clearInterval(poll);
      if (countdown) window.clearInterval(countdown);
      ws?.close();
    };
  });

  $effect(() => {
    const t = target;
    if (!t) return;
    onConsumeTarget();
    void openTarget(t);
  });

  async function openTarget(t: ChatTarget) {
    if (t.sessionId) {
      const session = sessions.find((s) => s.ID === t.sessionId) ?? ({ ID: t.sessionId } as Session);
      await loadHistory(session);
    } else if (t.agentName) {
      selectedAgent = t.agentName;
      newSession();
    }
    if (t.seed) {
      pendingSeed = t.seed;
      flushSeed();
    }
  }

  function flushSeed() {
    if (pendingSeed && status === "live") {
      const seed = pendingSeed;
      pendingSeed = null;
      sendTurn(seed);
    }
  }

  function connect() {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    ws = new WebSocket(`${protocol}://${location.host}/api/ws`);
    setStatus("connecting");
    ws.onopen = () => {
      setStatus("live");
      send({ type: "list" });
      flushSeed();
    };
    ws.onclose = () => setStatus("offline");
    ws.onerror = () => setStatus("offline");
    ws.onmessage = (event) => handleServerMessage(JSON.parse(event.data) as ServerMessage);
  }

  function setStatus(s: "connecting" | "live" | "offline") {
    status = s;
    onStatus(s);
  }

  function send(msg: ClientMessage) {
    if (ws?.readyState !== WebSocket.OPEN) {
      error = "WebSocket is offline";
      return;
    }
    ws.send(JSON.stringify(msg));
  }

  function handleServerMessage(msg: ServerMessage) {
    switch (msg.type) {
      case "state":
        sessions = msg.sessions ?? [];
        if (!selectedAgent && agents.length > 0) selectedAgent = agents[0].Name;
        if (activeSession) {
          const replacement = sessions.find((s) => s.ID === activeSession?.ID);
          if (replacement) activeSession = replacement;
        }
        break;
      case "session":
        if (msg.session) {
          activeSession = msg.session;
          selectedAgent = msg.session.AgentName;
          sessions = [msg.session, ...sessions.filter((s) => s.ID !== msg.session?.ID)];
        }
        break;
      case "history":
        messages = msg.history ?? [];
        pendingAssistant = "";
        break;
      case "message":
        if (msg.message && !messages.some((e) => sameMessage(e, msg.message))) {
          messages = [...messages, msg.message];
          if (msg.message.Role === "assistant") pendingAssistant = "";
        }
        break;
      case "delta":
        pendingAssistant += msg.delta ?? "";
        break;
      case "assistant":
        if (!pendingAssistant) pendingAssistant = msg.delta ?? "";
        break;
      case "permission_request":
        pendingPermission = msg.request ?? null;
        updatePermissionRemaining();
        break;
      case "notice":
        notice = msg.notice ?? null;
        sending = false;
        break;
      case "done":
        pendingPermission = null;
        sending = false;
        window.setTimeout(() => send({ type: "list" }), 1200);
        break;
      case "error":
        error = msg.error ?? "Unknown server error";
        sending = false;
        break;
    }
  }

  function sameMessage(a: Message, b: Message | undefined) {
    return !!b && a.ID === b.ID && a.SessionID === b.SessionID;
  }

  async function loadHistory(session: Session) {
    error = null;
    try {
      const detail = await getSession(session.ID);
      activeSession = detail.session;
      selectedAgent = detail.session.AgentName;
      messages = detail.history ?? [];
      projectName = detail.project_name ?? "";
      pendingAssistant = "";
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function sendTurn(text = messageText.trim()) {
    if (!text) return;
    if (!activeSession && !selectedAgent) {
      error = "Create or select an agent first";
      return;
    }
    error = null;
    notice = null;
    sending = true;
    pendingAssistant = "";
    send({
      type: "send_turn",
      request_id: crypto.randomUUID(),
      agent_name: activeSession ? undefined : selectedAgent,
      session_id: activeSession?.ID,
      message: text,
    });
    messageText = "";
  }

  function newSession() {
    activeSession = null;
    messages = [];
    projectName = "";
    pendingAssistant = "";
    notice = null;
    error = null;
  }

  function startSessionWith(agentName: string) {
    selectedAgent = agentName;
    newSession();
    newSessionOpen = false;
  }

  function runCommand(command: string) {
    openDropdown = null;
    if (!activeSession) {
      messageText = command;
      return;
    }
    sendTurn(command);
  }

  function decidePermission(allow: boolean) {
    if (!pendingPermission) return;
    send({
      type: "permission_decision",
      request_id: pendingPermission.id,
      decision: allow
        ? { behavior: "allow", updatedInput: pendingPermission.input }
        : { behavior: "deny", message: "Denied from web" },
    });
    pendingPermission = null;
  }

  function updatePermissionRemaining() {
    if (!pendingPermission?.expires_at) {
      permissionRemaining = 0;
      return;
    }
    permissionRemaining = Math.max(
      0,
      Math.ceil((new Date(pendingPermission.expires_at).getTime() - Date.now()) / 1000),
    );
  }

  function toggleDropdown(key: string) {
    openDropdown = openDropdown === key ? null : key;
  }

  function permissionCmd(input: Record<string, unknown>): string {
    return Object.entries(input)
      .map(([k, v]) => (k === "command" ? String(v) : `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`))
      .join("\n");
  }
</script>

<div class="chat" style="flex:1;display:flex;min-height:0">
  <!-- ===== sessions column ===== -->
  {#if sessOpen}
    <div class="sess-col">
      <div class="sess-head">
        <div class="sess-title">Sessions</div>
        <button class="sq-btn teal" onclick={() => (newSessionOpen = true)} title="New session">+</button>
        <button class="sq-btn" onclick={() => (sessOpen = false)} title="Collapse">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6l-6 6 6 6" /></svg>
        </button>
      </div>

      <div class="sess-filters">
        <div class="dd-wrap">
          <button class="filter-chip" onclick={() => toggleDropdown("fProject")}>
            {projectFilter === "all" ? "all projects" : projectLabel(projectFilter)} <span style="opacity:.55">▾</span>
          </button>
          {#if openDropdown === "fProject"}
            <div class="dd-menu">
              <button class="dd-opt" class:sel={projectFilter === "all"} onclick={() => { projectFilter = "all"; openDropdown = null; }}>all projects</button>
              {#each projects as p}
                <button class="dd-opt" class:sel={projectFilter === p.id} onclick={() => { projectFilter = p.id; openDropdown = null; }}>{p.name}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="filter-chip" onclick={() => toggleDropdown("fAgent")}>
            {agentFilter === "all" ? "all agents" : agentFilter} <span style="opacity:.55">▾</span>
          </button>
          {#if openDropdown === "fAgent"}
            <div class="dd-menu">
              <button class="dd-opt" class:sel={agentFilter === "all"} onclick={() => { agentFilter = "all"; openDropdown = null; }}>all agents</button>
              {#each agents as a}
                <button class="dd-opt" class:sel={agentFilter === a.Name} onclick={() => { agentFilter = a.Name; openDropdown = null; }}>{a.Name}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="filter-chip" onclick={() => toggleDropdown("fOrigin")}>
            {originFilter === "all" ? "all origins" : originFilter} <span style="opacity:.55">▾</span>
          </button>
          {#if openDropdown === "fOrigin"}
            <div class="dd-menu">
              {#each ["all", "web", "cli", "onboarding", "schedule", "roadmap"] as o}
                <button class="dd-opt" class:sel={originFilter === o} onclick={() => { originFilter = o as SessionOrigin | "all"; openDropdown = null; }}>{o === "all" ? "all origins" : o}</button>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <div class="sess-list">
        {#each filteredSessions as s (s.ID)}
          <button class="sess-row" class:sel={activeSession?.ID === s.ID} onclick={() => loadHistory(s)}>
            <span style={avatarStyle(agentGradient(s.AgentName), 32, 10, 13)}>{initial(s.AgentName)}</span>
            <span class="sess-row-text">
              <span class="sess-row-title">{s.Name || s.AgentName}</span>
              <span class="sess-row-sub mono">{sessionSub(s)}</span>
            </span>
            <span style={originStyle(s.Origin)}>{originLabel(s.Origin)}</span>
          </button>
        {/each}
        {#if filteredSessions.length === 0}
          <p class="empty-note">No sessions yet. Pick an agent and say hello.</p>
        {/if}
      </div>
    </div>
  {/if}

  <!-- ===== conversation ===== -->
  <div class="conv">
    <div class="conv-head">
      {#if !sessOpen}
        <button class="sq-btn" onclick={() => (sessOpen = true)} title="Show sessions">
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M9 4v16" /></svg>
        </button>
      {/if}
      <div class="conv-title">{sessionTitle}</div>
      {#if activeSession}<span style={originStyle(activeSession.Origin)}>{originLabel(activeSession.Origin)}</span>{/if}
      {#if !ctxOpen}
        <button class="sq-btn" onclick={() => (ctxOpen = true)} title="Show details">
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M15 4v16" /></svg>
        </button>
      {/if}
    </div>

    {#if projectName}
      <div class="proj-strip">
        <span class="proj-dot-sm" style="background:#3F8F7E"></span>
        <span class="mono proj-strip-text">part of <b>{projectName}</b></span>
      </div>
    {/if}

    <div class="msgs">
      {#each messages as m (m.ID)}
        {#if m.Role === "user"}
          <div class="row-end">
            <div class="bubble-user">{m.Content}</div>
          </div>
        {:else}
          <div class="row-start">
            <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
            <div class="bubble-assistant">{m.Content}</div>
          </div>
        {/if}
      {/each}

      {#if pendingAssistant}
        <div class="row-start">
          <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
          <div class="bubble-assistant">{pendingAssistant}<span class="cursor"></span></div>
        </div>
      {/if}

      {#if sending && !pendingAssistant && !pendingPermission}
        <div class="row-start" style="align-items:center">
          <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
          <span class="thinking">
            <span class="tdot"></span><span class="tdot d2"></span><span class="tdot d3"></span>
          </span>
        </div>
      {/if}

      {#if pendingPermission}
        <div class="row-start approve-wrap">
          <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
          <div class="approve-card">
            <div class="approve-head">
              <span class="approve-tag mono">approve · {pendingPermission.tool_name}</span>
              <span style="flex:1"></span>
              <span class="approve-timer mono">auto-denies in {permissionRemaining}s</span>
            </div>
            <div class="approve-body">
              <div class="approve-cmd mono">{permissionCmd(pendingPermission.input)}</div>
              <div class="approve-actions">
                <button class="approve-yes" onclick={() => decidePermission(true)}>Approve</button>
                <button class="approve-no" onclick={() => decidePermission(false)}>Deny</button>
              </div>
            </div>
          </div>
        </div>
      {/if}

      {#if notice}<div class="notice">{notice}</div>{/if}
      {#if error}<div class="row-start"><div class="bubble-error"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex:none"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.3 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.7 3.86a2 2 0 0 0-3.4 0z" /></svg>{error}</div></div>{/if}
    </div>

    <!-- composer -->
    <div class="composer">
      {#if showSlash}
        <div class="slash-menu">
          {#each SLASH_CMDS as c}
            <button class="slash-item" onclick={() => (messageText = c.cmd + " ")}>
              <span class="slash-cmd mono">{c.cmd}</span>
              <span class="slash-desc">{c.desc}</span>
            </button>
          {/each}
        </div>
      {/if}
      <div class="composer-box">
        <input
          class="composer-input"
          bind:value={messageText}
          placeholder={`Message ${activeAgent?.Name ?? "agent"}…   / for commands`}
          onkeydown={(e) => { if (e.key === "Enter") { e.preventDefault(); sendTurn(); } }}
        />
        <button class="composer-send" disabled={sending || status !== "live"} onclick={() => sendTurn()}>↑</button>
      </div>
      <div class="composer-meta">
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("model")}>/model {curModel} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "model"}
            <div class="dd-menu up">
              {#each modelOptions as o}
                <button class="dd-opt" class:sel={o === curModel} onclick={() => runCommand(`/model ${o}`)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("effort")}>/effort {curEffort} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "effort"}
            <div class="dd-menu up">
              {#each EFFORTS as o}
                <button class="dd-opt" class:sel={o === curEffort} onclick={() => runCommand(`/effort ${o}`)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("perm")}>/permission {curMode} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "perm"}
            <div class="dd-menu up">
              {#each ["approve", "yolo"] as o}
                <button class="dd-opt" class:sel={o === curMode} onclick={() => runCommand(`/permission ${o}`)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <span style={modeChip(curMode)}>{curMode}</span>
      </div>
    </div>
  </div>

  <!-- ===== context panel ===== -->
  {#if ctxOpen}
    <div class="ctx">
      <div class="ctx-collapse">
        <button class="sq-btn" onclick={() => (ctxOpen = false)} title="Collapse">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6" /></svg>
        </button>
      </div>
      <div style={avatarStyle(activeGrad, 64, 20, 26) + ";animation:floaty 5s ease-in-out infinite"}>{activeMono}</div>
      <div class="ctx-name">{activeAgent?.Name ?? selectedAgent ?? "—"}</div>
      <div class="ctx-chips">
        {#if activeAgent}<span style={providerChip(activeAgent.Provider)}>{activeAgent.Provider}</span>{/if}
        <span style={modeChip(curMode)}>{curMode}</span>
      </div>
      <div class="ctx-soul">
        {activeSession?.Description || `Runs on ${activeAgent?.Provider ?? "—"} · ${curModel} · effort ${curEffort}.`}
      </div>

      {#if projectName}
        <div class="label-mono" style="margin:24px 0 10px">linked project</div>
        <div class="ctx-proj">
          <div class="ctx-proj-row">
            <span class="proj-dot-sm" style="background:#3F8F7E"></span>
            <span class="ctx-proj-name">{projectName}</span>
          </div>
        </div>
      {/if}

      <div class="label-mono" style="margin:24px 0 10px">engine</div>
      <div class="ctx-specs">
        <div class="spec-row"><span>Model</span><span class="mono">{curModel}</span></div>
        <div class="spec-row"><span>Effort</span><span class="mono">{curEffort}</span></div>
        {#if activeAgent?.Profile}<div class="spec-row"><span>Profile</span><span class="mono">{activeAgent.Profile}</span></div>{/if}
        <div class="spec-row"><span>Permission</span><span class="mono">{curMode}</span></div>
      </div>
    </div>
  {/if}
</div>

<!-- ===== New session modal ===== -->
{#if newSessionOpen}
  <div class="modal-backdrop" role="presentation" onclick={() => (newSessionOpen = false)}>
    <div class="modal-card ns-modal" role="dialog" aria-modal="true" aria-label="New session" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div style="padding:24px 26px 4px">
        <div class="modal-title">New session</div>
        <div class="modal-sub">Who do you want to work with? Pick a colleague and we'll open a fresh chat.</div>
      </div>
      <div class="ns-list">
        {#each agents as a}
          <button class="ns-row" onclick={() => startSessionWith(a.Name)}>
            <span style={avatarStyle(agentGradient(a.Name), 46, 14, 19)}>{initial(a.Name)}</span>
            <span class="ns-row-text">
              <span class="ns-row-head">
                <b>{a.Name}</b>
                <span style={providerChip(a.Provider)}>{a.Provider}</span>
                <span style={modeChip(a.PermissionMode)}>{a.PermissionMode}</span>
              </span>
              <span class="ns-row-sub mono">{a.Model || a.Provider} · effort {a.Effort || "medium"}</span>
            </span>
            <span class="ns-arrow">→</span>
          </button>
        {/each}
        {#if agents.length === 0}<p class="empty-note">No agents yet — hire one first.</p>{/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .sess-col {
    width: 286px;
    flex: none;
    background: var(--surface-2);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
  }

  .sess-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 18px 18px 12px;
  }

  .sess-title {
    font: 800 17px "Hanken Grotesk";
    flex: 1;
  }

  .sq-btn {
    width: 30px;
    height: 30px;
    flex: none;
    border: 1px solid var(--field-line);
    background: #fff;
    border-radius: 9px;
    cursor: pointer;
    color: #9a8e80;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 17px;
  }

  .sq-btn.teal {
    color: var(--teal-deep);
  }

  .sess-filters {
    display: flex;
    gap: 6px;
    padding: 0 18px 12px;
    flex-wrap: wrap;
    position: relative;
    z-index: 5;
  }

  .dd-wrap {
    position: relative;
  }

  .filter-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 11px;
    border-radius: 999px;
    background: #f1eadf;
    border: 1px solid #e6dbcb;
    font: 500 11px "JetBrains Mono", monospace;
    color: #6f5b45;
    cursor: pointer;
  }

  .dd-menu {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    min-width: 150px;
    background: #fff;
    border: 1px solid var(--field-line);
    border-radius: 12px;
    box-shadow: 0 16px 40px -16px rgba(43, 37, 32, 0.34);
    padding: 6px;
    z-index: 25;
    display: flex;
    flex-direction: column;
  }

  .dd-menu.up {
    top: auto;
    bottom: calc(100% + 7px);
  }

  .dd-opt {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 11px;
    border-radius: 8px;
    font: 500 12.5px "JetBrains Mono", monospace;
    cursor: pointer;
    color: #5a5048;
    background: transparent;
    border: none;
    text-align: left;
  }

  .dd-opt:hover {
    background: #f6efe6;
  }

  .dd-opt.sel {
    color: var(--teal-deep);
    background: #e3f1ec;
  }

  .sess-list {
    flex: 1;
    overflow-y: auto;
    padding: 0 12px 16px;
  }

  .sess-row {
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    padding: 11px 12px;
    border-radius: 13px;
    cursor: pointer;
    margin-bottom: 2px;
    background: transparent;
    border: 1px solid transparent;
    text-align: left;
  }

  .sess-row:hover {
    background: #f6efe6;
  }

  .sess-row.sel {
    background: #fff;
    box-shadow: 0 2px 10px -6px rgba(43, 37, 32, 0.2);
    border: 1px solid var(--line-3);
  }

  .sess-row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .sess-row-title {
    font: 600 13.5px "Hanken Grotesk";
    color: var(--ink);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sess-row-sub {
    font-size: 11px;
    color: #9a8e80;
    margin-top: 2px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .conv {
    flex: 1;
    min-width: 360px;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }

  .conv-head {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 24px;
    border-bottom: 1px solid var(--line);
    background: var(--surface-2);
  }

  .conv-title {
    font: 700 16px "Hanken Grotesk";
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .proj-strip {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 24px;
    background: rgba(63, 143, 126, 0.06);
    border-bottom: 1px solid #e6efe9;
  }

  .proj-dot-sm {
    width: 8px;
    height: 8px;
    border-radius: 99px;
    flex: none;
  }

  .proj-strip-text {
    font-size: 11.5px;
    font-weight: 500;
    color: #5e8a7b;
  }

  .proj-strip-text b {
    color: var(--teal-ink);
  }

  .msgs {
    flex: 1;
    overflow-y: auto;
    padding: 26px 24px;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .row-end {
    display: flex;
    justify-content: flex-end;
  }

  .row-start {
    display: flex;
    gap: 12px;
    max-width: 88%;
  }

  .bubble-user {
    max-width: 74%;
    background: #fff;
    border: 1px solid var(--line-3);
    border-radius: 18px 18px 6px 18px;
    padding: 12px 16px;
    font: 400 15px/1.5 "Hanken Grotesk";
    color: var(--ink);
    box-shadow: 0 2px 8px -5px rgba(43, 37, 32, 0.14);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .bubble-assistant {
    background: #fff;
    border: 1px solid var(--line-3);
    border-radius: 6px 18px 18px 18px;
    padding: 13px 16px;
    font: 400 15px/1.6 "Hanken Grotesk";
    color: var(--ink-soft);
    box-shadow: 0 2px 8px -6px rgba(43, 37, 32, 0.12);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .bubble-error {
    display: inline-flex;
    align-items: flex-start;
    gap: 7px;
    background: #fbeeea;
    border: 1px solid #e7c3b5;
    border-radius: 6px 18px 18px 18px;
    padding: 13px 16px;
    font: 500 14px/1.6 "Hanken Grotesk";
    color: #a23e22;
    box-shadow: 0 2px 8px -6px rgba(162, 62, 34, 0.18);
  }

  .cursor {
    display: inline-block;
    width: 8px;
    height: 16px;
    background: var(--orange);
    border-radius: 2px;
    vertical-align: -3px;
    margin-left: 2px;
    animation: blink 1s steps(1) infinite;
  }

  .thinking {
    display: inline-flex;
    gap: 4px;
    align-items: center;
    padding: 9px 13px;
    border-radius: 16px;
    background: #fff;
    border: 1px solid var(--line-3);
  }

  .tdot {
    width: 6px;
    height: 6px;
    border-radius: 9px;
    background: var(--orange);
    animation: dotPulse 1.2s infinite;
  }

  .tdot.d2 {
    animation-delay: 0.2s;
  }

  .tdot.d3 {
    animation-delay: 0.4s;
  }

  .approve-wrap {
    margin-left: 0;
    max-width: 600px;
  }

  .approve-card {
    flex: 1;
    background: #fff;
    border: 1px solid #f0dca9;
    border-radius: 15px;
    overflow: hidden;
    box-shadow: 0 8px 22px -14px rgba(154, 110, 30, 0.32);
  }

  .approve-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 11px 15px;
    background: #fbf1dd;
    border-bottom: 1px solid #f0dca9;
  }

  .approve-tag {
    font-weight: 700;
    font-size: 11px;
    letter-spacing: 0.06em;
    color: var(--gold);
    text-transform: uppercase;
  }

  .approve-timer {
    font-size: 11px;
    font-weight: 500;
    color: #c99;
  }

  .approve-body {
    padding: 14px 15px;
  }

  .approve-cmd {
    font: 500 12.5px/1.5 "JetBrains Mono", monospace;
    color: var(--ink);
    background: var(--surface-3);
    border: 1px solid var(--line-3);
    border-radius: 9px;
    padding: 10px 12px;
    word-break: break-word;
    white-space: pre-wrap;
  }

  .approve-actions {
    display: flex;
    gap: 9px;
    margin-top: 12px;
  }

  .approve-yes {
    flex: 1;
    border: none;
    border-radius: 10px;
    padding: 9px;
    background: var(--teal);
    color: #fff;
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
    box-shadow: 0 6px 13px -6px rgba(63, 143, 126, 0.7);
  }

  .approve-no {
    flex: 1;
    border: 1px solid #e6d9cc;
    border-radius: 10px;
    padding: 9px;
    background: #fff;
    color: var(--muted);
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
  }

  .notice {
    border: 1px solid #cfe3d8;
    background: #eef6f0;
    color: #285d3f;
    border-radius: 12px;
    padding: 11px 14px;
    font: 500 13px "Hanken Grotesk";
  }

  .composer {
    padding: 14px 24px 18px;
    border-top: 1px solid var(--line);
    background: var(--surface-2);
    position: relative;
  }

  .slash-menu {
    position: absolute;
    bottom: 94px;
    left: 24px;
    right: 24px;
    max-width: 420px;
    background: #fff;
    border: 1px solid var(--field-line);
    border-radius: 14px;
    box-shadow: 0 16px 40px -16px rgba(43, 37, 32, 0.3);
    padding: 7px;
    animation: popIn 0.16s ease;
    display: flex;
    flex-direction: column;
  }

  .slash-item {
    display: flex;
    gap: 12px;
    align-items: baseline;
    padding: 8px 11px;
    border-radius: 9px;
    border: none;
    background: transparent;
    cursor: pointer;
    text-align: left;
  }

  .slash-item:hover {
    background: #f6efe6;
  }

  .slash-cmd {
    font: 600 13px "JetBrains Mono", monospace;
    color: var(--orange-ink);
    min-width: 104px;
  }

  .slash-desc {
    font: 400 12.5px "Hanken Grotesk";
    color: var(--muted-2);
  }

  .composer-box {
    display: flex;
    align-items: center;
    gap: 10px;
    background: #fff;
    border: 1px solid var(--field-line);
    border-radius: 14px;
    padding: 8px 8px 8px 16px;
  }

  .composer-input {
    flex: 1;
    border: none;
    outline: none;
    background: transparent;
    font: 400 15px "Hanken Grotesk";
    color: var(--ink);
    padding: 6px 0;
  }

  .composer-send {
    width: 36px;
    height: 36px;
    border: none;
    border-radius: 11px;
    background: var(--teal);
    color: #fff;
    font-size: 16px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 6px 14px -6px rgba(63, 143, 126, 0.7);
  }

  .composer-meta {
    display: flex;
    gap: 7px;
    margin-top: 11px;
    flex-wrap: wrap;
    align-items: center;
  }

  .chip-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 11px;
    border: 1px solid #e6dbcb;
    border-radius: 999px;
    background: #f1eadf;
    font: 500 12px "JetBrains Mono", monospace;
    color: #6f5b45;
    cursor: pointer;
  }

  .ctx {
    width: 288px;
    flex: none;
    background: var(--surface-2);
    border-left: 1px solid var(--line);
    padding: 18px 22px 24px;
    overflow-y: auto;
  }

  .ctx-collapse {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 4px;
  }

  .ctx-name {
    font: 800 22px "Hanken Grotesk";
    margin-top: 15px;
    letter-spacing: -0.01em;
  }

  .ctx-chips {
    display: flex;
    gap: 6px;
    margin-top: 9px;
    flex-wrap: wrap;
  }

  .ctx-soul {
    font: 400 13.5px/1.6 "Hanken Grotesk";
    color: var(--muted);
    margin-top: 14px;
    font-style: italic;
  }

  .ctx-proj {
    background: #fff;
    border: 1px solid var(--line-3);
    border-radius: 14px;
    padding: 14px;
  }

  .ctx-proj-row {
    display: flex;
    align-items: center;
    gap: 9px;
  }

  .ctx-proj-name {
    font: 600 14px "Hanken Grotesk";
    color: var(--ink);
  }

  .ctx-specs {
    background: #fff;
    border: 1px solid var(--line-3);
    border-radius: 14px;
    padding: 4px 14px;
  }

  .spec-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 9px 0;
    border-top: 1px solid #f1eae0;
    font: 400 13.5px "Hanken Grotesk";
    color: var(--muted);
  }

  .spec-row:first-child {
    border-top: none;
  }

  .spec-row span:last-child {
    font: 600 12.5px "JetBrains Mono", monospace;
    color: var(--ink);
  }

  /* new session modal */
  .ns-modal {
    width: 440px;
    max-width: 92vw;
  }

  .ns-list {
    padding: 16px 18px 22px;
    display: flex;
    flex-direction: column;
    gap: 9px;
  }

  .ns-row {
    display: flex;
    gap: 14px;
    align-items: center;
    padding: 13px;
    border: 1px solid var(--line-3);
    border-radius: 15px;
    cursor: pointer;
    background: #fff;
    text-align: left;
  }

  .ns-row:hover {
    border-color: #bfe0d6;
    background: #fbfdfc;
  }

  .ns-row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .ns-row-head {
    display: flex;
    align-items: center;
    gap: 7px;
    font: 800 16px "Hanken Grotesk";
  }

  .ns-row-sub {
    font-size: 12px;
    color: var(--muted-2);
    margin-top: 4px;
  }

  .ns-arrow {
    font: 600 16px "Hanken Grotesk";
    color: var(--teal-deep);
    flex: none;
  }
</style>
