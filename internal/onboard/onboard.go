// Package onboard implements Podium's first-run CLI wizard.
package onboard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/client"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/providercheck"
	"github.com/mar-schmidt/Podium/internal/store"
)

// Options configures the interactive onboarding wizard.
type Options struct {
	Addr string
	In   io.Reader
	Out  io.Writer
	Err  io.Writer
}

type answers struct {
	Role          string
	Temperament   string
	Collaboration string
	Autonomy      string
	Strengths     string
	Boundaries    string
	Playfulness   string
	CaresAbout    string
	Extra         string
}

// Run launches the first-run wizard.
func Run(ctx context.Context, opts Options) error {
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	// When input isn't an interactive terminal — for example when the wizard is
	// launched from `curl | bash`, where stdin is the installer's script pipe —
	// attach to the controlling terminal so keystrokes register and Ctrl-C works.
	// Reassigning os.Stdin means nested flows (provider login, $EDITOR) inherit
	// the terminal too, not just our prompts.
	if tty := controllingTTY(in); tty != nil {
		defer tty.Close()
		in = tty
		os.Stdin = tty
	}
	p := prompter{in: bufio.NewReader(in), out: out}
	fmt.Fprintln(out, "Welcome to Podium.")
	fmt.Fprintln(out, "Let's wake the stage, check your agent CLIs, and shape your first agent together.")
	fmt.Fprintln(out)

	statuses := providercheck.CheckAll(ctx, providercheck.Options{})
	printDoctor(out, statuses)
	ready := readyProviders(statuses)
	for len(ready) == 0 {
		fmt.Fprintln(out, "Podium needs Claude or Codex before it can generate a SOUL.md.")
		for _, s := range statuses {
			if !s.Found {
				fmt.Fprintf(out, "\n%s not found.\n  %s\n", titleProvider(s.Provider), s.InstallHint)
				continue
			}
			if p.confirm(fmt.Sprintf("Open the native %s login now?", titleProvider(s.Provider)), true) {
				_ = providercheck.RunNativeLogin(ctx, s.Provider, s.Path)
			}
		}
		statuses = providercheck.CheckAll(ctx, providercheck.Options{})
		printDoctor(out, statuses)
		ready = readyProviders(statuses)
		if len(ready) == 0 && !p.confirm("Try provider checks again?", true) {
			return errors.New("no working provider available; run `podium doctor` after installing or logging in")
		}
	}

	provider := chooseProvider(p, ready)
	if err := confirmProviderLogin(ctx, p, provider, statuses); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nGreat. %s will help draft this agent's SOUL.md.\n", titleProvider(provider))

	c, addr, err := ensureDaemon(ctx, opts.Addr, out, errOut)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Podium daemon is live at %s.\n", addr)

	ans := collectAnswers(p)
	name := chooseName(p, ans)
	permission := config.PermissionApprove
	model := ""
	effort := "medium"
	agent, err := c.CreateAgent(ctx, client.AgentCreateRequest{
		Name:           name,
		Provider:       provider,
		Model:          model,
		Effort:         effort,
		PermissionMode: permission,
	})
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	placeholder := deterministicSoul(name, ans, true)
	if _, err := c.UpdateAgent(ctx, name, client.AgentUpdateRequest{Soul: &placeholder}); err != nil {
		return fmt.Errorf("write placeholder SOUL.md: %w", err)
	}
	fmt.Fprintf(out, "\n%s exists. Now we'll ask %s to write the first SOUL.md draft.\n", agent.Name, titleProvider(provider))

	soul, err := generateSoul(ctx, c, name, ans, out)
	if err != nil {
		fmt.Fprintf(out, "\nLLM generation did not complete: %v\n", err)
		soul = deterministicSoul(name, ans, false)
	}
	for {
		fmt.Fprintln(out, "\n--- SOUL.md preview ---")
		fmt.Fprintln(out, soul)
		fmt.Fprintln(out, "--- end preview ---")
		switch p.choice("What should we do with this soul?", []string{"accept", "regenerate", "edit"}, "accept") {
		case "accept":
			if _, err := c.UpdateAgent(ctx, name, client.AgentUpdateRequest{Soul: &soul}); err != nil {
				return fmt.Errorf("save SOUL.md: %w", err)
			}
			fmt.Fprintf(out, "\n%s is ready. Try: podium chat --agent %s \"hello\"\n", name, name)
			return nil
		case "regenerate":
			next, err := generateSoul(ctx, c, name, ans, out)
			if err != nil {
				fmt.Fprintf(out, "Regeneration failed: %v\n", err)
				continue
			}
			soul = next
		case "edit":
			edited, err := editText(soul)
			if err != nil {
				fmt.Fprintf(out, "Editor unavailable: %v\n", err)
				edited = p.multiline("Paste the SOUL.md you want to save. End with a single dot on its own line.")
			}
			if strings.TrimSpace(edited) != "" {
				soul = CleanSoulMarkdown(edited)
			}
		}
	}
}

