<script lang="ts">
  import { onMount } from "svelte";
  import { getSession } from "../lib/api";
  import type {
    Agent,
    ClientMessage,
    Message,
    PermissionRequest,
    ServerMessage,
    Session,
    SessionOrigin,
  } from "../lib/types";

  // Cross-page navigation target: open an existing session and optionally send a
  // seed message (used by the Roadmap "Start"/"Open in chat" actions).
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
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let sending = $state(false);
  let modelDraft = $state("");
  let ws: WebSocket | null = null;
  let poll: number | undefined;
  let countdown: number | undefined;
  let pendingSeed: string | null = null;

  const activeAgent = $derived(
    agents.find((agent) => agent.Name === activeSession?.AgentName || agent.Name === selectedAgent),
  );
  const filteredSessions = $derived(
    sessions.filter((session) => {
      if (originFilter !== "all" && session.Origin !== originFilter) return false;
      if (agentFilter !== "all" && session.AgentName !== agentFilter) return false;
      return true;
    }),
  );
  const sessionTitle = $derived(
    activeSession ? activeSession.Name || activeSession.AgentName : selectedAgent || "New session",
  );

  onMount(() => {
    connect();
    poll = window.setInterval(() => send({ type: "list" }), 4000);
    countdown = window.setInterval(updatePermissionRemaining, 1000);
    return () => {
      if (poll) window.clearInterval(poll);
      if (countdown) window.clearInterval(countdown);
      ws?.close();
    };
  });

  // React to a cross-page navigation request.
  $effect(() => {
    const t = target;
    if (!t) return;
    onConsumeTarget();
    void openTarget(t);
  });

  async function openTarget(t: ChatTarget) {
    if (t.sessionId) {
      const session = sessions.find((s) => s.ID === t.sessionId) ?? { ID: t.sessionId } as Session;
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
          const replacement = sessions.find((session) => session.ID === activeSession?.ID);
          if (replacement) activeSession = replacement;
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

  async function loadHistory(session: Session) {
    error = null;
    try {
      const detail = await getSession(session.ID);
      activeSession = detail.session;
      selectedAgent = detail.session.AgentName;
      messages = detail.history ?? [];
      projectName = detail.project_name ?? "";
      pendingAssistant = "";
      modelDraft = detail.session.Model;
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
    permissionRemaining = Math.max(
      0,
      Math.ceil((new Date(pendingPermission.expires_at).getTime() - Date.now()) / 1000),
    );
  }

  function badgeClass(origin: SessionOrigin) {
    return `badge ${origin}`;
  }
</script>

<div class="chat-layout">
  <aside class="session-pane">
    <div class="pane-head">
      <span>Sessions</span>
      <button class="icon-button" onclick={newSession} title="New session">+</button>
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
      {#if filteredSessions.length === 0}
        <p class="empty">No sessions yet. Pick an agent and say hello.</p>
      {/if}
    </div>
  </aside>

  <section class="chat-shell">
    <header class="chat-header">
      <div>
        <div class="title-row">
          {#if activeSession}<span class={badgeClass(activeSession.Origin)}>{activeSession.Origin}</span>{/if}
          <h2>{sessionTitle}</h2>
        </div>
        {#if activeSession?.Origin === "roadmap" && projectName}
          <p class="provenance">part of <strong>{projectName}</strong></p>
        {:else}
          <p>{activeSession?.Description || "Ready"}</p>
        {/if}
      </div>
      <div class="settings-strip">
        {#if !activeSession}
          <select class="field compact" bind:value={selectedAgent} aria-label="Agent">
            {#each agents as agent}
              <option value={agent.Name}>{agent.Name}</option>
            {/each}
          </select>
        {/if}
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
      <button onclick={() => (messageText = "/effort medium")}>/effort</button>
      <button onclick={() => (messageText = "/profile default")}>/profile</button>
      <button onclick={() => (messageText = "/permission approve")}>/permission</button>
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
        {#if notice}<div class="notice">{notice}</div>{/if}
        {#if error}<div class="error">{error}</div>{/if}
      </div>
    </div>

    <form class="composer" onsubmit={(event) => { event.preventDefault(); sendTurn(); }}>
      <input class="field composer-input" bind:value={messageText} placeholder="Message or slash command" aria-label="Message" />
      <button class="button primary send-button" disabled={sending || status !== "live"}>{sending ? "Sending" : "Send"}</button>
    </form>
  </section>
</div>
