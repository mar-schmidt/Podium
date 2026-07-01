<script lang="ts">
  // A simple Cancel/Delete confirmation modal, reused by the session, roadmap
  // task, and schedule delete flows. Reuses the shared modal chrome from app.css.
  interface Props {
    title: string;
    message: string;
    confirmLabel?: string;
    busy?: boolean;
    error?: string | null;
    onConfirm: () => void;
    onCancel: () => void;
  }
  let {
    title,
    message,
    confirmLabel = "Delete",
    busy = false,
    error = null,
    onConfirm,
    onCancel,
  }: Props = $props();
</script>

<div class="modal-backdrop" role="presentation" onclick={onCancel}>
  <div
    class="modal-card confirm-modal"
    role="dialog"
    aria-modal="true"
    aria-label={title}
    tabindex="-1"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
  >
    <div class="modal-head">
      <div class="modal-title">{title}</div>
      <div class="modal-sub">{message}</div>
    </div>
    <div class="modal-body">
      {#if error}<div class="error-banner" style="margin-bottom:14px">{error}</div>{/if}
      <div style="display:flex;gap:9px;margin-top:6px">
        <button class="cm-cancel" onclick={onCancel} disabled={busy}>Cancel</button>
        <button class="cm-confirm" onclick={onConfirm} disabled={busy}>
          {busy ? "Working…" : confirmLabel}
        </button>
      </div>
    </div>
  </div>
</div>

<style>
  .confirm-modal {
    width: 440px;
    max-width: 94vw;
  }

  .cm-cancel {
    flex: none;
    padding: 13px 20px;
    border: 1px solid var(--field-line);
    border-radius: 13px;
    background: #fff;
    color: var(--muted);
    font: 600 14px "Hanken Grotesk";
    cursor: pointer;
  }

  .cm-cancel:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .cm-confirm {
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

  .cm-confirm:disabled {
    cursor: not-allowed;
    opacity: 0.45;
    box-shadow: none;
  }
</style>
