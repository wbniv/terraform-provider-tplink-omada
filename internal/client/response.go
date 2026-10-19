package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"
)

const maxAPIResponseBytes = 16 << 20

// HTTPError means the HTTP exchange completed with a non-success status.
// Even 404 is not proof that a managed resource is absent: a proxy or an
// unsupported endpoint can produce it. Resource-specific classification is separate.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP error %d (%s)", e.StatusCode, http.StatusText(e.StatusCode))
}

// APIError is an explicit nonzero Omada code in a valid HTTP 2xx response.
// Arbitrary server messages are excluded because they can echo secrets.
type APIError struct{ Code int }

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d (controller rejected the operation)", e.Code)
}

// ProtocolError means the response cannot be trusted as an acknowledgment.
type ProtocolError struct{ reason string }

func (e *ProtocolError) Error() string { return "invalid API response: " + e.reason }

// TransportError preserves its cause for errors.Is/As without including a
// token-bearing URL or transport/server-supplied text in diagnostics.
type TransportError struct {
	operation string
	cause     error
}

func (e *TransportError) Error() string { return e.operation + ": transport failure" }
func (e *TransportError) Unwrap() error { return e.cause }

func (c *Client) doAPIRequest(req *http.Request) (*APIResponse, error) {
	// Use a per-call copy so redirect policy is consistent for discovery, login,
	// JSON and multipart operations, without mutating the shared client.
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &TransportError{operation: "sending API request", cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
	if err != nil {
		return nil, &TransportError{operation: "reading API response", cause: err}
	}
	if len(body) > maxAPIResponseBytes {
		return nil, &ProtocolError{reason: "body exceeds 16 MiB limit"}
	}
	apiResp, err := decodeAPIResponse(body)
	if err != nil {
		return nil, err
	}
	if apiResp.ErrorCode != 0 {
		return nil, &APIError{Code: apiResp.ErrorCode}
	}
	return apiResp, nil
}

func decodeAPIResponse(body []byte) (*APIResponse, error) {
	invalid := func() (*APIResponse, error) {
		return nil, &ProtocolError{reason: "expected one JSON object with an explicit integer errorCode"}
	}
	if !utf8.Valid(body) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return invalid()
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return invalid()
		}
		key, ok := token.(string)
		if !ok {
			return invalid()
		}
		if _, duplicate := fields[key]; duplicate {
			return invalid()
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return invalid()
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return invalid()
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalid()
	}
	code, ok := fields["errorCode"]
	if !ok || bytes.Equal(bytes.TrimSpace(code), []byte("null")) {
		return invalid()
	}
	result := &APIResponse{Result: fields["result"]}
	if err := json.Unmarshal(code, &result.ErrorCode); err != nil {
		return invalid()
	}
	if msg, ok := fields["msg"]; ok {
		if err := json.Unmarshal(msg, &result.Msg); err != nil {
			return invalid()
		}
	}
	return result, nil
}

func isAPIError(err error, code int) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Code == code
}
