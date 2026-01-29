// HTTP Client Tool.
//
// Information Hiding:
// - HTTP client implementation details hidden
// - Request/response handling abstracted
// - Error handling and retries hidden

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPTool makes HTTP requests.
type HTTPTool struct {
	BaseTool
	client         *http.Client
	timeoutSecs    uint64
	allowedDomains []string
}

// NewHTTPTool creates a new HTTP tool with the given timeout.
func NewHTTPTool(timeoutSecs uint64) *HTTPTool {
	return &HTTPTool{
		client: &http.Client{
			Timeout: time.Duration(timeoutSecs) * time.Second,
		},
		timeoutSecs: timeoutSecs,
	}
}

// WithAllowedDomains sets the allowed domains for requests.
func (t *HTTPTool) WithAllowedDomains(domains []string) *HTTPTool {
	t.allowedDomains = domains
	return t
}

// Metadata returns the tool metadata.
func (t *HTTPTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:        "http_request",
		Description: "Make HTTP GET or POST requests to fetch data from URLs",
		Parameters: []ToolParameter{
			{Name: "url", ParamType: "string", Description: "The URL to request", Required: true},
			{Name: "method", ParamType: "string", Description: "HTTP method (GET or POST)", Required: false},
			{Name: "body", ParamType: "string", Description: "Request body for POST requests", Required: false},
		},
	}
}

type httpArgs struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Body   string `json:"body"`
}

// Validate validates the arguments.
func (t *HTTPTool) Validate(args json.RawMessage) error {
	var a httpArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if a.URL == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	return nil
}

// Execute makes the HTTP request.
func (t *HTTPTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	var a httpArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return FailureResult(fmt.Errorf("invalid arguments: %w", err)), nil
	}

	if a.URL == "" {
		return FailureResultf("URL cannot be empty"), nil
	}

	if !t.isDomainAllowed(a.URL) {
		return FailureResultf("access to domain in '%s' is not allowed", a.URL), nil
	}

	method := strings.ToUpper(a.Method)
	if method == "" {
		method = "GET"
	}

	if method != "GET" && method != "POST" {
		return FailureResultf("only GET and POST methods are supported"), nil
	}

	var req *http.Request
	var err error

	if method == "POST" {
		req, err = http.NewRequestWithContext(ctx, method, a.URL, strings.NewReader(a.Body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, a.URL, nil)
	}

	if err != nil {
		return FailureResult(fmt.Errorf("failed to create request: %w", err)), nil
	}

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return FailureResultf("request timed out after %d seconds", t.timeoutSecs), nil
		}
		return FailureResult(fmt.Errorf("request failed: %w", err)), nil
	}
	if resp == nil {
		return FailureResult(fmt.Errorf("nil response received")), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FailureResult(fmt.Errorf("failed to read response body: %w", err)), nil
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return SuccessResult(fmt.Sprintf("Status: %s\n\n%s", resp.Status, string(body))), nil
	}

	return FailureResultf("HTTP error: %s\n\n%s", resp.Status, string(body)), nil
}

// isDomainAllowed checks if the URL's domain is in the allowlist.
// Uses proper URL parsing to prevent bypass attacks.
func (t *HTTPTool) isDomainAllowed(urlStr string) bool {
	if len(t.allowedDomains) == 0 {
		return true
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := u.Hostname()
	for _, domain := range t.allowedDomains {
		// Exact match or subdomain match
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
