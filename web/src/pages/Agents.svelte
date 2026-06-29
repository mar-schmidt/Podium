<script lang="ts">
  import { getAgent, updateAgent } from "../lib/api";
  import { agentGradient, avatarStyle, initial, modeChip, providerChip } from "../lib/theme";
  import type { Agent } from "../lib/types";

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
  let saving = $state(false);
  let editError = $state<string | null>(null);

  const EFFORTS = ["low", "medium", "high", "xhigh", "max"];
  const modelOptions = $derived(
    edProvider === "codex" ? ["gpt-5.1", "gpt-5.1-mini", "o4"] : ["sonnet", "opus", "haiku"],
  );

  function specs(a: Agent): string {
    return `${a.Model || a.Provider} · ${a.Effort || "medium"} · profile: ${a.Profile || "default"}`;
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
    editOpen = true;
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
      };
      editOpen = false;
      onChanged();
    } catch (e) {
      editError = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
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
      <div style="display:flex;align-items:flex-end;gap:14px">
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

<style>
  .roster {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(330px, 1fr));
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
</style>
