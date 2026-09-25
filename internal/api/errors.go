// Package api contiene las operaciones que el watcher ofrece a clientes
// automáticos: la API /v1 y el servidor MCP son dos transportes sobre este
// mismo paquete, y por eso no pueden contestar distinto a la misma pregunta.
//
// Cada operación valida su entrada, comprueba el permiso del cliente, deja un
// evento de auditoría y devuelve un tipo con esquema. Los errores llevan un
// código estable y dicen si reintentar sirve de algo.
package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

// ErrorCode es un código de error estable.
type ErrorCode string

const (
	CodeInvalidArgument   ErrorCode = "invalid_argument"
	CodeOutOfRange        ErrorCode = "out_of_range"
	CodeUnauthenticated   ErrorCode = "unauthenticated"
	CodePermissionDenied  ErrorCode = "permission_denied"
	CodeNotConfigured     ErrorCode = "not_configured"
	CodeSourceUnavailable ErrorCode = "source_unavailable"
	CodeRateLimited       ErrorCode = "rate_limited"
	CodeTimeout           ErrorCode = "timeout"
	CodeNotFound          ErrorCode = "not_found"
	CodeMethodNotAllowed  ErrorCode = "method_not_allowed"
	CodeInternal          ErrorCode = "internal"
)

// ErrorCodes enumera los códigos, para el esquema.
var ErrorCodes = []ErrorCode{
	CodeInvalidArgument, CodeOutOfRange, CodeUnauthenticated, CodePermissionDenied,
	CodeNotConfigured, CodeSourceUnavailable, CodeRateLimited, CodeTimeout,
	CodeNotFound, CodeMethodNotAllowed, CodeInternal,
}

// Error es el error de cualquier operación, igual en REST y en MCP.
type Error struct {
	Code              ErrorCode `json:"code" jsonschema:"Stable error code. Branch on this, not on the message."`
	Message           string    `json:"message" jsonschema:"What went wrong, for humans and models."`
	Field             string    `json:"field,omitempty" jsonschema:"The input field that caused the error, if any."`
	Hint              string    `json:"hint,omitempty" jsonschema:"What to do next to recover."`
	Retryable         bool      `json:"retryable" jsonschema:"Whether repeating the same call later can succeed."`
	RetryAfterSeconds int       `json:"retryAfterSeconds,omitempty" jsonschema:"Seconds to wait before retrying, when known."`
	RequiredScope     string    `json:"requiredScope,omitempty" jsonschema:"The scope the client is missing, for permission_denied."`
	UpstreamCode      string    `json:"upstreamCode,omitempty" jsonschema:"Classified OCI failure (e.g. oci_throttled, oci_permission, timeout) behind a source_unavailable error."`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// ErrorEnvelope es el cuerpo de una respuesta de error.
type ErrorEnvelope struct {
	Error *Error `json:"error"`
}

// HTTPStatus traduce un código a su estado HTTP.
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeInvalidArgument, CodeOutOfRange:
		return http.StatusBadRequest
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeNotConfigured, CodeSourceUnavailable:
		return http.StatusServiceUnavailable
	case CodeTimeout:
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

// AsError convierte cualquier error en un *Error.
func AsError(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return fromSource(err)
}

// fromSource traduce un fallo de la fuente a un error de la API.
func fromSource(err error) *Error {
	code, message := source.Classify(err)
	switch code {
	case "not_configured":
		return &Error{
			Code: CodeNotConfigured, Message: "the watcher has no OCI credentials, so nothing can be measured",
			Hint: "set OCI_TENANCY_ID, OCI_USER_ID, OCI_FINGERPRINT, OCI_PRIVATE_KEY_PATH and OCI_REGION, or run with DATA_SOURCE=fixture for the demo; get_watcher_diagnostics shows what is missing",
		}
	case "timeout":
		return &Error{Code: CodeTimeout, Message: message, Retryable: true, UpstreamCode: code}
	case "invalid_metric":
		return &Error{Code: CodeInvalidArgument, Message: message, Field: "metrics"}
	}
	return &Error{
		Code: CodeSourceUnavailable, Message: message, Retryable: source.Retryable(code), UpstreamCode: code,
		Hint: "get_watcher_diagnostics shows which OCI source failed and since when",
	}
}

func invalid(field, format string, args ...any) *Error {
	return &Error{Code: CodeInvalidArgument, Field: field, Message: fmt.Sprintf(format, args...)}
}

func denied(scope audit.Scope) *Error {
	return &Error{
		Code: CodePermissionDenied, RequiredScope: string(scope),
		Message: fmt.Sprintf("this client lacks the %q scope", scope),
		Hint:    "ask the operator for a key with that scope; the data you can already read is still valid",
	}
}
