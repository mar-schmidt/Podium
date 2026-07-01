<script lang="ts">
  import { onMount, tick } from "svelte";
  import { deleteSession, getSession, listProjects } from "../lib/api";
  import { live } from "../lib/live.svelte";
  import ConfirmModal from "../lib/ConfirmModal.svelte";
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
    PermissionMode,
    PermissionRequest,
    Project,
    ServerMessage,
    Session,
    SessionOrigin,
    TurnState,
    UserInputQuestion,
    UserInputRequest,
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
  }: {
    agents?: Agent[];
    target?: ChatTarget | null;
    onConsumeTarget?: () => void;
  } = $props();

  // Connection + session-list state is owned by the shared live store so it
  // survives navigating away from the chat page (attention signalling must keep
  // working everywhere). This page reads it reactively and drives its own
  // per-session view state locally.
  const status = $derived(live.status);
  const sessions = $derived(live.sessions);
  const activeTurns = $derived(live.activeTurns);
  let activeSession = $state<Session | null>(null);
  let projectName = $state<string>("");

  // Session delete confirmation.
  let pendingDelete = $state<Session | null>(null);
  let deleteBusy = $state(false);
  let deleteError = $state<string | null>(null);
  let messages = $state<Message[]>([]);
  let pendingAssistant = $state("");
  let pendingPermission = $state<PermissionRequest | null>(null);
  let pendingUserInput = $state<UserInputRequest | null>(null);
  let userInputAnswers = $state<Record<string, string[]>>({});
  let permissionRemaining = $state(0);
  let messageText = $state("");
  let selectedAgent = $state("");
  let originFilter = $state<SessionOrigin | "all">("all");
  let agentFilter = $state("all");
  let projectFilter = $state("all");
  let projects = $state<Project[]>([]);
  let draftModel = $state("");
  let draftEffort = $state("");
  let draftPermissionMode = $state<PermissionMode | "">("");
  let draftProjectID = $state("");
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let sending = $state(false);
  let unsubscribe: (() => void) | undefined;
  let countdown: number | undefined;
  let pendingSeed: string | null = null;
  let msgsEl: HTMLDivElement | null = null;
  const LAST_SESSION_KEY = "podium:last-chat-session";

  // Layout / UI state.
  let sessOpen = $state(true);
  let ctxOpen = $state(false);
  let isPhone = $state(false);
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
  const curModel = $derived(
    activeSession ? activeSession.Model || activeAgent?.Model || "—" : draftModel || activeAgent?.Model || "—",
  );
  const curEffort = $derived(
    activeSession ? activeSession.Effort || activeAgent?.Effort || "medium" : draftEffort || activeAgent?.Effort || "medium",
  );
  const curMode = $derived(
    activeSession
      ? activeSession.PermissionMode || activeAgent?.PermissionMode || "approve"
      : draftPermissionMode || activeAgent?.PermissionMode || "approve",
  );
  const curProjectID = $derived(activeSession ? activeSession.ProjectID : draftProjectID);
  const linkedProjectName = $derived(curProjectID ? projectName || projectLabel(curProjectID) : "");
  const showSlash = $derived(messageText.startsWith("/"));
  const activeTurn = $derived(activeSession ? activeTurns[activeSession.ID] : undefined);

  function sessionSub(s: Session): string {
    return `${s.AgentName} · ${s.Provider}${s.Model ? " " + s.Model : ""}`;
  }

  onMount(() => {
    const mq = window.matchMedia("(max-width: 768px)");
    const syncPhone = () => {
      isPhone = mq.matches;
      if (mq.matches) {
        sessOpen = false;
        ctxOpen = false;
      }
    };
    syncPhone();
    mq.addEventListener("change", syncPhone);
    live.connect();
    unsubscribe = live.subscribe(handleServerMessage);
    listProjects().then((p) => (projects = p)).catch(() => {});
    restoreLastSession();
    countdown = window.setInterval(updatePermissionRemaining, 1000);
    return () => {
      mq.removeEventListener("change", syncPhone);
      if (countdown) window.clearInterval(countdown);
      unsubscribe?.();
    };
  });

  // When the shared socket (re)connects, re-attach to the open session and flush
  // any pending seed so a freshly opened chat still starts its turn.
  $effect(() => {
    if (live.status === "live") {
      attachActiveSession();
      flushSeed();
    }
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

  function send(msg: ClientMessage) {
    if (live.status !== "live") {
      error = "WebSocket is offline";
      return;
    }
    live.send(msg);
  }

  // handleServerMessage is registered with the live store and handles only this
  // page's view concerns; the store owns sessions/activeTurns/attention itself.
  function handleServerMessage(msg: ServerMessage) {
    switch (msg.type) {
      case "state":
        if (!selectedAgent && agents.length > 0) selectedAgent = agents[0].Name;
        if (activeSession) {
          const replacement = live.sessions.find((s) => s.ID === activeSession?.ID);
          if (replacement) activeSession = replacement;
          sending = !!live.activeTurns[activeSession.ID];
        }
        break;
      case "session":
        if (msg.session) {
          activeSession = msg.session;
          selectedAgent = msg.session.AgentName;
          rememberSession(msg.session.ID);
          if (!msg.session.ProjectID) projectName = "";
          resetDraftSettings();
        }
        break;
      case "turn_state":
        applyTurnState(msg.turn_state);
        break;
      case "history":
        messages = msg.history ?? [];
        pendingAssistant = "";
        pendingPermission = null;
        pendingUserInput = null;
        break;
      case "message":
        if (!messageForActiveSession(msg)) break;
        if (msg.message && !messages.some((e) => sameMessage(e, msg.message))) {
          messages = [...messages, msg.message];
          if (msg.message.Role === "assistant") {
            pendingAssistant = "";
            scrollMessagesToBottom();
          }
        }
        break;
      case "delta":
        if (!messageForActiveSession(msg)) break;
        pendingAssistant += msg.delta ?? "";
        break;
      case "assistant":
        if (!messageForActiveSession(msg)) break;
        if (!pendingAssistant) pendingAssistant = msg.delta ?? "";
        break;
      case "permission_request":
        if (!messageForActiveSession(msg)) break;
        pendingPermission = msg.request ?? null;
        updatePermissionRemaining();
        break;
      case "user_input_request":
        if (!messageForActiveSession(msg)) break;
        pendingUserInput = msg.input ?? null;
        userInputAnswers = initialUserInputAnswers(pendingUserInput);
        break;
      case "notice":
        notice = msg.notice ?? null;
        sending = false;
        break;
      case "done":
        if (messageForActiveSession(msg)) {
          pendingPermission = null;
          if (pendingUserInput?.provider !== "claude") pendingUserInput = null;
          sending = false;
        }
        window.setTimeout(() => live.send({ type: "list" }), 1200);
        break;
      case "error":
        if (messageForActiveSession(msg)) {
          error = msg.error ?? "Unknown server error";
          sending = false;
        }
        break;
    }
  }

  function rememberSession(id: string) {
    localStorage.setItem(LAST_SESSION_KEY, id);
  }

  function restoreLastSession() {
    const id = localStorage.getItem(LAST_SESSION_KEY);
    if (!id) return;
    void loadHistory({ ID: id } as Session);
  }

  function attachActiveSession() {
    if (activeSession?.ID && live.status === "live") {
      live.send({ type: "attach_session", request_id: crypto.randomUUID(), session_id: activeSession.ID });
    }
  }

  function messageForActiveSession(msg: ServerMessage): boolean {
    return !msg.session_id || msg.session_id === activeSession?.ID;
  }

  // applyTurnState updates only this page's view of the active session; the
  // store maintains the activeTurns map (and thus attention) independently.
  function applyTurnState(state: TurnState | undefined) {
    if (!state?.session_id) return;
    if (state.session_id !== activeSession?.ID) return;
    sending = state.status === "running";
    pendingAssistant = state.pending_assistant ?? "";
    pendingPermission = state.pending_permission ?? null;
    pendingUserInput = state.pending_user_input ?? null;
    if (pendingUserInput) userInputAnswers = initialUserInputAnswers(pendingUserInput);
    if (state.error) error = state.error;
    updatePermissionRemaining();
  }

  function sameMessage(a: Message, b: Message | undefined) {
    return !!b && a.ID === b.ID && a.SessionID === b.SessionID;
  }

  async function scrollMessagesToBottom() {
    await tick();
    requestAnimationFrame(() => {
      if (!msgsEl) return;
      msgsEl.scrollTo({ top: msgsEl.scrollHeight, behavior: "smooth" });
    });
  }

  async function loadHistory(session: Session) {
    error = null;
    try {
      const detail = await getSession(session.ID);
      activeSession = detail.session;
      selectedAgent = detail.session.AgentName;
      rememberSession(detail.session.ID);
      messages = detail.history ?? [];
      projectName = detail.project_name ?? (detail.session.ProjectID ? projectLabel(detail.session.ProjectID) : "");
      pendingAssistant = "";
      pendingPermission = null;
      pendingUserInput = null;
      sending = !!activeTurns[detail.session.ID];
      attachActiveSession();
      if (isPhone) sessOpen = false;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function confirmDeleteSession() {
    if (!pendingDelete) return;
    const id = pendingDelete.ID;
    deleteBusy = true;
    deleteError = null;
    try {
      await deleteSession(id);
      // The store owns the session list; refresh it from the daemon.
      live.send({ type: "list" });
      if (activeSession?.ID === id) {
        activeSession = null;
        messages = [];
        projectName = "";
        pendingAssistant = "";
        pendingPermission = null;
        pendingUserInput = null;
        sending = false;
        localStorage.removeItem(LAST_SESSION_KEY);
      }
      pendingDelete = null;
      send({ type: "list" });
    } catch (e) {
      deleteError = e instanceof Error ? e.message : String(e);
    } finally {
      deleteBusy = false;
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
    pendingPermission = null;
    pendingUserInput = null;
    send({
      type: "send_turn",
      request_id: crypto.randomUUID(),
      agent_name: activeSession ? undefined : selectedAgent,
      session_id: activeSession?.ID,
      message: text,
      model: activeSession ? undefined : draftModel || undefined,
      effort: activeSession ? undefined : draftEffort || undefined,
      permission_mode: activeSession ? undefined : draftPermissionMode || undefined,
      project_id: activeSession ? undefined : draftProjectID || undefined,
    });
    messageText = "";
  }

  function resetDraftSettings() {
    draftModel = "";
    draftEffort = "";
    draftPermissionMode = "";
    draftProjectID = "";
  }

  function newSession(resetDrafts = true) {
    activeSession = null;
    messages = [];
    projectName = "";
    localStorage.removeItem(LAST_SESSION_KEY);
    if (resetDrafts) resetDraftSettings();
    pendingAssistant = "";
    pendingPermission = null;
    pendingUserInput = null;
    notice = null;
    error = null;
    if (isPhone) sessOpen = false;
  }

  function startSessionWith(agentName: string) {
    selectedAgent = agentName;
    newSession(false);
    newSessionOpen = false;
  }

  function setModel(model: string) {
    if (activeSession) {
      updateSessionSettings({ model });
      return;
    }
    draftModel = model;
    openDropdown = null;
  }

  function setEffort(effort: string) {
    if (activeSession) {
      updateSessionSettings({ effort });
      return;
    }
    draftEffort = effort;
    openDropdown = null;
  }

  function setPermissionMode(mode: PermissionMode) {
    if (activeSession) {
      updateSessionSettings({ permission_mode: mode });
      return;
    }
    draftPermissionMode = mode;
    openDropdown = null;
  }

  function setDraftProject(projectID: string) {
    draftProjectID = projectID;
    projectName = "";
    openDropdown = null;
  }

  function updateSessionSettings(patch: { model?: string; effort?: string; permission_mode?: PermissionMode }) {
    openDropdown = null;
    if (!activeSession) return;
    send({
      type: "update_session_settings",
      request_id: crypto.randomUUID(),
      session_id: activeSession.ID,
      ...patch,
    });
  }

  function stopActiveTurn() {
    if (!activeSession) return;
    send({ type: "stop_turn", request_id: crypto.randomUUID(), session_id: activeSession.ID });
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

  function initialUserInputAnswers(req: UserInputRequest | null): Record<string, string[]> {
    const answers: Record<string, string[]> = {};
    for (const q of req?.questions ?? []) answers[q.id] = [];
    return answers;
  }

  function toggleUserInput(q: UserInputQuestion, value: string) {
    const current = userInputAnswers[q.id] ?? [];
    if (q.multi_select) {
      userInputAnswers = {
        ...userInputAnswers,
        [q.id]: current.includes(value) ? current.filter((v) => v !== value) : [...current, value],
      };
      return;
    }
    userInputAnswers = { ...userInputAnswers, [q.id]: [value] };
  }

  function setFreeUserInput(q: UserInputQuestion, value: string) {
    userInputAnswers = { ...userInputAnswers, [q.id]: value.trim() ? [value] : [] };
  }

  function userInputSelected(q: UserInputQuestion, value: string) {
    return (userInputAnswers[q.id] ?? []).includes(value);
  }

  function userInputReady(req: UserInputRequest | null) {
    return !!req && req.questions.every((q) => (userInputAnswers[q.id] ?? []).some((v) => v.trim()));
  }

  function submitUserInput() {
    const req = pendingUserInput;
    if (!req || !userInputReady(req)) return;
    const decision = { answers: userInputAnswers };
    pendingUserInput = null;
    if (req.provider === "claude") {
      send({ type: "user_input_decision", request_id: req.id, input: decision });
      sendTurn(formatUserInputFollowup(req, userInputAnswers));
      return;
    }
    send({ type: "user_input_decision", request_id: req.id, input: decision });
  }

  function formatUserInputFollowup(req: UserInputRequest, answers: Record<string, string[]>): string {
    if (req.questions.length === 1) {
      const q = req.questions[0];
      return `Answer to "${q.question}": ${(answers[q.id] ?? []).join(", ")}`;
    }
    return [
      "Answers:",
      ...req.questions.map((q) => `- ${q.question}: ${(answers[q.id] ?? []).join(", ")}`),
    ].join("\n");
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

  function closeMobilePanels() {
    if (!isPhone) return;
    sessOpen = false;
    ctxOpen = false;
    openDropdown = null;
  }

  function permissionCmd(input: Record<string, unknown>): string {
    return Object.entries(input)
      .map(([k, v]) => (k === "command" ? String(v) : `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`))
      .join("\n");
  }
</script>

<div class="chat" style="flex:1;display:flex;min-height:0">
  {#if isPhone && (sessOpen || ctxOpen)}
    <button class="mobile-panel-backdrop" aria-label="Close panel" onclick={closeMobilePanels}></button>
  {/if}

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
          <div class="sess-row-wrap">
            <button class="sess-row" class:sel={activeSession?.ID === s.ID} onclick={() => loadHistory(s)}>
              <span class="sess-avatar-wrap">
                <span style={avatarStyle(agentGradient(s.AgentName), 32, 10, 13)}>{initial(s.AgentName)}</span>
                {#if live.attention.has(s.ID)}
                  <span class="attn-dot" title="Needs your attention"></span>
                {/if}
              </span>
              <span class="sess-row-text">
                <span class="sess-row-title">{s.Name || s.AgentName}</span>
                <span class="sess-row-sub mono">{sessionSub(s)}</span>
              </span>
              {#if activeTurns[s.ID]}
                <span class="run-pill mono" class:needs={activeTurns[s.ID].pending === "permission" || activeTurns[s.ID].pending === "question"}>
                  {activeTurns[s.ID].pending === "permission" ? "approve" : activeTurns[s.ID].pending === "question" ? "question" : "running"}
                </span>
              {/if}
              <span style={originStyle(s.Origin)}>{originLabel(s.Origin)}</span>
            </button>
            <button class="sess-x" title="Delete session" aria-label="Delete session" onclick={() => (pendingDelete = s)}>
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18" /></svg>
            </button>
          </div>
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

    {#if linkedProjectName}
      <div class="proj-strip">
        <span class="proj-dot-sm" style="background:#3F8F7E"></span>
        <span class="mono proj-strip-text">part of <b>{linkedProjectName}</b></span>
      </div>
    {/if}

    <div class="msgs" bind:this={msgsEl}>
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

      {#if sending && !pendingAssistant && !pendingPermission && !pendingUserInput}
        <div class="row-start" style="align-items:center">
          <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
          <span class="thinking">
            <span class="tdot"></span><span class="tdot d2"></span><span class="tdot d3"></span>
          </span>
        </div>
      {/if}

      {#if pendingUserInput}
        <div class="row-start question-wrap">
          <div style={avatarStyle(activeGrad, 30, 10, 13)}>{activeMono}</div>
          <div class="question-card">
            <div class="question-head">
              <span class="approve-tag mono">question · {pendingUserInput.provider ?? "provider"}</span>
            </div>
            <div class="question-body">
              {#each pendingUserInput.questions as q}
                <div class="question-block">
                  {#if q.header}<div class="question-header">{q.header}</div>{/if}
                  <div class="question-text">{q.question}</div>
                  {#if q.options && q.options.length > 0}
                    <div class="question-options">
                      {#each q.options as option}
                        <button
                          class="question-option"
                          class:sel={userInputSelected(q, option.label)}
                          onclick={() => toggleUserInput(q, option.label)}
                        >
                          <span class="question-dot">{q.multi_select ? (userInputSelected(q, option.label) ? "✓" : "") : ""}</span>
                          <span class="question-option-text">
                            <span>{option.label}</span>
                            {#if option.description}<small>{option.description}</small>{/if}
                          </span>
                        </button>
                      {/each}
                    </div>
                  {:else}
                    <input
                      class="question-free"
                      type={q.is_secret ? "password" : "text"}
                      placeholder="Answer"
                      value={(userInputAnswers[q.id] ?? [])[0] ?? ""}
                      oninput={(e) => setFreeUserInput(q, e.currentTarget.value)}
                    />
                  {/if}
                </div>
              {/each}
              <div class="approve-actions">
                <button class="approve-yes" disabled={!userInputReady(pendingUserInput)} onclick={submitUserInput}>Send answer</button>
              </div>
            </div>
          </div>
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
        {#if activeTurn}
          <button class="composer-stop" title="Stop active turn" onclick={stopActiveTurn}>■</button>
        {:else}
          <button class="composer-send" disabled={sending || status !== "live"} onclick={() => sendTurn()}>↑</button>
        {/if}
      </div>
      <div class="composer-meta">
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("model")}>/model {curModel} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "model"}
            <div class="dd-menu up">
              {#each modelOptions as o}
                <button class="dd-opt" class:sel={o === curModel} onclick={() => setModel(o)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("effort")}>/effort {curEffort} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "effort"}
            <div class="dd-menu up">
              {#each EFFORTS as o}
                <button class="dd-opt" class:sel={o === curEffort} onclick={() => setEffort(o)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="chip-btn mono" onclick={() => toggleDropdown("perm")}>/permission {curMode} <span style="opacity:.55">▾</span></button>
          {#if openDropdown === "perm"}
            <div class="dd-menu up">
              {#each ["approve", "yolo"] as o}
                <button class="dd-opt" class:sel={o === curMode} onclick={() => setPermissionMode(o as PermissionMode)}>{o}</button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="dd-wrap">
          <button class="chip-btn mono" class:mutedChip={!!activeSession} onclick={() => { if (!activeSession) toggleDropdown("project"); }} title={activeSession ? "Project is fixed for this session" : "Project for this new session"}>
            /project {curProjectID ? projectLabel(curProjectID) : "none"} <span style="opacity:.55">▾</span>
          </button>
          {#if openDropdown === "project" && !activeSession}
            <div class="dd-menu up">
              <button class="dd-opt" class:sel={!draftProjectID} onclick={() => setDraftProject("")}>no project</button>
              {#each projects as p}
                <button class="dd-opt" class:sel={draftProjectID === p.id} onclick={() => setDraftProject(p.id)}>{p.name}</button>
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

      {#if linkedProjectName}
        <div class="label-mono" style="margin:24px 0 10px">linked project</div>
        <div class="ctx-proj">
          <div class="ctx-proj-row">
            <span class="proj-dot-sm" style="background:#3F8F7E"></span>
            <span class="ctx-proj-name">{linkedProjectName}</span>
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
      <div class="ns-controls">
        <div class="dd-wrap">
          <button class="filter-chip ns-project-chip" onclick={() => toggleDropdown("nsProject")}>
            {draftProjectID ? projectLabel(draftProjectID) : "no project"} <span style="opacity:.55">▾</span>
          </button>
          {#if openDropdown === "nsProject"}
            <div class="dd-menu">
              <button class="dd-opt" class:sel={!draftProjectID} onclick={() => setDraftProject("")}>no project</button>
              {#each projects as p}
                <button class="dd-opt" class:sel={draftProjectID === p.id} onclick={() => setDraftProject(p.id)}>{p.name}</button>
              {/each}
            </div>
          {/if}
        </div>
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

{#if pendingDelete}
  <ConfirmModal
    title="Delete session"
    message="This permanently removes {pendingDelete.Name || pendingDelete.AgentName} and its chat history. This cannot be undone."
    confirmLabel="Delete session"
    busy={deleteBusy}
    error={deleteError}
    onConfirm={confirmDeleteSession}
    onCancel={() => (pendingDelete = null)}
  />
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

  .sess-row-wrap {
    position: relative;
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

  .sess-x {
    position: absolute;
    top: 5px;
    right: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: #b98b7c;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.12s ease;
  }

  .sess-row-wrap:hover .sess-x {
    opacity: 1;
  }

  .sess-x:hover {
    background: #fbeeea;
    color: #a23e22;
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

  .run-pill {
    flex: none;
    padding: 3px 7px;
    border-radius: 999px;
    border: 1px solid #cfe3d8;
    background: #eaf5f0;
    color: var(--teal-deep);
    font-size: 9.5px;
    font-weight: 700;
    text-transform: uppercase;
  }

  .run-pill.needs {
    border-color: #f0dca9;
    background: #fbf1dd;
    color: #9a6e1e;
  }

  .sess-avatar-wrap {
    position: relative;
    display: inline-flex;
    flex: none;
  }

  /* Red dot flags a session that is blocked on the user (permission/question). */
  .attn-dot {
    position: absolute;
    top: -2px;
    right: -2px;
    width: 11px;
    height: 11px;
    border-radius: 999px;
    background: #d64528;
    border: 2px solid var(--surface);
    box-shadow: 0 0 0 2px rgba(214, 69, 40, 0.22);
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

  .question-wrap {
    margin-left: 0;
    max-width: 640px;
  }

  .approve-card {
    flex: 1;
    background: #fff;
    border: 1px solid #f0dca9;
    border-radius: 15px;
    overflow: hidden;
    box-shadow: 0 8px 22px -14px rgba(154, 110, 30, 0.32);
  }

  .question-card {
    flex: 1;
    background: #fff;
    border: 1px solid #cfe3d8;
    border-radius: 15px;
    overflow: hidden;
    box-shadow: 0 8px 22px -14px rgba(63, 143, 126, 0.32);
  }

  .approve-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 11px 15px;
    background: #fbf1dd;
    border-bottom: 1px solid #f0dca9;
  }

  .question-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 11px 15px;
    background: #eaf5f0;
    border-bottom: 1px solid #cfe3d8;
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

  .question-body {
    padding: 14px 15px;
  }

  .question-block + .question-block {
    margin-top: 15px;
    padding-top: 14px;
    border-top: 1px solid var(--line-3);
  }

  .question-header {
    font: 700 11px "JetBrains Mono", monospace;
    color: var(--teal-deep);
    text-transform: uppercase;
    margin-bottom: 6px;
  }

  .question-text {
    font: 700 15px/1.35 "Hanken Grotesk";
    color: var(--ink);
    margin-bottom: 10px;
  }

  .question-options {
    display: grid;
    gap: 8px;
  }

  .question-option {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    width: 100%;
    min-height: 44px;
    padding: 10px 11px;
    border: 1px solid var(--line-3);
    border-radius: 10px;
    background: var(--surface-3);
    color: var(--ink);
    cursor: pointer;
    text-align: left;
  }

  .question-option.sel {
    border-color: #9bcdbd;
    background: #eaf5f0;
  }

  .question-dot {
    width: 17px;
    height: 17px;
    flex: none;
    border-radius: 50%;
    border: 1px solid #9bcdbd;
    color: var(--teal-deep);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    line-height: 1;
    margin-top: 1px;
  }

  .question-option.sel .question-dot {
    background: var(--teal);
    border-color: var(--teal);
    box-shadow: inset 0 0 0 4px #fff;
  }

  .question-option-text {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
    font: 700 13.5px/1.25 "Hanken Grotesk";
  }

  .question-option-text small {
    font: 500 12px/1.3 "Hanken Grotesk";
    color: var(--muted);
  }

  .question-free {
    width: 100%;
    min-height: 40px;
    border: 1px solid var(--field-line);
    border-radius: 10px;
    padding: 9px 11px;
    font: 500 13.5px "Hanken Grotesk";
    color: var(--ink);
    background: #fff;
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

  .approve-yes:disabled {
    opacity: 0.45;
    cursor: default;
    box-shadow: none;
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

  .composer-stop {
    width: 36px;
    height: 36px;
    border: 1px solid #e7c3b5;
    border-radius: 11px;
    background: #fbeeea;
    color: #a23e22;
    font-size: 13px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
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

  .ns-controls {
    padding: 14px 26px 0;
    display: flex;
    justify-content: flex-start;
  }

  .ns-project-chip {
    max-width: 240px;
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

  .mutedChip {
    cursor: default;
    opacity: 0.72;
  }

  .mobile-panel-backdrop {
    display: none;
  }

  @media (max-width: 768px) {
    .mobile-panel-backdrop {
      display: block;
      position: fixed;
      inset: 0 0 calc(72px + env(safe-area-inset-bottom)) 0;
      z-index: 30;
      border: none;
      background: rgba(43, 37, 32, 0.22);
      backdrop-filter: blur(1px);
      padding: 0;
    }

    .chat {
      width: 100%;
      min-width: 0;
      overflow: hidden;
    }

    .sess-col,
    .ctx {
      position: fixed;
      top: 0;
      bottom: calc(72px + env(safe-area-inset-bottom));
      z-index: 35;
      width: min(88vw, 340px);
      max-width: calc(100vw - 34px);
      box-shadow: 18px 0 42px -32px rgba(43, 37, 32, 0.58);
    }

    .sess-col {
      left: 0;
      border-right: 1px solid var(--line);
    }

    .ctx {
      right: 0;
      border-left: 1px solid var(--line);
      padding: 16px 18px 20px;
      box-shadow: -18px 0 42px -32px rgba(43, 37, 32, 0.58);
    }

    .conv {
      width: 100%;
      min-width: 0;
    }

    .conv-head {
      padding: 12px 14px;
      gap: 9px;
    }

    .conv-title {
      font-size: 15px;
    }

    .proj-strip {
      padding: 6px 14px;
    }

    .msgs {
      padding: 18px 14px;
      gap: 14px;
    }

    .row-start,
    .approve-wrap {
      max-width: 100%;
    }

    .row-start {
      gap: 9px;
    }

    .bubble-user {
      max-width: 88%;
      padding: 11px 13px;
      font-size: 14.5px;
    }

    .bubble-assistant,
    .bubble-error {
      padding: 11px 13px;
      font-size: 14.5px;
    }

    .approve-head {
      align-items: flex-start;
      flex-direction: column;
      gap: 4px;
    }

    .approve-head span[style*="flex:1"] {
      display: none;
    }

    .approve-actions {
      flex-direction: column;
    }

    .composer {
      padding: 10px 12px 12px;
    }

    .composer-box {
      gap: 8px;
      padding: 7px 7px 7px 12px;
    }

    .composer-input {
      min-width: 0;
      font-size: 14.5px;
    }

    .composer-send {
      width: 34px;
      height: 34px;
    }

    .composer-meta {
      flex-wrap: nowrap;
      overflow-x: auto;
      padding-bottom: 2px;
      scrollbar-width: none;
    }

    .composer-meta::-webkit-scrollbar {
      display: none;
    }

    .composer-meta .dd-wrap,
    .chip-btn {
      flex: none;
    }

    .slash-menu {
      left: 12px;
      right: 12px;
      bottom: 82px;
      max-width: none;
    }

    .slash-item {
      align-items: flex-start;
      flex-direction: column;
      gap: 2px;
    }

    .slash-cmd {
      min-width: 0;
    }

    .ns-list {
      max-height: calc(100dvh - 138px);
      overflow-y: auto;
      padding: 14px;
    }

    .ns-row-head {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
