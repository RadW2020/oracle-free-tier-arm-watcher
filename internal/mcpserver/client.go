package mcpserver

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Connect abre una sesión MCP contra un watcher con una clave. Lo usan los
// tests de integración y el runner guionizado de las evals: hablan el mismo
// protocolo, por la misma ruta HTTP, que un cliente real como Claude Code.
func Connect(ctx context.Context, endpoint, key string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "oci-watcher-test-client", Version: "dev"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearer{key: key, next: http.DefaultTransport}},
	}
	return client.Connect(ctx, transport, nil)
}

type bearer struct {
	key  string
	next http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.key != "" {
		r.Header.Set("Authorization", "Bearer "+b.key)
	}
	return b.next.RoundTrip(r)
}
