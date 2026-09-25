package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Case es una tarea que un agente tiene que poder resolver con la interfaz
// del watcher. Se evalúa el resultado y el comportamiento, nunca la
// redacción: la respuesta final se pide en un bloque JSON con un contrato
// fijo, y la trayectoria se lee del registro de auditoría del propio
// servidor.
type Case struct {
	ID       string `yaml:"id"`
	UseCase  string `yaml:"useCase"`
	Title    string `yaml:"title"`
	Scenario string `yaml:"scenario"`
	// Scopes de la clave con la que corre el agente. Por defecto, read.
	Scopes []string `yaml:"scopes"`
	Prompt string   `yaml:"prompt"`
	// Answer es el contrato de la respuesta: clave → qué debe contener.
	Answer      yaml.Node     `yaml:"answer"`
	Expect      []Expectation `yaml:"expect"`
	Reference   Trajectory    `yaml:"reference"`
	Adversarial []Adversarial `yaml:"adversarial"`
}

// Trajectory es una secuencia de llamadas y la respuesta que sale de ella.
type Trajectory struct {
	Calls  []Call         `yaml:"calls"`
	Answer map[string]any `yaml:"answer"`
}

// Call es una llamada a una tool.
type Call struct {
	Tool string         `yaml:"tool"`
	Args map[string]any `yaml:"args"`
}

// Adversarial es una trayectoria incorrecta que los evaluadores tienen que
// rechazar. Es lo que demuestra que una regresión se detectaría: si una de
// éstas pasa, el caso no está probando nada.
type Adversarial struct {
	Name       string `yaml:"name"`
	Trajectory `yaml:",inline"`
}

// AnswerFields devuelve el contrato de la respuesta en el orden del YAML.
func (c Case) AnswerFields() [][2]string {
	var out [][2]string
	n := c.Answer
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, [2]string{n.Content[i].Value, n.Content[i+1].Value})
	}
	return out
}

// Contract es el texto que se le añade al agente para que su respuesta sea
// evaluable. Sólo describe el formato: nada de pistas sobre cómo resolver la
// tarea.
//
// El encuadre del principio no es una pista: el watcher etiqueta los
// escenarios como sintéticos (meta.dataSource=fixture) y las instrucciones
// del servidor piden no presentarlos como datos de la cuenta. Sin decirle al
// agente que el escenario ES la cuenta de la tarea, un agente prudente se
// niega a contestar sobre "mi factura" —pasó en la primera ejecución real de
// billing-at-risk—, y eso mide la etiqueta, no la tarea.
func (c Case) Contract() string {
	var b strings.Builder
	b.WriteString("This is an evaluation. The watcher you are connected to serves a synthetic scenario (meta.dataSource=fixture); for this task, treat that scenario as the operator's account.\n\n")
	b.WriteString("When you have finished, end your reply with exactly one fenced ```json block containing an object with these keys:\n")
	for _, f := range c.AnswerFields() {
		fmt.Fprintf(&b, "- %s: %s\n", f[0], f[1])
	}
	b.WriteString("Use null for any value you could not determine.")
	return b.String()
}

// LoadCases carga los casos de un directorio, opcionalmente filtrados.
func LoadCases(dir, filter string) ([]Case, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var cases []Case
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Case
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if c.ID == "" || c.Scenario == "" || c.Prompt == "" || len(c.Expect) == 0 {
			return nil, fmt.Errorf("%s: id, scenario, prompt and expect are required", f)
		}
		if filter != "" && !strings.Contains(c.ID, filter) {
			continue
		}
		if len(c.Scopes) == 0 {
			c.Scopes = []string{"read"}
		}
		cases = append(cases, c)
	}
	return cases, nil
}