func ensureDaemon(ctx context.Context, addr string, out, errOut io.Writer) (*client.Client, string, error) {
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	c := client.New(addr)
	if _, err := c.Health(ctx); err == nil {
		return c, addr, nil
	}
	fmt.Fprintln(out, "Starting podiumd for onboarding...")
	podiumd, err := findPodiumd()
	if err != nil {
		return nil, addr, err
	}
	cmd := exec.CommandContext(ctx, podiumd)
	cmd.Stdout = errOut
	cmd.Stderr = errOut
	if err := cmd.Start(); err != nil {
		return nil, addr, fmt.Errorf("start podiumd: %w", err)
	}
	for i := 0; i < 30; i++ {
		time.Sleep(300 * time.Millisecond)
		if _, err := c.Health(ctx); err == nil {
			return c, addr, nil
		}
	}
	return nil, addr, errors.New("podiumd did not become ready")
}

func findPodiumd() (string, error) {
	if p, err := exec.LookPath("podiumd"); err == nil {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "podiumd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidate := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("podiumd not found on PATH or next to %s", exe)
}

func printDoctor(out io.Writer, statuses []providercheck.Status) {
	fmt.Fprintln(out, "Provider check:")
	for _, s := range statuses {
		state := "missing"
		if s.Ready {
			state = "ready"
		} else if s.Found {
			state = "found"
		}
		fmt.Fprintf(out, "  %s: %s", titleProvider(s.Provider), state)
		if s.Version != "" {
			fmt.Fprintf(out, " (%s)", s.Version)
		}
		if s.Path != "" {
			fmt.Fprintf(out, " at %s", s.Path)
		}
		fmt.Fprintln(out)
		if !s.Ready && s.Error != "" {
			fmt.Fprintf(out, "    %s\n", s.Error)
		}
	}
}

func readyProviders(statuses []providercheck.Status) []config.Provider {
	var out []config.Provider
	for _, s := range statuses {
		if s.Ready {
			out = append(out, s.Provider)
		}
	}
	return out
}

func chooseProvider(p prompter, providers []config.Provider) config.Provider {
	if len(providers) == 1 {
		return providers[0]
	}
	labels := make([]string, 0, len(providers))
	for _, provider := range providers {
		labels = append(labels, string(provider))
	}
	choice := p.choice("Which provider do you want to start using?", labels, labels[0])
	return config.Provider(choice)
}

func confirmProviderLogin(ctx context.Context, p prompter, provider config.Provider, statuses []providercheck.Status) error {
	var status providercheck.Status
	for _, candidate := range statuses {
		if candidate.Provider == provider {
			status = candidate
			break
		}
	}
	if status.Path == "" {
		return fmt.Errorf("%s is not available", titleProvider(provider))
	}
	if p.confirm(fmt.Sprintf("Is %s logged in and ready to run?", titleProvider(provider)), true) {
		return nil
	}
	fmt.Fprintf(p.out, "Opening %s's native login flow.\n", titleProvider(provider))
	if err := providercheck.RunNativeLogin(ctx, provider, status.Path); err != nil {
		return err
	}
	next := providercheck.Check(ctx, provider, providercheck.Options{})
	if !next.Ready {
		return fmt.Errorf("%s still does not look ready: %s", titleProvider(provider), next.Error)
	}
	return nil
}

func collectAnswers(p prompter) answers {
	fmt.Fprintln(p.out, "\nNow for the fun part: this agent should feel like someone you influenced.")
	return answers{
		Role:          p.choice("What should this agent gravitate toward?", []string{"builder", "researcher", "operator", "creative partner", "reviewer"}, "builder"),
		Temperament:   p.choice("What temperament should it have?", []string{"calm and precise", "warm and curious", "bold and proactive", "playful and inventive"}, "warm and curious"),
		Collaboration: p.choice("How should it collaborate with you?", []string{"ask before big moves", "make reasonable calls", "challenge me thoughtfully", "keep momentum high"}, "make reasonable calls"),
		Autonomy:      p.choice("How much autonomy should it take?", []string{"low", "medium", "high"}, "medium"),
		Strengths:     p.ask("What strengths should it lean into?", "systems thinking, clear writing, careful implementation"),
		Boundaries:    p.ask("What boundaries should it respect?", "avoid destructive changes, explain risky choices, keep user data private"),
		Playfulness:   p.choice("How much playfulness belongs in its voice?", []string{"subtle", "moderate", "sparkly"}, "moderate"),
		CaresAbout:    p.ask("What should this agent care about most?", "helping me finish meaningful work with less friction"),
		Extra:         p.ask("Anything else you want woven into the soul?", ""),
	}
}

func chooseName(p prompter, ans answers) string {
	suggestions := suggestNames(ans)
	return sanitizeName(p.choice("Choose a name, or type your own", suggestions, suggestions[0]))
}

func suggestNames(ans answers) []string {
	switch ans.Role {
	case "researcher":
		return []string{"atlas", "mira", "sage", "lumen"}
	case "operator":
		return []string{"riley", "switch", "marin", "pilot"}
	case "creative partner":
		return []string{"sol", "nova", "fig", "muse"}
	case "reviewer":
		return []string{"quinn", "arden", "scope", "vera"}
	default:
		return []string{"juno", "forge", "ember", "rowan"}
	}
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "juno"
	}
	return out
}

