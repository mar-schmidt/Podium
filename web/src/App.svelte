<script lang="ts">
  import { onMount } from "svelte";
  import type {
    Agent,
    ClientMessage,
    Health,
    Message,
    PermissionRequest,
    ServerMessage,
    Session,
  } from "./lib/types";

  let health = $state<Health | null>(null);
  let status = $state<"connecting" | "live" | "offline">("connecting");
  let agents = $state<Agent[]>([]);
  let sessions = $state<Session[]>([]);
  let activeSession = $state<Session | null>(null);
  let messages = $state<Message[]>([]);
  let pendingAssistant = $state("");
  let pendingPermission = $state<PermissionRequest | null>(null);
  let agentName = $state("web-agent");
  let messageText = $state("");
  let newAgentName = $state("web-agent");
  let permissionMode = $state<"approve" | "yolo">("approve");
  let error = $state<string | null>(null);
  let sending = $state(false);
  let ws: WebSocket | null = null;

  const activeAgent = $derived(agents.find((agent) => agent.Name === activeSession?.AgentName));

  onMount(() => {
    void fetchHealth();
    connect();
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
        if (!activeSession && sessions.length > 0) activeSession = sessions[0];
        break;
      case "session":
        if (msg.session) {
          activeSession = msg.session;
          if (!sessions.some((session) => session.ID === msg.session?.ID)) {
            sessions = [msg.session, ...sessions];
          }
        }
        break;
      case "history":
        messages = msg.history ?? [];
        pendingAssistant = "";
        break;
      case "message":
        if (msg.message && !messages.some((existing) => existing.ID === msg.message?.ID)) {
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
        break;
      case "done":
        pendingPermission = null;
        sending = false;
        break;
      case "error":
        error = msg.error ?? "Unknown server error";
        sending = false;
        break;
    }
  }

  async function createAgent() {
    error = null;
    const res = await fetch("/api/agents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name: newAgentName,
        provider: "claude",
        permission_mode: permissionMode,
      }),
    });
    if (!res.ok) {
      error = await res.text();
      return;
    }
    const agent = (await res.json()) as Agent;
    agents = [agent, ...agents.filter((item) => item.Name !== agent.Name)];
    agentName = agent.Name;
  }

  async function selectSession(session: Session) {
    error = null;
    const res = await fetch(`/api/sessions/${session.ID}`);
    if (!res.ok) {
      error = await res.text();
      return;
    }
    const detail = (await res.json()) as { session: Session; history: Message[] };
    activeSession = detail.session;
    messages = detail.history;
    pendingAssistant = "";
  }

  function sendTurn() {
    const text = messageText.trim();
    if (!text) return;
    error = null;
    sending = true;
    pendingAssistant = "";
    send({
      type: "send_turn",
      request_id: crypto.randomUUID(),
      agent_name: activeSession ? undefined : agentName,
      session_id: activeSession?.ID,
      message: text,
    });
    messageText = "";
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
</script>

<main class="grid min-h-screen grid-cols-[280px_1fr] bg-[var(--color-bg)] text-[var(--color-ink)]">
  <aside class="border-r border-black/10 bg-[var(--color-surface)] px-4 py-4">
    <div class="mb-5 flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold">Podium</h1>
        <p class="text-xs text-[var(--color-muted)]">{health?.version ?? "local"}</p>
      </div>
      <span class:live={status === "live"} class="status-dot" title={status}></span>
    </div>

    <section class="mb-5">
      <h2 class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">Agents</h2>
      <div class="space-y-2">
        <input class="field" bind:value={newAgentName} aria-label="Agent name" />
        <select class="field" bind:value={permissionMode} aria-label="Permission mode">
          <option value="approve">approve</option>
          <option value="yolo">yolo</option>
        </select>
        <button class="button w-full" onclick={createAgent}>Create</button>
      </div>
      <div class="mt-3 space-y-1">
        {#each agents as agent}
          <button class="list-button" class:selected={agent.Name === agentName} onclick={() => (agentName = agent.Name)}>
            <span>{agent.Name}</span>
            <small>{agent.Provider} · {agent.PermissionMode}</small>
          </button>
        {/each}
      </div>
    </section>

    <section>
      <h2 class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">Sessions</h2>
      <div class="space-y-1">
        {#each sessions as session}
          <button class="list-button" class:selected={activeSession?.ID === session.ID} onclick={() => selectSession(session)}>
            <span>{session.AgentName}</span>
            <small>{session.Origin} · {session.ID.slice(0, 8)}</small>
          </button>
        {/each}
      </div>
    </section>
  </aside>

  <section class="grid min-h-screen grid-rows-[auto_1fr_auto]">
    <header class="border-b border-black/10 bg-white/45 px-6 py-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">{activeSession?.AgentName ?? agentName}</h2>
          <p class="text-sm text-[var(--color-muted)]">
            {activeSession?.ID ?? "new web session"}
            {#if activeAgent} · {activeAgent.Provider} · {activeAgent.PermissionMode}{/if}
          </p>
        </div>
        <button class="button secondary" onclick={() => ((activeSession = null), (messages = []), (pendingAssistant = ""))}>
          New
        </button>
      </div>
    </header>

    <div class="overflow-auto px-6 py-5">
      <div class="mx-auto flex max-w-3xl flex-col gap-3">
        {#each messages as message}
          <article class:assistant={message.Role === "assistant"} class="bubble">
            <div class="role">{message.Role}</div>
            <p>{message.Content}</p>
          </article>
        {/each}
        {#if pendingAssistant}
          <article class="bubble assistant">
            <div class="role">assistant</div>
            <p>{pendingAssistant}</p>
          </article>
        {/if}
        {#if pendingPermission}
          <div class="permission">
            <div>
              <strong>{pendingPermission.tool_name}</strong>
              <pre>{JSON.stringify(pendingPermission.input, null, 2)}</pre>
            </div>
            <div class="flex gap-2">
              <button class="button" onclick={() => decidePermission(true)}>Allow</button>
              <button class="button secondary" onclick={() => decidePermission(false)}>Deny</button>
            </div>
          </div>
        {/if}
        {#if error}
          <div class="error">{error}</div>
        {/if}
      </div>
    </div>

    <form class="border-t border-black/10 bg-[var(--color-surface)] p-4" onsubmit={(event) => { event.preventDefault(); sendTurn(); }}>
      <div class="mx-auto flex max-w-3xl gap-2">
        <input class="field flex-1" bind:value={messageText} placeholder="Message" aria-label="Message" />
        <button class="button" disabled={sending || status !== "live"}>{sending ? "Sending" : "Send"}</button>
      </div>
    </form>
  </section>
</main>
