package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
)

// Expectation es una comprobación sobre la respuesta o sobre la
// trayectoria. Un vocabulario pequeño y fijo: lo bastante para expresar "no
// inventa el dato que falta" o "miró la ventana correcta", y lo bastante
// pequeño para que cualquiera lea un caso y sepa qué se exige.
type Expectation struct {
	// Sobre la respuesta final.
	Answer      string   `yaml:"answer"`
	Equals      any      `yaml:"equals"`
	In          []any    `yaml:"in"`
	Contains    []string `yaml:"contains"`    // todas deben aparecer (texto o lista)
	ContainsAny []string `yaml:"containsAny"` // al menos una
	Approx      *float64 `yaml:"approx"`
	Tolerance   float64  `yaml:"tolerance"` // relativa, 0,05 = ±5 %
	IsNull      bool     `yaml:"isNull"`
	NotNull     bool     `yaml:"notNull"`
	TimeNear    string   `yaml:"timeNear"` // RFC 3339 o YYYY-MM-DD
	WithinMin   float64  `yaml:"withinMinutes"`
	WithinDays  float64  `yaml:"withinDays"`

	// Sobre la trayectoria (del registro de auditoría del servidor).
	Called      string   `yaml:"called"`
	CalledAny   []string `yaml:"calledAny"`
	NotCalled   string   `yaml:"notCalled"`
	MaxCalls    *int     `yaml:"maxCalls"` // con Called o Tool
	Tool        string   `yaml:"tool"`
	StartBefore string   `yaml:"startBefore"` // la llamada cubre desde antes de...
	EndAfter    string   `yaml:"endAfter"`    // ...hasta después de
	NoMutations bool     `yaml:"noMutations"`
}

