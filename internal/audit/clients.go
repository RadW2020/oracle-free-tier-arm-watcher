// Package audit sabe quién llama y deja constancia de lo que hace.
//
// Antes había una sola API_KEY compartida: el log no distinguía a Checkly de
// un agente, y la única traza eran los intentos fallidos. Aquí cada cliente
// tiene nombre y permisos, y cada llamada a la API /v1 o al servidor MCP
// deja un evento que contesta "¿qué hizo el agente?".
package audit

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"sort"
	"strings"
)

// Scope es un permiso de un cliente.
type Scope string

const (
	// ScopeRead permite leer: estado, cuotas, riesgo, saturación, diagnóstico.
	ScopeRead Scope = "read"
	// ScopeRefresh permite pedir una lectura nueva a OCI. Es el único efecto
	// que un cliente puede provocar, y gasta cuota de la API de OCI.
	ScopeRefresh Scope = "refresh"
	// ScopeAudit permite leer el registro de auditoría.
	ScopeAudit Scope = "audit"
)

var knownScopes = map[Scope]bool{ScopeRead: true, ScopeRefresh: true, ScopeAudit: true}

// Client es un cliente autenticado.
type Client struct {
	Name   string
	Scopes []Scope
}

// Has dice si el cliente tiene un permiso.
func (c Client) Has(s Scope) bool {
	for _, have := range c.Scopes {
		if have == s {
			return true
		}
	}
	return false
}

// ScopeStrings devuelve los permisos como cadenas.
func (c Client) ScopeStrings() []string {
	out := make([]string, len(c.Scopes))
	for i, s := range c.Scopes {
		out[i] = string(s)
	}
	return out
}

type entry struct {
	digest [sha256.Size]byte
	client Client
}

// Registry resuelve claves a clientes.
type Registry struct {
	entries   []entry
	anonymous *Client
}

// ParseClients construye el registro.
//
// apiClients tiene el formato `nombre:permiso+permiso:clave`, separado por
// comas: `checkly:read:abc,claude:read+refresh:def`. legacyKey es la
// API_KEY de siempre, que pasa a ser el cliente "legacy" con permiso de
// lectura para que Checkly siga funcionando sin tocar nada.
func ParseClients(apiClients, legacyKey string) (*Registry, error) {
	r := &Registry{}
	seen := map[string]bool{}
	if legacyKey != "" {
		r.add(Client{Name: "legacy", Scopes: []Scope{ScopeRead}}, legacyKey)
		seen["legacy"] = true
	}
	for _, raw := range strings.Split(apiClients, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
			return nil, fmt.Errorf("API_CLIENTS entry %q: want name:scope+scope:key", redact(raw))
		}
		name := parts[0]
		if seen[name] {
			return nil, fmt.Errorf("API_CLIENTS: duplicate client %q", name)
		}
		seen[name] = true
		var scopes []Scope
		for _, s := range strings.Split(parts[1], "+") {
			scope := Scope(strings.TrimSpace(s))
			if !knownScopes[scope] {
				return nil, fmt.Errorf("API_CLIENTS client %q: unknown scope %q (valid: read, refresh, audit)", name, scope)
			}
			scopes = append(scopes, scope)
		}
		sort.Slice(scopes, func(i, j int) bool { return scopes[i] < scopes[j] })
		r.add(Client{Name: name, Scopes: scopes}, parts[2])
	}
	return r, nil
}

// AllowAnonymous deja pasar peticiones sin clave como un cliente anónimo
// con todos los permisos. Sólo tiene sentido en el demo con fixtures.
func (r *Registry) AllowAnonymous() {
	r.anonymous = &Client{Name: "anonymous", Scopes: []Scope{ScopeAudit, ScopeRead, ScopeRefresh}}
}

func (r *Registry) add(c Client, key string) {
	r.entries = append(r.entries, entry{digest: sha256.Sum256([]byte(key)), client: c})
}

// Enforced dice si hace falta clave.
func (r *Registry) Enforced() bool { return r.anonymous == nil }

// Configured dice si hay algún cliente definido.
func (r *Registry) Configured() bool { return len(r.entries) > 0 }

// Clients devuelve los nombres de los clientes definidos.
func (r *Registry) Clients() []string {
	names := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		names = append(names, e.client.Name)
	}
	return names
}

// Authenticate resuelve una clave.
//
// Compara digests SHA-256 en tiempo constante y recorre todas las entradas
// siempre: ni la longitud de la clave ni su posición en la lista se pueden
// deducir midiendo cuánto tarda en fallar. La comparación anterior
// (providedKey != apiKey) no lo garantizaba.
func (r *Registry) Authenticate(key string) (Client, bool) {
	if key == "" {
		if r.anonymous != nil {
			return *r.anonymous, true
		}
		return Client{}, false
	}
	digest := sha256.Sum256([]byte(key))
	var found *Client
	for i := range r.entries {
		if subtle.ConstantTimeCompare(digest[:], r.entries[i].digest[:]) == 1 && found == nil {
			found = &r.entries[i].client
		}
	}
	if found == nil {
		return Client{}, false
	}
	return *found, true
}

func redact(entry string) string {
	if i := strings.LastIndex(entry, ":"); i >= 0 {
		return entry[:i+1] + "***"
	}
	return "***"
}

type ctxKey int

const (
	clientKey ctxKey = iota
	requestIDKey
	transportKey
	sessionKey
)

// WithClient guarda el cliente autenticado en el contexto.
func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

// ClientFrom devuelve el cliente del contexto.
func ClientFrom(ctx context.Context) (Client, bool) {
	c, ok := ctx.Value(clientKey).(Client)
	return c, ok
}

// WithRequest guarda el ID de petición, el transporte y la sesión MCP.
func WithRequest(ctx context.Context, requestID, transport, session string) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	ctx = context.WithValue(ctx, transportKey, transport)
	if session != "" {
		ctx = context.WithValue(ctx, sessionKey, session)
	}
	return ctx
}

// RequestID devuelve el ID de petición del contexto.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Transport devuelve el transporte ("rest" | "mcp") del contexto.
func Transport(ctx context.Context) string {
	t, _ := ctx.Value(transportKey).(string)
	return t
}

// Session devuelve la sesión MCP del contexto, si la hay.
func Session(ctx context.Context) string {
	s, _ := ctx.Value(sessionKey).(string)
	return s
}
