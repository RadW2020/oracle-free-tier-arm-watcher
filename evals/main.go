// Comando evals ejecuta la batería de evals de la interfaz para agentes.
//
//	go run ./evals                          # runner guionizado (determinista, sin modelo)
//	go run ./evals -runner claude-code      # Claude Code real contra cada escenario
//	go run ./evals -runner claude-code -case incident -model sonnet
//
// Cada caso levanta su propio watcher en memoria con un escenario de
// fixtures y una clave nueva, ejecuta el agente y evalúa la respuesta y la
// trayectoria que registró el servidor. El informe JSON queda en
// evals/results/.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	runnerName := flag.String("runner", "scripted", "scripted | claude-code")
	filter := flag.String("case", "", "only run cases whose id contains this text")
	model := flag.String("model", "", "model for the claude-code runner (default: Claude Code's default)")
	timeout := flag.Duration("timeout", 5*time.Minute, "per-case timeout for real agents")
	casesDir := flag.String("cases", "evals/cases", "directory with the case files")
	outDir := flag.String("out", "evals/results", "directory for the JSON report ('' to skip)")
	flag.Parse()

	cases, err := LoadCases(*casesDir, *filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "no cases matched")
		os.Exit(2)
	}

	var runner Runner
	switch *runnerName {
	case "scripted":
		runner = scriptedRunner{}
	case "claude-code":
		runner = claudeRunner{model: *model, timeout: *timeout}
	default:
		fmt.Fprintf(os.Stderr, "unknown runner %q\n", *runnerName)
		os.Exit(2)
	}

	ctx := context.Background()
	var runs []Run
	for _, c := range cases {
		if runner.Name() == "scripted" {
			runs = append(runs, execute(ctx, runner, c, "reference", c.Reference, true))
			for _, adv := range c.Adversarial {
				runs = append(runs, execute(ctx, runner, c, "adversarial:"+adv.Name, adv.Trajectory, false))
			}
		} else {
			fmt.Fprintf(os.Stderr, "running %s ...\n", c.ID)
			runs = append(runs, execute(ctx, runner, c, "agent", Trajectory{}, true))
		}
	}

	failed := printReport(runner.Name(), runs)
	if *outDir != "" {
		if err := writeReport(*outDir, runner.Name(), runs); err != nil {
			fmt.Fprintln(os.Stderr, "report:", err)
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func printReport(runner string, runs []Run) int {
	failed := 0
	fmt.Printf("\n%-24s %-34s %-8s %s\n", "CASE", "VARIANT", "RESULT", "DETAIL")
	for _, r := range runs {
		// Una adversaria bien rechazada es CAUGHT, no PASS: el informe tiene
		// que leerse sin saber cómo funciona el arnés.
		status := "PASS"
		switch {
		case !r.ExpectPass && r.Correct():
			status = "CAUGHT"
		case !r.ExpectPass:
			status = "MISSED"
			failed++
		case !r.Correct():
			status = "FAIL"
			failed++
		}
		detail := ""
		switch {
		case r.Output.Error != "":
			detail = r.Output.Error
		case !r.ExpectPass && !r.Passed:
			detail = "rejected by: " + firstFailure(r.Checks)
		case !r.Passed:
			detail = firstFailure(r.Checks)
		case !r.ExpectPass && r.Passed:
			detail = "an adversarial trajectory PASSED: the graders are not testing this case"
		default:
			detail = fmt.Sprintf("%d checks, %d calls", len(r.Checks), len(r.Calls))
			if r.Output.CostUSD > 0 {
				detail += fmt.Sprintf(", %d turns, $%.3f", r.Output.Turns, r.Output.CostUSD)
			}
		}
		fmt.Printf("%-24s %-34s %-8s %s\n", r.Case, truncate(r.Variant, 34), status, truncate(detail, 110))
	}
	fmt.Printf("\n%s runner: %d/%d correct\n", runner, len(runs)-failed, len(runs))
	return failed
}

func firstFailure(rs []Result) string {
	for _, r := range rs {
		if !r.Passed {
			return r.Check + " (" + r.Detail + ")"
		}
	}
	return ""
}

func writeReport(dir, runner string, runs []Run) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%s.json", time.Now().UTC().Format("20060102T150405Z"), runner)
	raw, err := json.MarshalIndent(map[string]any{"runner": runner, "generatedAt": time.Now().UTC(), "runs": runs}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	fmt.Println("report:", path)
	return os.WriteFile(path, raw, 0o644)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
