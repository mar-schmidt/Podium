<script lang="ts">
  import { onMount } from "svelte";
  import { listSkills } from "../lib/api";
  import type { Skill, SkillSource } from "../lib/types";

  let skills = $state<Skill[]>([]);
  let loading = $state(true);
  let loadError = $state<string | null>(null);

  let query = $state("");
  let sourceFilter = $state<"all" | SkillSource>("all");
  let open = $state<Record<string, boolean>>({});

  onMount(async () => {
    try {
      skills = await listSkills();
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  });

  // Source styling — mirrors the design: shared=circle, claude=square, codex=diamond.
  const SRC: Record<SkillSource, { label: string; fg: string; bg: string; bd: string; glyph: string; path: string }> = {
    agents: { label: "shared", fg: "#2F6E60", bg: "#E7F0EC", bd: "#C7DBD2", glyph: "circle", path: "~/.agents/skills/" },
    claude: { label: "claude", fg: "#B0572F", bg: "#F8EBE2", bd: "#ECD3C2", glyph: "square", path: "~/.claude/skills/" },
    codex: { label: "codex", fg: "#4B5560", bg: "#EAEEF1", bd: "#D6DCE2", glyph: "diamond", path: "~/.codex/skills/" },
  };
  const ORDER: SkillSource[] = ["agents", "claude", "codex"];

  function dotStyle(src: SkillSource): string {
    const m = SRC[src];
    const shape =
      m.glyph === "circle" ? "border-radius:99px" : m.glyph === "square" ? "border-radius:2px" : "border-radius:1px;transform:rotate(45deg)";
    return `width:8px;height:8px;background:${m.fg};flex:none;${shape}`;
  }
  function chipStyle(src: SkillSource): string {
    const m = SRC[src];
    return `display:inline-flex;align-items:center;gap:6px;padding:4px 9px 4px 8px;border-radius:8px;font:600 11.5px 'JetBrains Mono',monospace;color:${m.fg};background:${m.bg};border:1px solid ${m.bd};white-space:nowrap`;
  }

  function toggle(name: string) {
    open = { ...open, [name]: !open[name] };
  }

  const counts = $derived.by(() => {
    const c = { all: skills.length, agents: 0, claude: 0, codex: 0 };
    for (const sk of skills) for (const s of sk.sources) c[s]++;
    return c;
  });

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return skills.filter((sk) => {
      if (sourceFilter !== "all" && !sk.sources.includes(sourceFilter)) return false;
      if (q && !(sk.name.toLowerCase().includes(q) || sk.description.toLowerCase().includes(q))) return false;
      return true;
    });
  });

  const flowSources = $derived(
    ORDER.map((key) => ({
      key,
      label: SRC[key].label,
      path: SRC[key].path,
      count: skills.filter((sk) => sk.sources.includes(key)).length,
    })),
  );

  const isEmpty = $derived(!loading && filtered.length === 0);
  const emptyTitle = $derived(
    query.trim()
      ? `No skills match “${query.trim()}”`
      : sourceFilter !== "all"
        ? `No ${SRC[sourceFilter as SkillSource].label} skills found`
        : "No skills yet",
  );

  const pills: ("all" | SkillSource)[] = ["all", "agents", "claude", "codex"];
  function pillLabel(p: "all" | SkillSource): string {
    return p === "all" ? "All" : SRC[p].label;
  }
  function pillStyle(p: "all" | SkillSource): string {
    const active = sourceFilter === p;
    return (
      "display:inline-flex;align-items:center;gap:6px;border:none;cursor:pointer;padding:7px 12px;border-radius:9px;font:600 12.5px 'Hanken Grotesk';" +
      (active ? "background:#FFFDFB;color:#2B2520;box-shadow:0 1px 3px rgba(43,37,32,.12)" : "background:transparent;color:#8A7F73")
    );
  }
</script>

