package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/app"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

// agentClient es el nombre con el que corre el agente evaluado. Cada caso
// levanta su propio watcher con una clave nueva, así que todo lo que el
// registro de auditoría atribuye a este cliente lo hizo el agente del caso.
const agentClient = "eval-agent"

// Env es lo que un runner necesita para hablar con el watcher del caso.
type Env struct {
	MCPURL string
	Key    string
}

// Output es lo que devuelve un runner.
type Output struct {
	FinalText  string         `json:"finalText,omitempty"`
	Answer     map[string]any `json:"answer"`
	Turns      int            `json:"turns,omitempty"`
	CostUSD    float64        `json:"costUsd,omitempty"`
	DurationMs int64          `json:"durationMs"`
	Error      string         `json:"error,omitempty"`
}

// Runner ejecuta un agente contra un caso.
type Runner interface {
	Name() string
	Run(ctx context.Context, c Case, t Trajectory, env Env) Output
}

// Run es una ejecución de un caso: la trayectoria real y el veredicto.
type Run struct {
	Case       string        `json:"case"`
	Variant    string        `json:"variant"`
	Runner     string        `json:"runner"`
	Scenario   string        `json:"scenario"`
	Passed     bool          `json:"passed"`
	Checks     []Result      `json:"checks"`
	Calls      []CallSummary `json:"calls"`
	Output     Output        `json:"output"`
	ExpectPass bool          `json:"expectPass"`
}

// CallSummary es una llamada tal y como la vio el servidor.
type CallSummary struct {
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args,omitempty"`
	Outcome   string         `json:"outcome"`
	ErrorCode string         `json:"errorCode,omitempty"`
}

// Correct dice si el resultado es el esperado: las trayectorias de
// referencia tienen que pasar y las adversarias tienen que fallar.
func (r Run) Correct() bool { return r.Passed == r.ExpectPass }

// execute levanta un watcher para el caso, ejecuta el runner y evalúa.
func execute(ctx context.Context, runner Runner, c Case, variant string, t Trajectory, expectPass bool) Run {
	key := randomKey()
	a, err := app.New(app.Config{
		DataSource: "fixture",
		Scenario:   c.Scenario,
		APIClients: fmt.Sprintf("%s:%s:%s", agentClient, strings.Join(c.Scopes, "+"), key),
		Logger:     zerolog.New(io.Discard),
	})
	run := Run{Case: c.ID, Variant: variant, Runner: runner.Name(), Scenario: c.Scenario, ExpectPass: expectPass}
	if err != nil {
		run.Output.Error = err.Error()
		return run
	}
	_ = a.Prime(ctx)
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	run.Output = runner.Run(ctx, c, t, Env{MCPURL: srv.URL + "/mcp", Key: key})

	events := a.Audit.Query(audit.Filter{Client: agentClient, Limit: 500})
	slices.Reverse(events)
	for _, e := range events {
		run.Calls = append(run.Calls, CallSummary{Tool: e.Operation, Args: e.Args, Outcome: e.Outcome, ErrorCode: e.ErrorCode})
	}
	run.Checks = Grade(c.Expect, run.Output.Answer, events)
	run.Passed = run.Output.Error == "" && Passed(run.Checks)
	return run
}

func randomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Runner guionizado: sin modelo, determinista. Reproduce una trayectoria
// escrita en el caso por la ruta MCP real. Sirve para probar el arnés y los
// evaluadores: la referencia tiene que pasar y las adversarias no.
// ---------------------------------------------------------------------------

type scriptedRunner struct{}

func (scriptedRunner) Name() string { return "scripted" }

func (scriptedRunner) Run(ctx context.Context, c Case, t Trajectory, env Env) Output {
	start := time.Now()
	out := Output{Answer: t.Answer}
	session, err := mcpserver.Connect(ctx, env.MCPURL, env.Key)
	if err != nil {
		out.Error = "connect: " + err.Error()
		return out
	}
	defer session.Close()
	for _, call := range t.Calls {
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.Tool, Arguments: call.Args}); err != nil {
			out.Error = fmt.Sprintf("%s: %v", call.Tool, err)
			return out
		}
	}
	out.DurationMs = time.Since(start).Milliseconds()
	return out
}

// ---------------------------------------------------------------------------
// Runner de Claude Code: un agente real, headless, que sólo ve las tools del
// watcher. Nada de Bash, ni lectura de ficheros, ni web: si resuelve la
// tarea es porque la interfaz se lo permite.
// ---------------------------------------------------------------------------

type claudeRunner struct {
	model   string
	timeout time.Duration
}

func (r claudeRunner) Name() string { return "claude-code" }

func (r claudeRunner) Run(ctx context.Context, c Case, _ Trajectory, env Env) Output {
	start := time.Now()
	out := Output{}
	dir, err := os.MkdirTemp("", "oci-watcher-eval-*")
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer os.RemoveAll(dir)

	cfg := map[string]any{"mcpServers": map[string]any{"oci-watcher": map[string]any{
		"type": "http", "url": env.MCPURL, "headers": map[string]string{"Authorization": "Bearer " + env.Key},
	}}}
	raw, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		out.Error = err.Error()
		return out
	}

	args := []string{
		"-p", c.Prompt,
		"--output-format", "json",
		"--mcp-config", cfgPath,
		"--strict-mcp-config",
		"--tools", "",
		"--allowedTools", "mcp__oci-watcher",
		"--append-system-prompt", c.Contract(),
	}
	if r.model != "" {
		args = append(args, "--model", r.model)
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", args...)
	// Un directorio vacío: el agente no debe leer el CLAUDE.md ni el
	// AGENTS.md de este repo. Lo que se evalúa es la interfaz, no la
	// documentación para quien programa en él.
	cmd.Dir = dir
	stdout, err := cmd.Output()
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			msg += ": " + strings.TrimSpace(string(ee.Stderr))
		}
		out.Error = "claude: " + msg
		return out
	}

	var res struct {
		Result  string  `json:"result"`
		IsError bool    `json:"is_error"`
		Turns   int     `json:"num_turns"`
		CostUSD float64 `json:"total_cost_usd"`
	}
	if err := json.Unmarshal(stdout, &res); err != nil {
		out.Error = "cannot parse claude output: " + err.Error()
		return out
	}
	out.FinalText, out.Turns, out.CostUSD = res.Result, res.Turns, res.CostUSD
	if res.IsError {
		out.Error = "claude reported an error: " + res.Result
		return out
	}
	answer, err := ExtractAnswer(res.Result)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Answer = answer
	return out
}
