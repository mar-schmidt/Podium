<script lang="ts">
  import { onMount } from "svelte";
  import type {
    Agent,
    ClientMessage,
    Health,
    Message,
    PermissionMode,
    PermissionRequest,
    Provider,
    ServerMessage,
    Session,
    SessionOrigin,
  } from "./lib/types";

  let health = $state<Health | null>(null);
  let status = $state<"connecting" | "live" | "offline">("connecting");
  let agents = $state<Agent[]>([]);
  let sessions = $state<Session[]>([]);
  let activeSession = $state<Session | null>(null);
  let messages = $state<Message[]>([]);
  let pendingAssistant = $state("");
  let pendingPermission = $state<PermissionRequest | null>(null);
  let permissionRemaining = $state(0);
  let messageText = $state("");
  let selectedAgent = $state("");
  let originFilter = $state<SessionOrigin | "all">("all");
  let agentFilter = $state("all");
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let sending = $state(false);
  let hireOpen = $state(false);
  let hireName = $state("jared");
  let hireProvider = $state<Provider>("claude");
  let hirePermission = $state<PermissionMode>("approve");
  let hireModel = $state("");
  let hireEffort = $state("medium");
  let modelDraft = $state("");
  let ws: WebSocket | null = null;
  let poll: number | undefined;
  let countdown: number | undefined;

  const activeAgent = $derived(agents.find((agent) => agent.Name === activeSession?.AgentName || agent.Name === selectedAgent));
  const filteredSessions = $derived(
    sessions.filter((session) => {
      if (originFilter !== "all" && session.Origin !== originFilter) return false;
      if (agentFilter !== "all" && session.AgentName !== agentFilter) return false;
      return true;
    }),
  );
  const sessionTitle = $derived(activeSession ? activeSession.Name || activeSession.AgentName : selectedAgent || "New session");
  const sessionDescription = $derived(activeSession?.Description || "Ready");

  onMount(() => {
    void fetchHealth();
    connect();
    poll = window.setInterval(() => send({ type: "list" }), 4000);
    countdown = window.setInterval(updatePermissionRemaining, 1000);
    return () => {
      if (poll) window.clearInterval(poll);
      if (countdown) window.clearInterval(countdown);
      ws?.close();
    };
  });

  async function fetchHealth() {
    try {
      const res = await fetch("/healthz");
      if (!res.ok) throw new Error(`status ${res.status}`);
      health = (await res.json()) as Health;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function connect() {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    ws = new WebSocket(`${protocol}://${location.host}/api/ws`);
    status = "connecting";
    ws.onopen = () => {
      status = "live";
      send({ type: "list" });
    };
    ws.onclose = () => {
      status = "offline";
    };
    ws.onerror = () => {
      status = "offline";
    };
    ws.onmessage = (event) => {
      handleServerMessage(JSON.parse(event.data) as ServerMessage);
    };
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
        agents = msg.agents ?? [];
        sessions = msg.sessions ?? [];
        if (!selectedAgent && agents.length > 0) selectedAgent = agents[0].Name;
        if (activeSession) {
          const replacement = sessions.find((session) => session.ID === activeSession?.ID);
          if (replacement) activeSession = replacement;
        } else if (sessions.length > 0) {
          activeSession = sessions[0];
          selectedAgent = sessions[0].AgentName;
          void loadHistory(sessions[0]);
        }
        break;
      case "session":
        if (msg.session) {
          activeSession = msg.session;
          selectedAgent = msg.session.AgentName;
          sessions = [msg.session, ...sessions.filter((session) => session.ID !== msg.session?.ID)];
          modelDraft = msg.session.Model;
        }
        break;
      case "history":
        messages = msg.history ?? [];
        pendingAssistant = "";
        break;
      case "message":
        if (msg.message && !messages.some((existing) => sameMessage(existing, msg.message))) {
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

  async function hireAgent() {
    error = null;
    const res = await fetch("/api/agents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name: hireName.trim(),
        provider: hireProvider,
        model: hireModel.trim(),
        effort: hireEffort,
        permission_mode: hirePermission,
      }),
    });
    if (!res.ok) {
      error = await res.text();
      return;
    }
    const agent = (await res.json()) as Agent;
    agents = [agent, ...agents.filter((item) => item.Name !== agent.Name)];
    selectedAgent = agent.Name;
    hireOpen = false;
  }

  async function loadHistory(session: Session) {
    error = null;
    const res = await fetch(`/api/sessions/${session.ID}`);
    if (!res.ok) {
      error = await res.text();
      return;
    }
    const detail = (await res.json()) as { session: Session; history: Message[] };
    activeSession = detail.session;
    selectedAgent = detail.session.AgentName;
    messages = detail.history;
    pendingAssistant = "";
    modelDraft = detail.session.Model;
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
    pendingAssistant = "";
    notice = null;
    error = null;
  }

  function runCommand(command: string) {
    if (!activeSession) {
      messageText = command;
      return;
    }
    sendTurn(command);
  }

  function setModel() {
    const value = modelDraft.trim();
    if (value) runCommand(`/model ${value}`);
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
    permissionRemaining = Math.max(0, Math.ceil((new Date(pendingPermission.expires_at).getTime() - Date.now()) / 1000));
  }

  function badgeClass(origin: SessionOrigin) {
    return `badge ${origin}`;
  }

  function stopModalEvent(event: Event) {
    event.stopPropagation();
  }
</script>

<main class="app-shell">
  <aside class="sidebar">
    <header class="brand-row">
      <div>
        <h1>Podium</h1>
        <p>{health ? `${health.version} (${health.commit})` : "local daemon"}</p>
      </div>
      <span class:live={status === "live"} class="status-dot" title={status}></span>
    </header>

    <div class="toolbar-row">
      <button class="button primary" onclick={() => (hireOpen = true)}>Hire agent</button>
      <button class="icon-button" onclick={newSession} title="New session">+</button>
    </div>

    <section class="panel">
      <div class="panel-heading">
        <span>Agents</span>
      </div>
      <div class="agent-list">
        {#each agents as agent}
          <button class:selected={selectedAgent === agent.Name} class="agent-row" onclick={() => ((selectedAgent = agent.Name), newSession())}>
            <strong>{agent.Name}</strong>
            <small>{agent.Provider} / {agent.PermissionMode}</small>
          </button>
        {/each}
      </div>
    </section>

    <section class="panel sessions-panel">
      <div class="panel-heading">
        <span>Sessions</span>
      </div>
      <div class="filters">
        <select class="field compact" bind:value={originFilter} aria-label="Origin filter">
          <option value="all">all origins</option>
          <option value="web">web</option>
          <option value="cli">cli</option>
          <option value="schedule">schedule</option>
          <option value="roadmap">roadmap</option>
        </select>
        <select class="field compact" bind:value={agentFilter} aria-label="Agent filter">
          <option value="all">all agents</option>
          {#each agents as agent}
            <option value={agent.Name}>{agent.Name}</option>
          {/each}
        </select>
      </div>
      <div class="session-list">
        {#each filteredSessions as session}
          <button class:selected={activeSession?.ID === session.ID} class="session-row" onclick={() => loadHistory(session)}>
            <span class={badgeClass(session.Origin)}>{session.Origin}</span>
            <strong>{session.Name || session.AgentName}</strong>
            <small>{session.Description || session.ID.slice(0, 8)}</small>
          </button>
        {/each}
      </div>
    </section>
  </aside>

  <section class="chat-shell">
    <header class="chat-header">
      <div>
        <div class="title-row">
          {#if activeSession}<span class={badgeClass(activeSession.Origin)}>{activeSession.Origin}</span>{/if}
          <h2>{sessionTitle}</h2>
        </div>
        <p>{sessionDescription}</p>
      </div>
      <div class="settings-strip">
        <input class="field model-field" bind:value={modelDraft} placeholder="model" aria-label="Model" />
        <button class="button secondary" onclick={setModel}>/model</button>
        <select class="field compact" value={activeSession?.Effort || activeAgent?.Effort || "medium"} onchange={(event) => runCommand(`/effort ${(event.currentTarget as HTMLSelectElement).value}`)} aria-label="Effort">
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
          <option value="xhigh">xhigh</option>
          <option value="max">max</option>
        </select>
        <select class="field compact" value={activeSession?.PermissionMode || activeAgent?.PermissionMode || "approve"} onchange={(event) => runCommand(`/permission ${(event.currentTarget as HTMLSelectElement).value}`)} aria-label="Permission">
          <option value="approve">approve</option>
          <option value="yolo">yolo</option>
        </select>
      </div>
    </header>

    <div class="slash-row">
      <button onclick={() => (messageText = "/model ")}>/model</button>
      <button onclick={() => (messageText = "/effort medium")} >/effort</button>
      <button onclick={() => (messageText = "/profile default")} >/profile</button>
      <button onclick={() => (messageText = "/permission approve")} >/permission</button>
      <button onclick={() => (messageText = "/name ")}>/name</button>
      <button onclick={() => (messageText = "/describe ")}>/describe</button>
      <button onclick={() => runCommand("/help")}>/help</button>
    </div>

    <div class="messages-pane">
      <div class="message-stack">
        {#each messages as message}
          <article class:assistant={message.Role === "assistant"} class="bubble">
            <div class="role">{message.Role}</div>
            <p>{message.Content}</p>
          </article>
        {/each}
        {#if pendingAssistant}
          <article class="bubble assistant streaming">
            <div class="role">assistant</div>
            <p>{pendingAssistant}</p>
          </article>
        {/if}
        {#if pendingPermission}
          <section class="permission-card">
            <div>
              <div class="permission-title">
                <strong>{pendingPermission.tool_name}</strong>
                <span>{permissionRemaining}s</span>
              </div>
              <pre>{JSON.stringify(pendingPermission.input, null, 2)}</pre>
            </div>
            <div class="permission-actions">
              <button class="button primary" onclick={() => decidePermission(true)}>Allow</button>
              <button class="button secondary" onclick={() => decidePermission(false)}>Deny</button>
            </div>
          </section>
        {/if}
        {#if notice}
          <div class="notice">{notice}</div>
        {/if}
        {#if error}
          <div class="error">{error}</div>
        {/if}
      </div>
    </div>

    <form class="composer" onsubmit={(event) => { event.preventDefault(); sendTurn(); }}>
      <input class="field composer-input" bind:value={messageText} placeholder="Message or slash command" aria-label="Message" />
      <button class="button primary send-button" disabled={sending || status !== "live"}>{sending ? "Sending" : "Send"}</button>
    </form>
  </section>

  {#if hireOpen}
    <div class="modal-backdrop" role="presentation" onclick={() => (hireOpen = false)}>
      <div class="modal" role="dialog" aria-modal="true" aria-label="Hire agent" tabindex="-1" onclick={stopModalEvent} onkeydown={stopModalEvent}>
        <header>
          <h2>Hire agent</h2>
          <button class="icon-button" onclick={() => (hireOpen = false)} title="Close">x</button>
        </header>
        <div class="modal-grid">
          <label>Name<input class="field" bind:value={hireName} /></label>
          <label>Provider
            <select class="field" bind:value={hireProvider}>
              <option value="claude">claude</option>
              <option value="codex">codex</option>
            </select>
          </label>
          <label>Permission
            <select class="field" bind:value={hirePermission}>
              <option value="approve">approve</option>
              <option value="yolo">yolo</option>
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
          <label class="full">Model<input class="field" bind:value={hireModel} placeholder="provider default" /></label>
        </div>
        <footer>
          <button class="button secondary" onclick={() => (hireOpen = false)}>Cancel</button>
          <button class="button primary" onclick={hireAgent}>Create</button>
        </footer>
      </div>
    </div>
  {/if}
</main>