func generateSoul(ctx context.Context, c *client.Client, name string, ans answers, out io.Writer) (string, error) {
	session, err := c.CreateSession(ctx, client.SessionCreateRequest{AgentName: name, Origin: store.OriginOnboarding})
	if err != nil {
		return "", err
	}
	events, errs := c.Chat(ctx, client.ChatRequest{SessionID: session.ID, Message: SoulPrompt(name, ans)})
	var b strings.Builder
	gotDelta := false
	for event := range events {
		switch event.Type {
		case "delta":
			gotDelta = true
			b.WriteString(event.Delta)
			fmt.Fprint(out, ".")
		case "assistant":
			if !gotDelta {
				b.WriteString(event.Delta)
			}
		case "permission_request":
			if event.Request != nil {
				_ = c.DecidePermission(ctx, event.Request.ID, adapter.PermissionDecision{
					Behavior: "deny",
					Message:  "SOUL.md generation does not need tools",
				})
			}
		case "error":
			return "", errors.New(event.Error)
		}
	}
	if err := <-errs; err != nil {
		return "", err
	}
	fmt.Fprintln(out)
	soul := CleanSoulMarkdown(b.String())
	if strings.TrimSpace(soul) == "" {
		return "", errors.New("provider returned an empty SOUL.md draft")
	}
	return soul, nil
}

