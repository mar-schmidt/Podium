<script lang="ts">
  import { deleteAgent, getAgent, listProfiles, updateAgent } from "../lib/api";
  import { agentGradient, avatarStyle, initial, modeChip, providerChip } from "../lib/theme";
  import type { Agent, ProfileInfo } from "../lib/types";

  // A fallback chain row: a provider plus an optional profile. Encodes to a
  // single token (profile name when set, otherwise the bare provider).
  type FbRow = { provider: string; profile: string };

  interface ChatTarget {
    sessionId?: string;
    agentName?: string;
    seed?: string;
  }

  let {
    agents = [],
    onHire = () => {},
    onOpenChat = (_t: ChatTarget) => {},
    onChanged = () => {},
  }: {
    agents?: Agent[];
    onHire?: () => void;
    onOpenChat?: (t: ChatTarget) => void;
    onChanged?: () => void;
  } = $props();

  let selected = $state<Agent | null>(null);

  // Edit modal state.
  let editOpen = $state(false);
  let edName = $state("");
  let edProvider = $state("claude");
  let edModel = $state("");
  let edEffort = $state("high");
  let edProfile = $state("");
  let edPermission = $state("approve");
  let edSoul = $state("");
  let edFallback = $state<FbRow[]>([]);
  let profiles = $state<ProfileInfo[]>([]);
  let saving = $state(false);
  let editError = $state<string | null>(null);

  // Delete modal state.
  let deleteOpen = $state(false);
  let deleteName = $state("");
  let deleting = $state(false);
  let deleteError = $state<string | null>(null);

  const EFFORTS = ["low", "medium", "high", "xhigh", "max"];
  const modelOptions = $derived(
    edProvider === "codex" ? ["gpt-5.1", "gpt-5.1-mini", "o4"] : ["sonnet", "opus", "haiku"],
  );

  function specs(a: Agent): string {
    return `${a.Model || a.Provider} · ${a.Effort || "medium"} · profile: ${a.Profile || "default"}`;
  }

  // Decode a stored fallback chain into editable rows. A profile name resolves
  // to its provider; a bare provider token (or legacy "default") becomes a
  // profile-less row pinned to that provider.
  function decodeFallback(tokens: string[], agentProvider: string): FbRow[] {
    return (tokens ?? []).map((tok) => {
      if (tok === "claude" || tok === "codex") return { provider: tok, profile: "" };
      if (tok === "default") return { provider: agentProvider, profile: "" };
      const p = profiles.find((pr) => pr.Name === tok);
      return { provider: p ? p.Provider : agentProvider, profile: tok };
    });
  }

  function encodeFallback(rows: FbRow[]): string[] {
    return rows.map((r) => r.profile || r.provider);
  }

  // Profiles selectable for a row's provider, plus the row's current value if
  // it isn't in that set (so stale/unknown profiles survive a round-trip).
  function profileOptions(provider: string, current: string): string[] {
    const names = profiles.filter((p) => p.Provider === provider).map((p) => p.Name);
    if (current && !names.includes(current)) names.push(current);
    return names;
  }

  function setRowProvider(i: number, provider: string) {
    const row = edFallback[i];
    // Drop the profile if it doesn't belong to the newly chosen provider.
    const valid = profiles.some((p) => p.Name === row.profile && p.Provider === provider);
    edFallback[i] = { provider, profile: valid ? row.profile : "" };
  }

  function addRow() {
    edFallback = [...edFallback, { provider: edProvider, profile: "" }];
  }

  function removeRow(i: number) {
    edFallback = edFallback.filter((_, idx) => idx !== i);
  }

  function moveRow(i: number, delta: number) {
    const j = i + delta;
    if (j < 0 || j >= edFallback.length) return;
    const next = [...edFallback];
    [next[i], next[j]] = [next[j], next[i]];
    edFallback = next;
  }

  async function openEdit(a: Agent) {
    editError = null;
    edName = a.Name;
    edProvider = a.Provider;
    edModel = a.Model;
    edEffort = a.Effort || "high";
    edProfile = a.Profile;
    edPermission = a.PermissionMode;
    edSoul = "";
    edFallback = decodeFallback(a.Fallback, a.Provider);
    editOpen = true;
    try {
      profiles = await listProfiles();
      // Re-decode now that provider info is available for profile tokens.
      edFallback = decodeFallback(a.Fallback, a.Provider);
    } catch {
      // Profiles are optional; dropdowns just stay empty.
    }
    try {
      const detail = await getAgent(a.Name);
      edSoul = detail.Soul;
    } catch {
      // SOUL is optional; leave blank on error.
    }
  }

  async function save() {
    saving = true;
    editError = null;
    try {
      const detail = await updateAgent(edName, {
        provider: edProvider,
        model: edModel,
        effort: edEffort,
        profile: edProfile,
        permission_mode: edPermission,
        fallback: encodeFallback(edFallback),
        soul: edSoul,
      });
      // Reflect the saved engine fields in the detail view.
      selected = {
        Name: detail.Name,
        Provider: detail.Provider,
        Profile: detail.Profile,
        Model: detail.Model,
        Effort: detail.Effort,
        PermissionMode: detail.PermissionMode,
        Fallback: detail.Fallback,
      };
      editOpen = false;
      onChanged();
    } catch (e) {
      editError = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  function openDelete(a: Agent) {
    deleteName = "";
    deleteError = null;
    deleteOpen = true;
    edName = a.Name;
  }

  async function confirmDelete() {
    if (!selected || deleteName.trim() !== selected.Name) return;
    deleting = true;
    deleteError = null;
    try {
      await deleteAgent(selected.Name, deleteName);
      deleteOpen = false;
      selected = null;
      onChanged();
    } catch (e) {
      deleteError = e instanceof Error ? e.message : String(e);
    } finally {
      deleting = false;
    }
  }

  function seg(on: boolean): string {
    return (
      "flex:1;padding:11px;border-radius:11px;cursor:pointer;font:600 13.5px 'Hanken Grotesk';" +
      (on
        ? "border:1px solid #BFE0D6;background:#E3F1EC;color:#2F6E60"
        : "border:1px solid #EAE0D4;background:#fff;color:#6F6459")
    );
  }

  function chip(on: boolean): string {
    return (
      "padding:6px 12px;border-radius:9px;cursor:pointer;font:600 12px 'JetBrains Mono',monospace;" +
      (on
        ? "border:1px solid #BFE0D6;background:#E3F1EC;color:#2F6E60"
        : "border:1px solid #EAE0D4;background:#fff;color:#6F6459")
    );
  }
</script>

{#if !selected}
  <div class="page">
    <header style="margin-bottom:22px">
      <div class="agents-head-row" style="display:flex;align-items:flex-end;gap:14px">
        <div>
          <h1 style="margin:0;font:800 24px 'Hanken Grotesk';letter-spacing:-.02em">Agents</h1>
          <p style="margin:3px 0 0;font:400 13px 'Hanken Grotesk';color:var(--muted-2)">Your roster of named colleagues. Each owns a workspace, a soul, and its own defaults.</p>
        </div>
        <span class="spacer"></span>
        <button class="head-cta" onclick={onHire}>+ Hire agent</button>
      </div>
    </header>

    <div class="roster">
      {#each agents as a}
        <button class="agent-card" onclick={() => (selected = a)}>
          <span style={avatarStyle(agentGradient(a.Name), 56, 17, 23)}>{initial(a.Name)}</span>
          <div class="ac-body">
            <div class="ac-head">
              <span class="ac-name">{a.Name}</span>
              <span style={providerChip(a.Provider)}>{a.Provider}</span>
              <span style={modeChip(a.PermissionMode)}>{a.PermissionMode}</span>
            </div>
            <div class="ac-specs mono">{specs(a)}</div>
          </div>
        </button>
      {/each}
      <button class="agent-add" onclick={onHire}>
        <span style="font-size:26px;line-height:1">+</span> Hire a new agent
      </button>
    </div>
  </div>
{:else}
  {@const a = selected}
  <div class="page">
    <button class="back-btn" onclick={() => (selected = null)}>← All agents</button>
    <div class="ad-top">
      <span style={avatarStyle(agentGradient(a.Name), 80, 24, 32)}>{initial(a.Name)}</span>
      <div style="flex:1">
        <div class="ad-headrow">
          <span class="ad-name">{a.Name}</span>
          <span style={providerChip(a.Provider)}>{a.Provider}</span>
          <span style={modeChip(a.PermissionMode)}>{a.PermissionMode}</span>
        </div>
        <div class="ad-soul">Runs on {a.Provider} · {a.Model || "provider default"} · effort {a.Effort || "medium"}.</div>
        <div class="ad-actions">
          <button class="head-cta" onclick={() => onOpenChat({ agentName: a.Name })}>Start a chat</button>
          <button class="ad-edit" onclick={() => openEdit(a)}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
            Edit
          </button>
          <button class="ad-delete" onclick={() => openDelete(a)}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18" /><path d="M8 6V4h8v2" /><path d="M19 6l-1 14H6L5 6" /><path d="M10 11v5" /><path d="M14 11v5" /></svg>
            Delete
          </button>
        </div>
      </div>
    </div>

    <div class="ad-grid">
      <div class="ad-panel">
        <div class="label-mono" style="margin-bottom:14px">defaults</div>
        <div class="ad-spec"><span>Provider</span><span class="mono">{a.Provider}</span></div>
        <div class="ad-spec"><span>Model</span><span class="mono">{a.Model || "provider default"}</span></div>
        <div class="ad-spec"><span>Effort</span><span class="mono">{a.Effort || "medium"}</span></div>
        <div class="ad-spec"><span>Profile</span><span class="mono">{a.Profile || "default"}</span></div>
        <div class="ad-spec"><span>Fallback</span><span class="mono">{a.Fallback && a.Fallback.length ? a.Fallback.join(" → ") : "none"}</span></div>
        <div class="ad-spec"><span>Permission</span><span class="mono">{a.PermissionMode}</span></div>
        <div class="ad-spec"><span>Workspace</span><span class="mono">~/.podium/agents/{a.Name}</span></div>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Edit modal ===== -->
{#if editOpen}
  <div class="modal-backdrop" role="presentation" onclick={() => (editOpen = false)}>
    <div class="modal-card ed-modal" role="dialog" aria-modal="true" aria-label="Edit agent" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="modal-head">
        <div class="modal-title">Edit {edName}</div>
        <div class="modal-sub">Tune how this colleague runs and rewrite their SOUL.md. Changes save to their durable defaults and workspace.</div>
      </div>
      <div class="modal-body" style="max-height:74vh;overflow-y:auto">
        {#if editError}<div class="error-banner" style="margin-bottom:14px">{editError}</div>{/if}

        <div class="label-mono" style="margin-bottom:8px">provider</div>
        <div style="display:flex;gap:9px">
          <button style={seg(edProvider === "claude")} onclick={() => { edProvider = "claude"; }}>Claude</button>
          <button style={seg(edProvider === "codex")} onclick={() => { edProvider = "codex"; }}>Codex</button>
        </div>

        <div class="ed-row">
          <span class="ed-key">model</span>
          <div class="ed-chips">
            {#each modelOptions as m}
              <button style={chip(m === edModel)} onclick={() => (edModel = m)}>{m}</button>
            {/each}
          </div>
        </div>
        <div class="ed-row">
          <span class="ed-key">effort</span>
          <div class="ed-chips">
            {#each EFFORTS as e}
              <button style={chip(e === edEffort)} onclick={() => (edEffort = e)}>{e}</button>
            {/each}
          </div>
        </div>
        <div class="ed-row">
          <span class="ed-key">profile</span>
          <input class="field-input" style="flex:1" bind:value={edProfile} placeholder="auth profile (optional)" />
        </div>

        <div class="label-mono" style="margin:18px 0 4px">fallback chain</div>
        <div class="fb-hint">Tried in order when the provider rate-limits. Pick a provider; add a profile if one exists.</div>
        {#each edFallback as row, i (i)}
          <div class="fb-row">
            <div class="fb-provs">
              <button style={chip(row.provider === "claude")} onclick={() => setRowProvider(i, "claude")}>claude</button>
              <button style={chip(row.provider === "codex")} onclick={() => setRowProvider(i, "codex")}>codex</button>
            </div>
            <select class="fb-select" bind:value={row.profile}>
              <option value="">no profile</option>
              {#each profileOptions(row.provider, row.profile) as p}
                <option value={p}>{p}</option>
              {/each}
            </select>
            <button class="fb-move" title="Move up" disabled={i === 0} onclick={() => moveRow(i, -1)} aria-label="Move up">↑</button>
            <button class="fb-move" title="Move down" disabled={i === edFallback.length - 1} onclick={() => moveRow(i, 1)} aria-label="Move down">↓</button>
            <button class="fb-x" title="Remove" onclick={() => removeRow(i)} aria-label="Remove">×</button>
          </div>
        {/each}
        <button class="fb-add" onclick={addRow}>+ Add fallback</button>

        <div class="ed-row">
          <span class="ed-key">permission</span>
          <div class="ed-chips">
            {#each ["approve", "yolo"] as p}
              <button style={chip(p === edPermission)} onclick={() => (edPermission = p)}>{p}</button>
            {/each}
          </div>
        </div>

        <div class="label-mono" style="margin:20px 0 8px">SOUL.md</div>
        <textarea class="field-area mono" rows="8" bind:value={edSoul} placeholder="# Name&#10;&#10;## Who you are…" style="font:400 12.5px/1.7 'JetBrains Mono',monospace;min-height:160px;white-space:pre"></textarea>

        <div style="display:flex;gap:9px;margin-top:22px">
          <button class="ed-cancel" onclick={() => (editOpen = false)}>Cancel</button>
          <button class="modal-cta" style="margin-top:0;flex:1" disabled={saving} onclick={save}>{saving ? "Saving…" : "Save changes"}</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Delete modal ===== -->
{#if deleteOpen && selected}
  <div class="modal-backdrop" role="presentation" onclick={() => (deleteOpen = false)}>
    <div class="modal-card delete-modal" role="dialog" aria-modal="true" aria-label="Delete agent" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="modal-head">
        <div class="modal-title">Delete {selected.Name}</div>
        <div class="modal-sub">This archives sessions into <span class="mono">~/.podium/agents/{selected.Name}/workspace/session-archive</span>, removes them from active history, and deletes the agent from Podium and config.yaml. Agent files are preserved.</div>
      </div>
      <div class="modal-body">
        {#if deleteError}<div class="error-banner" style="margin-bottom:14px">{deleteError}</div>{/if}
        <div class="label-mono" style="margin-bottom:8px">type agent name</div>
        <input class="field-input mono" bind:value={deleteName} placeholder={selected.Name} autocomplete="off" />
        <div style="display:flex;gap:9px;margin-top:22px">
          <button class="ed-cancel" onclick={() => (deleteOpen = false)}>Cancel</button>
          <button class="delete-confirm" disabled={deleting || deleteName.trim() !== selected.Name} onclick={confirmDelete}>{deleting ? "Deleting..." : "Delete agent"}</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .roster {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 330px), 1fr));
    gap: 16px;
  }

  .agent-card {
    background: var(--surface);
    border: 1px solid var(--line-2);
    border-radius: 20px;
    padding: 20px;
    cursor: pointer;
    box-shadow: 0 1px 2px rgba(43, 37, 32, 0.04), 0 16px 40px -26px rgba(43, 37, 32, 0.22);
    display: flex;
    gap: 15px;
    text-align: left;
  }

  .agent-card:hover {
    border-color: #d9cdbe;
  }

  .ac-body {
    flex: 1;
    min-width: 0;
  }

  .ac-head {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .ac-name {
    font: 800 18px "Hanken Grotesk";
  }

  .ac-specs {
    font: 400 11px "JetBrains Mono", monospace;
    color: var(--faint);
    margin-top: 9px;
  }

  .agent-add {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 7px;
    min-height: 140px;
    border: 1.5px dashed #decfbe;
    border-radius: 20px;
    color: #a8825e;
    font: 600 15px "Hanken Grotesk";
    cursor: pointer;
    background: rgba(255, 253, 251, 0.5);
  }

  .back-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: none;
    background: none;
    cursor: pointer;
    font: 600 13px "Hanken Grotesk";
    color: #a8825e;
    margin-bottom: 18px;
  }

  .ad-top {
    display: flex;
    gap: 20px;
    align-items: flex-start;
    max-width: 880px;
  }

  .ad-headrow {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .ad-name {
    font: 800 28px "Hanken Grotesk";
    letter-spacing: -0.02em;
  }

  .ad-soul {
    font: 400 15px/1.6 "Hanken Grotesk";
    color: var(--muted);
    margin-top: 8px;
    max-width: 560px;
    font-style: italic;
  }

  .ad-actions {
    display: flex;
    gap: 9px;
    margin-top: 16px;
  }

  .ad-edit {
    padding: 9px 16px;
    border: 1px solid var(--field-line);
    border-radius: 11px;
    background: #fff;
    color: var(--muted);
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .ad-delete {
    padding: 9px 16px;
    border: 1px solid #e7c3b5;
    border-radius: 11px;
    background: #fff;
    color: #a23e22;
    font: 600 13.5px "Hanken Grotesk";
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .ad-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin-top: 26px;
    max-width: 880px;
  }

  .ad-panel {
    background: var(--surface);
    border: 1px solid var(--line-2);
    border-radius: 18px;
    padding: 20px;
  }

  .ad-spec {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 9px 0;
    border-top: 1px solid #f1eae0;
    font: 400 13.5px "Hanken Grotesk";
    color: var(--muted);
  }

  .ad-spec span:last-child {
    font: 600 12.5px "JetBrains Mono", monospace;
    color: var(--ink);
  }

  /* edit modal */
  .ed-modal {
    width: 520px;
    max-width: 94vw;
  }

  .delete-modal {
    width: 480px;
    max-width: 94vw;
  }

  .ed-row {
    display: flex;
    align-items: center;
    gap: 9px;
    margin-top: 11px;
  }

  .ed-key {
    font: 500 11px "Hanken Grotesk";
    color: #9a8e80;
    width: 66px;
    flex: none;
  }

  .ed-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .fb-hint {
    font: 400 11.5px "Hanken Grotesk";
    color: #9a8e80;
    margin-bottom: 10px;
  }

  .fb-row {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 8px;
  }

  .fb-provs {
    display: flex;
    gap: 6px;
    flex: none;
  }

  .fb-select {
    flex: 1;
    min-width: 0;
    padding: 7px 10px;
    border: 1px solid var(--field-line);
    border-radius: 9px;
    background: #fff;
    color: var(--ink);
    font: 500 12px "JetBrains Mono", monospace;
    cursor: pointer;
  }

  .fb-move,
  .fb-x {
    flex: none;
    width: 30px;
    height: 30px;
    border: 1px solid var(--field-line);
    border-radius: 9px;
    background: #fff;
    color: var(--muted);
    font-size: 15px;
    line-height: 1;
    cursor: pointer;
  }

  .fb-x {
    border-color: #e7c3b5;
    color: #a23e22;
  }

  .fb-move:disabled {
    opacity: 0.35;
    cursor: not-allowed;
  }

  .fb-add {
    margin-top: 2px;
    padding: 8px 14px;
    border: 1.5px dashed #decfbe;
    border-radius: 10px;
    background: rgba(255, 253, 251, 0.5);
    color: #a8825e;
    font: 600 12.5px "Hanken Grotesk";
    cursor: pointer;
  }

  .ed-cancel {
    flex: none;
    padding: 13px 20px;
    border: 1px solid var(--field-line);
    border-radius: 13px;
    background: #fff;
    color: var(--muted);
    font: 600 14px "Hanken Grotesk";
    cursor: pointer;
  }

  .delete-confirm {
    flex: 1;
    padding: 13px 20px;
    border: none;
    border-radius: 13px;
    background: var(--orange);
    color: #fff;
    font: 700 14px "Hanken Grotesk";
    cursor: pointer;
    box-shadow: 0 10px 22px -8px rgba(217, 102, 61, 0.7);
  }

  .delete-confirm:disabled {
    cursor: not-allowed;
    opacity: 0.45;
    box-shadow: none;
  }

  @media (max-width: 768px) {
    .agents-head-row {
      align-items: stretch !important;
      flex-direction: column;
      gap: 12px !important;
    }

    .agents-head-row .spacer {
      display: none;
    }

    .agents-head-row .head-cta {
      width: 100%;
    }

    .agent-card {
      padding: 16px;
      gap: 12px;
    }

    .agent-add {
      min-height: 104px;
    }

    .ad-top {
      flex-direction: column;
      gap: 14px;
    }

    .ad-name {
      font-size: 24px;
    }

    .ad-actions {
      flex-direction: column;
      align-items: stretch;
    }

    .ad-actions button {
      justify-content: center;
      width: 100%;
    }

    .ad-grid {
      grid-template-columns: 1fr;
      margin-top: 20px;
    }

    .ad-spec {
      align-items: flex-start;
      flex-direction: column;
      gap: 4px;
    }

    .ad-spec span:last-child {
      max-width: 100%;
      overflow-wrap: anywhere;
    }

    .ed-row {
      align-items: stretch;
      flex-direction: column;
      gap: 7px;
    }

    .ed-key {
      width: auto;
    }
  }
</style>