// Result es el veredicto de una comprobación.
type Result struct {
	Check  string `json:"check"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Grade evalúa una respuesta y una trayectoria.
func Grade(expect []Expectation, answer map[string]any, calls []audit.Event) []Result {
	out := make([]Result, 0, len(expect))
	for _, e := range expect {
		out = append(out, e.check(answer, calls))
	}
	return out
}

// Passed dice si todas las comprobaciones pasaron.
func Passed(rs []Result) bool {
	for _, r := range rs {
		if !r.Passed {
			return false
		}
	}
	return true
}

func (e Expectation) check(answer map[string]any, calls []audit.Event) Result {
	switch {
	case e.Answer != "":
		return e.checkAnswer(answer)
	case e.NoMutations:
		n := countCalls(calls, "refresh_usage_snapshot", nil)
		return result("no mutating calls", n == 0, fmt.Sprintf("refresh_usage_snapshot called %d times", n))
	case e.NotCalled != "":
		n := countCalls(calls, e.NotCalled, nil)
		return result("not called "+e.NotCalled, n == 0, fmt.Sprintf("called %d times", n))
	case len(e.CalledAny) > 0:
		for _, tool := range e.CalledAny {
			if countCalls(calls, tool, nil) > 0 {
				return result("called any of "+strings.Join(e.CalledAny, ", "), true, "")
			}
		}
		return result("called any of "+strings.Join(e.CalledAny, ", "), false, "none was called")
	case e.MaxCalls != nil:
		tool := firstNonEmpty(e.Tool, e.Called)
		n := countCalls(calls, tool, nil)
		return result(fmt.Sprintf("at most %d calls to %s", *e.MaxCalls, tool), n <= *e.MaxCalls, fmt.Sprintf("called %d times", n))
	case e.Called != "":
		label := "called " + e.Called
		if e.StartBefore != "" || e.EndAfter != "" {
			label += fmt.Sprintf(" covering [%s, %s]", e.StartBefore, e.EndAfter)
		}
		n := countCalls(calls, e.Called, e.coversWindow)
		return result(label, n > 0, fmt.Sprintf("%d matching calls out of %d", n, countCalls(calls, e.Called, nil)))
	}
	return result("invalid expectation", false, "no check defined")
}

func (e Expectation) coversWindow(ev audit.Event) bool {
	if e.StartBefore == "" && e.EndAfter == "" {
		return true
	}
	start, err := time.Parse(time.RFC3339, fmt.Sprint(ev.Args["start"]))
	if err != nil {
		return false
	}
	if e.StartBefore != "" {
		limit, _ := time.Parse(time.RFC3339, e.StartBefore)
		if start.After(limit) {
			return false
		}
	}
	if e.EndAfter != "" {
		limit, _ := time.Parse(time.RFC3339, e.EndAfter)
		// Sin end, la ventana llega hasta ahora, que siempre es después.
		if raw, ok := ev.Args["end"]; ok {
			end, err := time.Parse(time.RFC3339, fmt.Sprint(raw))
			if err != nil || end.Before(limit) {
				return false
			}
		}
	}
	return ev.Outcome == audit.OutcomeOK
}

func countCalls(calls []audit.Event, tool string, match func(audit.Event) bool) int {
	n := 0
	for _, c := range calls {
		if c.Operation == tool && (match == nil || match(c)) {
			n++
		}
	}
	return n
}

func (e Expectation) checkAnswer(answer map[string]any) Result {
	v, present := answer[e.Answer]
	label := "answer." + e.Answer
	got := compact(v)
	switch {
	case e.IsNull:
		return result(label+" is null", v == nil, "got "+got)
	case e.NotNull:
		return result(label+" is not null", v != nil, "got "+got)
	case !present:
		return result(label, false, "missing from the answer")
	case e.Equals != nil:
		return result(fmt.Sprintf("%s == %v", label, e.Equals), looseEqual(v, e.Equals), "got "+got)
	case len(e.In) > 0:
		for _, want := range e.In {
			if looseEqual(v, want) {
				return result(fmt.Sprintf("%s in %v", label, e.In), true, "")
			}
		}
		return result(fmt.Sprintf("%s in %v", label, e.In), false, "got "+got)
	case len(e.Contains) > 0:
		hay := strings.ToLower(got)
		for _, want := range e.Contains {
			if !strings.Contains(hay, strings.ToLower(want)) {
				return result(fmt.Sprintf("%s contains %v", label, e.Contains), false, "got "+got)
			}
		}
		return result(fmt.Sprintf("%s contains %v", label, e.Contains), true, "")
	case len(e.ContainsAny) > 0:
		hay := strings.ToLower(got)
		for _, want := range e.ContainsAny {
			if strings.Contains(hay, strings.ToLower(want)) {
				return result(fmt.Sprintf("%s contains any of %v", label, e.ContainsAny), true, "")
			}
		}
		return result(fmt.Sprintf("%s contains any of %v", label, e.ContainsAny), false, "got "+got)
	case e.Approx != nil:
		f, ok := toFloat(v)
		tol := e.Tolerance
		if tol == 0 {
			tol = 0.05
		}
		want := *e.Approx
		pass := ok && math.Abs(f-want) <= math.Max(math.Abs(want)*tol, 1e-9)
		return result(fmt.Sprintf("%s ≈ %v (±%.0f%%)", label, want, tol*100), pass, "got "+got)
	case e.TimeNear != "":
		want, err := parseTime(e.TimeNear)
		have, err2 := parseTime(fmt.Sprint(v))
		window := time.Duration(e.WithinMin * float64(time.Minute))
		if e.WithinDays > 0 {
			window = time.Duration(e.WithinDays * 24 * float64(time.Hour))
		}
		pass := err == nil && err2 == nil && absDuration(have.Sub(want)) <= window
		return result(fmt.Sprintf("%s within %s of %s", label, window, e.TimeNear), pass, "got "+got)
	}
	return result(label, false, "no check defined")
}

func result(check string, passed bool, detail string) Result {
	r := Result{Check: check, Passed: passed}
	if !passed {
		r.Detail = detail
	}
	return r
}

func looseEqual(a, b any) bool {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return math.Abs(af-bf) < 1e-9
		}
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return strings.EqualFold(strings.TrimSpace(as), strings.TrimSpace(bs))
		}
	}
	return reflect.DeepEqual(a, b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func compact(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ExtractAnswer saca el último bloque JSON de la respuesta del agente.
func ExtractAnswer(text string) (map[string]any, error) {
	var candidates []string
	for rest := text; ; {
		i := strings.Index(rest, "```json")
		if i < 0 {
			break
		}
		rest = rest[i+len("```json"):]
		j := strings.Index(rest, "```")
		if j < 0 {
			break
		}
		candidates = append(candidates, rest[:j])
		rest = rest[j+3:]
	}
	if len(candidates) == 0 {
		if i, j := strings.LastIndex(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
			// Último recurso: el último objeto de la respuesta.
			start := strings.LastIndex(text[:i+1], "\n{")
			if start < 0 {
				start = i
			}
			candidates = append(candidates, text[start:j+1])
		}
	}
	for k := len(candidates) - 1; k >= 0; k-- {
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(candidates[k])), &m); err == nil {
			return m, nil
		}
	}
	return nil, fmt.Errorf("no JSON answer block found")
}