// SoulPrompt builds the LLM request for a first-agent SOUL.md.
func SoulPrompt(name string, ans answers) string {
	return fmt.Sprintf(`You are helping a user create a newborn Podium agent.

Write ONLY the contents of a markdown file named SOUL.md. Do not wrap it in code fences.
The agent name is %q.

User-shaped questionnaire:
- Role: %s
- Temperament: %s
- Collaboration style: %s
- Autonomy: %s
- Strengths: %s
- Boundaries: %s
- Playfulness: %s
- Cares about: %s
- Extra notes: %s

Use exactly these markdown sections:
# Identity

Name: %s

One short paragraph describing who this agent is and why it exists.

## Working style

- 3 to 5 bullets

## Strengths

- 3 to 5 bullets

## Boundaries

- 3 to 5 bullets

## Voice

- 2 to 4 bullets

Make the agent feel alive, specific, and influenced by the user's choices. Keep it practical for a coding/research assistant.`, name, ans.Role, ans.Temperament, ans.Collaboration, ans.Autonomy, ans.Strengths, ans.Boundaries, ans.Playfulness, ans.CaresAbout, ans.Extra, name)
}

// CleanSoulMarkdown removes common chat wrapping from generated markdown.
func CleanSoulMarkdown(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(s, "# Identity") {
		s = "# Identity\n\n" + s
	}
	return strings.TrimSpace(s) + "\n"
}

func deterministicSoul(name string, ans answers, placeholder bool) string {
	note := ""
	if placeholder {
		note = "\nThis is a temporary soul while Podium asks the selected provider for a richer first draft.\n"
	} else {
		note = "\nThis fallback was generated from the questionnaire because the LLM draft was unavailable.\n"
	}
	return CleanSoulMarkdown(fmt.Sprintf(`# Identity

Name: %s
%s
%s is a %s agent with a %s temperament. It exists to help the user move with clarity while preserving the choices that shaped it.

## Working style

- Collaboration style: %s.
- Autonomy level: %s.
- Cares most about: %s.

## Strengths

- %s.

## Boundaries

- %s.

## Voice

- Playfulness: %s.
- Extra notes: %s.
`, name, note, name, ans.Role, ans.Temperament, ans.Collaboration, ans.Autonomy, ans.CaresAbout, ans.Strengths, ans.Boundaries, ans.Playfulness, firstNonEmpty(ans.Extra, "stay useful, kind, and direct")))
}

func editText(initial string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return "", errors.New("EDITOR is not set")
	}
	tmp, err := os.CreateTemp("", "podium-soul-*.md")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(initial); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// controllingTTY returns the process's controlling terminal when in is not an
// interactive terminal (for example, when onboarding is launched from
// `curl | bash`, where stdin is the installer's script pipe). It returns nil
// when in is already a terminal or no controlling terminal is available (such
// as CI), so the caller keeps the original reader and prompts fall back to
// their defaults rather than hanging.
func controllingTTY(in io.Reader) *os.File {
	if f, ok := in.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return nil
		}
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return tty
}

func titleProvider(provider config.Provider) string {
	switch provider {
	case config.ProviderClaude:
		return "Claude"
	case config.ProviderCodex:
		return "Codex"
	default:
		return string(provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p prompter) ask(prompt, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", prompt, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", prompt)
	}
	line, _ := p.in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func (p prompter) confirm(prompt string, def bool) bool {
	defText := "Y/n"
	if !def {
		defText = "y/N"
	}
	for {
		answer := strings.ToLower(p.ask(prompt+" "+defText, ""))
		if answer == "" {
			return def
		}
		switch answer {
		case "y", "yes", "j", "ja":
			return true
		case "n", "no", "nej":
			return false
		}
	}
}

func (p prompter) choice(prompt string, options []string, def string) string {
	for {
		fmt.Fprintf(p.out, "%s\n", prompt)
		for i, opt := range options {
			fmt.Fprintf(p.out, "  %d. %s\n", i+1, opt)
		}
		answer := p.ask("Choose a number or type your own", def)
		if idx, err := strconv.Atoi(answer); err == nil && idx >= 1 && idx <= len(options) {
			return options[idx-1]
		}
		if strings.TrimSpace(answer) != "" {
			return answer
		}
	}
}

func (p prompter) multiline(prompt string) string {
	fmt.Fprintln(p.out, prompt)
	var lines []string
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		lines = append(lines, line)
		if err != nil {
			break
		}
	}
	return strings.Join(lines, "\n")
}