<div class="skills-page">
  <div class="skills-inner">
    <!-- header -->
    <div style="display:flex;align-items:flex-start;gap:16px;flex-wrap:wrap">
      <div style="flex:1;min-width:240px">
        <div style="font:800 24px 'Hanken Grotesk';letter-spacing:-.02em">Skills</div>
        <div style="font:400 13px/1.55 'Hanken Grotesk';color:#8A7F73;margin-top:3px;max-width:560px">
          A read-only catalogue of the reusable <span style="font:500 12px 'JetBrains Mono',monospace;color:#8A7560">SKILL.md</span> files found on this
          machine. Nothing is configured here — the page just shows what exists and where it lives.
        </div>
      </div>
    </div>

    <!-- merge diagram -->
    <div
      style="margin-top:22px;background:#FFFDFB;border:1px solid #EDE4D9;border-radius:18px;box-shadow:0 1px 2px rgba(43,37,32,.04),0 18px 44px -32px rgba(43,37,32,.22);padding:18px 20px 16px"
    >
      <div style="display:flex;align-items:center;gap:9px;margin-bottom:16px">
        <span style="font:600 10px 'JetBrains Mono',monospace;letter-spacing:.13em;color:#A89C8E;text-transform:uppercase">How skills reach your agents</span>
        <span style="flex:1;height:1px;background:#F1EAE0"></span>
        <span style="font:400 12px 'Hanken Grotesk';color:#8A7F73">Wherever a skill lives, it joins one pool every agent can draw from.</span>
      </div>

      <div style="display:flex;align-items:stretch;gap:0;min-height:264px">
        <!-- three source lists -->
        <div style="width:236px;flex:none;display:flex;flex-direction:column;gap:12px">
          {#each flowSources as g (g.key)}
            <div
              style="flex:1;min-height:0;background:#FBF7F1;border:1px solid #EDE4D9;border-radius:13px;padding:13px 14px;display:flex;flex-direction:column;gap:9px;justify-content:center"
            >
              <div style="display:flex;align-items:center;gap:8px">
                <span style={chipStyle(g.key)}><span style={dotStyle(g.key)}></span>{g.label}</span>
                <span style="margin-left:auto;font:600 11px 'JetBrains Mono',monospace;color:#B7AC9E">{g.count}</span>
              </div>
              <span style="font:500 11.5px 'JetBrains Mono',monospace;color:#8A7F73;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">{g.path}</span>
            </div>
          {/each}
        </div>

        <!-- connectors -->
        <div style="width:92px;flex:none;position:relative">
          <svg width="92" height="264" viewBox="0 0 92 264" preserveAspectRatio="none" style="display:block">
            <path d="M0,42 C46,42 46,132 92,132" fill="none" stroke="#2F6E60" stroke-width="2" stroke-linecap="round" stroke-dasharray="2 7" opacity=".55" style="animation:pdFlow 1.1s linear infinite" />
            <path d="M0,132 L92,132" fill="none" stroke="#B0572F" stroke-width="2" stroke-linecap="round" stroke-dasharray="2 7" opacity=".55" style="animation:pdFlow 1.1s linear infinite" />
            <path d="M0,222 C46,222 46,132 92,132" fill="none" stroke="#4B5560" stroke-width="2" stroke-linecap="round" stroke-dasharray="2 7" opacity=".55" style="animation:pdFlow 1.1s linear infinite" />
            <polygon points="80,124 92,132 80,140" fill="#C7B49B" />
          </svg>
        </div>

        <!-- shared podium pool -->
        <div
          style="flex:1;min-width:0;background:radial-gradient(120% 130% at 100% 0%,rgba(70,160,140,.10),transparent 55%),#FFFDFB;border:1px solid #DCD0C1;border-radius:14px;padding:15px 17px;display:flex;flex-direction:column;box-shadow:0 14px 36px -28px rgba(47,110,96,.5)"
        >
          <div style="display:flex;align-items:center;gap:10px;margin-bottom:13px">
            <div style="width:28px;height:28px;border-radius:9px;background:linear-gradient(150deg,#46A08C,#2F6E60);display:flex;align-items:center;justify-content:center;box-shadow:0 6px 14px -6px rgba(47,110,96,.6);flex:none">
              <span style="width:9px;height:9px;background:#fff;border-radius:2px;transform:rotate(45deg)"></span>
            </div>
            <div style="flex:1;min-width:0">
              <div style="font:800 15px 'Hanken Grotesk';letter-spacing:-.01em;line-height:1.1">Podium pool</div>
              <div style="font:400 11px 'Hanken Grotesk';color:#8A7F73">one library · every agent · every chat</div>
            </div>
            <span style="display:inline-flex;align-items:center;gap:5px;padding:4px 10px;border-radius:8px;background:#EAF2EE;border:1px solid #CFE2DA;font:600 11px 'JetBrains Mono',monospace;color:#2F6E60">{skills.length} skills</span>
          </div>
          <div style="display:flex;flex-wrap:wrap;align-content:flex-start;gap:7px;flex:1;min-height:0;overflow:hidden">
            {#each skills as p (p.name)}
              <span style="display:inline-flex;align-items:center;gap:7px;padding:5px 10px;border-radius:8px;background:#FBF7F1;border:1px solid #EFE6DB;font:600 11.5px 'JetBrains Mono',monospace;color:#4A4138">
                {p.name}
                <span style="display:flex;align-items:center;gap:3px">
                  {#each p.sources as d (d)}<span style={dotStyle(d)}></span>{/each}
                </span>
              </span>
            {/each}
          </div>
        </div>
      </div>
    </div>

    <!-- toolbar -->
    <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-top:22px">
      <div style="position:relative;flex:1;min-width:240px;max-width:420px">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#A89C8E" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="position:absolute;left:13px;top:50%;transform:translateY(-50%);pointer-events:none"><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></svg>
        <input
          bind:value={query}
          placeholder="Search skills by name or description…"
          style="width:100%;padding:11px 14px 11px 38px;border:1px solid #EAE0D4;border-radius:12px;background:#FFFDFB;font:500 13.5px 'Hanken Grotesk';color:#2B2520;outline:none;box-shadow:0 1px 2px rgba(43,37,32,.03)"
        />
      </div>
      <div style="display:flex;align-items:center;gap:4px;padding:4px;border-radius:12px;background:#EFE7DC;border:1px solid #E6DBCC">
        {#each pills as p (p)}
          <button onclick={() => (sourceFilter = p)} style={pillStyle(p)}>
            {#if p !== "all"}<span style={dotStyle(p)}></span>{/if}
            {pillLabel(p)}
            <span style={`font:600 11px 'JetBrains Mono',monospace;${sourceFilter === p ? "color:#A89C8E" : "color:#B7AC9E"}`}>{counts[p]}</span>
          </button>
        {/each}
      </div>
    </div>

    <!-- list -->
    <div style="display:flex;flex-direction:column;gap:11px;margin-top:18px">
      {#each filtered as s (s.name)}
        <div style="background:#FFFDFB;border:1px solid #EDE4D9;border-radius:16px;box-shadow:0 1px 2px rgba(43,37,32,.04),0 14px 38px -30px rgba(43,37,32,.22);overflow:hidden">
          <!-- row header -->
          <div
            role="button"
            tabindex="0"
            onclick={() => toggle(s.name)}
            onkeydown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), toggle(s.name))}
            style={`display:flex;align-items:flex-start;gap:13px;padding:16px 20px;cursor:pointer;${open[s.name] ? "background:#FCF8F2" : "background:transparent"}`}
          >
            <span style={`flex:none;display:flex;align-items:center;justify-content:center;width:22px;height:22px;margin-top:1px;transition:transform .16s ease;transform:rotate(${open[s.name] ? "90deg" : "0deg"})`}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#B7AC9E" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m9 6 6 6-6 6" /></svg>
            </span>
            <div style="flex:1;min-width:0">
              <div style="display:flex;align-items:center;gap:9px;flex-wrap:wrap">
                <span style="font:700 15.5px 'JetBrains Mono',monospace;color:#241F1A;letter-spacing:-.01em">{s.name}</span>
                {#if s.conflict}
                  <span style="display:inline-flex;align-items:center;gap:5px;padding:3px 8px 3px 7px;border-radius:7px;font:600 10.5px 'JetBrains Mono',monospace;color:#9A6B1A;background:#FBF1DD;border:1px solid #ECD8A6">
                    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="#C0922E" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3 2 20h20L12 3z" /><path d="M12 10v4" /><path d="M12 17.5v.2" /></svg>
                    versions differ
                  </span>
                {/if}
              </div>
              <div style="font:400 13px/1.5 'Hanken Grotesk';color:#6F6459;margin-top:5px;max-width:620px">{s.description}</div>
            </div>
            <div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap;justify-content:flex-end;max-width:230px">
              {#each s.sources as b (b)}
                <span style={chipStyle(b)}><span style={dotStyle(b)}></span>{SRC[b].label}</span>
              {/each}
            </div>
          </div>

          <!-- expanded -->
          {#if open[s.name]}
            <div style="border-top:1px solid #F1EAE0;padding:18px 22px 20px;animation:pdExpand .16s ease">
              <div style="font:600 10px 'JetBrains Mono',monospace;letter-spacing:.12em;color:#A89C8E;text-transform:uppercase;margin-bottom:11px">Where it lives</div>
              <div style="display:flex;flex-direction:column;gap:7px;margin-bottom:18px">
                {#each s.locations as loc (loc.source)}
                  <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">
                    <span style={chipStyle(loc.source)}><span style={dotStyle(loc.source)}></span>{SRC[loc.source].label}</span>
                    <span style="font:500 12.5px 'JetBrains Mono',monospace;color:#7A6F62">{loc.path}</span>
                  </div>
                {/each}
              </div>

              {#each s.contents as c, i (c.source || i)}
                <div style="margin-top:14px">
                  <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
                    <span style="font:600 10px 'JetBrains Mono',monospace;letter-spacing:.12em;color:#A89C8E;text-transform:uppercase">SKILL.md</span>
                    {#if c.source}<span style={chipStyle(c.source)}><span style={dotStyle(c.source)}></span>{SRC[c.source].label}</span>{/if}
                    <span style="flex:1;height:1px;background:#F1EAE0"></span>
                    <span style="font:500 10px 'JetBrains Mono',monospace;color:#C4B8A8">read-only</span>
                  </div>
                  <pre style="margin:0;background:#FBF7F1;border:1px solid #EFE6DB;border-radius:12px;padding:15px 17px;font:500 12px/1.7 'JetBrains Mono',monospace;color:#4A4138;white-space:pre-wrap;word-break:break-word">{c.body}</pre>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}

      {#if loadError}
        <div style="padding:16px 20px;border:1px solid #ECD3C2;background:#F8EBE2;border-radius:14px;color:#B0572F;font:500 13px 'Hanken Grotesk'">
          Could not load skills: {loadError}
        </div>
      {/if}

      <!-- empty state -->
      {#if isEmpty && !loadError}
        <div style="display:flex;flex-direction:column;align-items:center;text-align:center;gap:14px;padding:54px 24px;border:1.5px dashed #E0D3C3;border-radius:18px;background:rgba(255,253,251,.5)">
          <div style="width:54px;height:54px;border-radius:16px;background:#EFE7DC;display:flex;align-items:center;justify-content:center">
            <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="#A8967F" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 8a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>
          </div>
          <div style="font:700 16px 'Hanken Grotesk';color:#3A332C">{emptyTitle}</div>
          <div style="font:400 13.5px/1.65 'Hanken Grotesk';color:#7A6F62;max-width:440px">
            Skills live in <span style="font:600 12.5px 'JetBrains Mono',monospace;color:#2F6E60">~/.agents/skills/</span>. Add a
            <span style="font:600 12.5px 'JetBrains Mono',monospace;color:#8A7560">SKILL.md</span> folder there to make it available to all agents.
          </div>
        </div>
      {/if}
    </div>

    <!-- persistent help row -->
    <div style="display:flex;align-items:center;gap:12px;margin:22px 0 28px;padding:13px 16px;border-radius:13px;background:#FAF6F0;border:1px solid #EFE6DB">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#A8967F" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" style="flex:none"><path d="M3 8a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>
      <div style="font:400 12.5px/1.5 'Hanken Grotesk';color:#7A6F62">
        Skills live in <span style="font:600 12px 'JetBrains Mono',monospace;color:#2F6E60">~/.agents/skills/</span> — drop a
        <span style="font:600 12px 'JetBrains Mono',monospace;color:#8A7560">SKILL.md</span> folder there and every agent can use it. Podium reads them, it never installs
        or edits them.
      </div>
    </div>
  </div>
</div>

<style>
  .skills-page {
    flex: 1;
    overflow-y: auto;
    padding: 24px 28px 0;
    min-height: 0;
    background: #f4ece2;
    color: #2b2520;
  }
  .skills-inner {
    max-width: 1080px;
    margin: 0 auto;
  }
  @keyframes pdExpand {
    0% {
      opacity: 0;
      transform: translateY(-4px);
    }
    100% {
      opacity: 1;
      transform: none;
    }
  }
  @keyframes pdFlow {
    to {
      stroke-dashoffset: -18;
    }
  }
</style>
