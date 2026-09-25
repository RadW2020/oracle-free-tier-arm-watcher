package main

import (
	"context"
	"testing"
)

// TestScriptedEvals corre en `go test ./...` y en CI. No evalúa a ningún
// modelo: prueba que cada caso distingue lo bueno de lo malo. La trayectoria
// de referencia tiene que pasar y cada trayectoria adversaria tiene que
// fallar; si una adversaria pasa, el caso no detectaría la regresión que
// dice detectar.
func TestScriptedEvals(t *testing.T) {
	cases, err := LoadCases("cases", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("cases = %d; want at least 10", len(cases))
	}
	ctx := context.Background()
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			if len(c.Adversarial) == 0 {
				t.Error("every case needs at least one adversarial trajectory")
			}
			ref := execute(ctx, scriptedRunner{}, c, "reference", c.Reference, true)
			if !ref.Correct() {
				t.Errorf("reference failed: %s %s", ref.Output.Error, firstFailure(ref.Checks))
			}
			for _, adv := range c.Adversarial {
				run := execute(ctx, scriptedRunner{}, c, adv.Name, adv.Trajectory, false)
				if run.Output.Error != "" {
					t.Errorf("adversarial %s errored: %s", adv.Name, run.Output.Error)
				}
				if !run.Correct() {
					t.Errorf("adversarial %q passed every check: the case does not catch it", adv.Name)
				}
			}
		})
	}
}

func TestExtractAnswer(t *testing.T) {
	text := "I looked at the timeline.\n```json\n{\"draft\": true}\n```\nFinal:\n```json\n{\"cause\": \"ingress_throttle\", \"totalDrops\": 81000}\n```"
	a, err := ExtractAnswer(text)
	if err != nil || a["cause"] != "ingress_throttle" {
		t.Errorf("answer = %v, %v; want the last block", a, err)
	}
	if _, err := ExtractAnswer("no json here"); err == nil {
		t.Error("a reply without an answer block must be an error, not an empty answer")
	}
}
