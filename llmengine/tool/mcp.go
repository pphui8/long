package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultMCPServerURL = "http://127.0.0.1:9002/mcp"
	defaultMCPTimeout   = 30 * time.Second
	mcpProtocolVersion  = "2025-06-18"
)

type MCPClient struct {
	client    *http.Client
	endpoint  string
	nextID    atomic.Int64
	sessionID string
	mu        sync.Mutex
}

type MCPTool struct {
	client      *MCPClient
	name        string
	description string
	inputSchema json.RawMessage
}

func NewMCPTools(ctx context.Context, endpoint string) ([]*MCPTool, error) {
	client, err := NewMCPClient(endpoint)
	if err != nil {
		return nil, err
	}
	if err := client.Initialize(ctx); err != nil {
		return nil, err
	}
	return client.ListTools(ctx)
}

func MCPServerURL(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return DefaultMCPServerURL
	}
	return endpoint
}

func NewMCPClient(endpoint string) (*MCPClient, error) {
	normalized, err := normalizeMCPServerURL(endpoint)
	if err != nil {
		return nil, err
	}
	return &MCPClient{
		client:   &http.Client{Timeout: defaultMCPTimeout},
		endpoint: normalized,
	}, nil
}

func (c *MCPClient) Initialize(ctx context.Context) error {
	_, err := c.send(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "long",
			"version": "0.1.0",
		},
	}, true)
	if err != nil {
		return err
	}

	_, err = c.send(ctx, "notifications/initialized", map[string]any{}, false)
	return err
}

func (c *MCPClient) ListTools(ctx context.Context) ([]*MCPTool, error) {
	result, err := c.send(ctx, "tools/list", map[string]any{}, true)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, err
	}

	tools := make([]*MCPTool, 0, len(parsed.Tools))
	for _, remoteTool := range parsed.Tools {
		name := strings.TrimSpace(remoteTool.Name)
		if name == "" {
			continue
		}
		tools = append(tools, &MCPTool{
			client:      c,
			name:        name,
			description: strings.TrimSpace(remoteTool.Description),
			inputSchema: remoteTool.InputSchema,
		})
	}
	return tools, nil
}

func (t *MCPTool) Name() string {
	return t.name
}

func (t *MCPTool) Description() string {
	description := strings.TrimSpace(t.description)
	if description == "" {
		description = "Remote MCP tool."
	}
	if len(t.inputSchema) == 0 || string(t.inputSchema) == "null" {
		return description
	}
	return description + " Input should be JSON matching this schema: " + string(t.inputSchema)
}

func (t *MCPTool) Call(ctx context.Context, input string) (string, error) {
	result, err := t.client.send(ctx, "tools/call", map[string]any{
		"name":      t.name,
		"arguments": t.arguments(input),
	}, true)
	if err != nil {
		return "", err
	}
	return formatMCPToolResult(result)
}

func (t *MCPTool) arguments(input string) map[string]any {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]any{}
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}

	if property, ok := singleSchemaProperty(t.inputSchema); ok {
		return map[string]any{property: trimmed}
	}
	return map[string]any{"input": trimmed}
}

func (c *MCPClient) send(ctx context.Context, method string, params any, expectResult bool) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if expectResult {
		request["id"] = id
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	if sessionID := c.getSessionID(); sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
		c.setSessionID(sessionID)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("mcp %s returned status %d: %s", method, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if !expectResult {
		return nil, nil
	}

	payload, err := readMCPResponse(resp)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("mcp %s failed with code %d: %s", method, parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Result) == 0 {
		return nil, errors.New("mcp response missing result")
	}
	return parsed.Result, nil
}

func (c *MCPClient) getSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

func (c *MCPClient) setSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
}

func readMCPResponse(resp *http.Response) ([]byte, error) {
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readMCPEventStream(resp.Body)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("mcp response body is empty")
	}
	return trimmed, nil
}

func readMCPEventStream(body io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	var data strings.Builder
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		if line == "" && data.Len() > 0 {
			return []byte(strings.TrimSpace(data.String())), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(data.String())
	if trimmed == "" {
		return nil, errors.New("mcp event stream response missing data")
	}
	return []byte(trimmed), nil
}

func formatMCPToolResult(result json.RawMessage) (string, error) {
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return "", err
	}

	parts := make([]string, 0, len(parsed.Content))
	for _, content := range parsed.Content {
		if strings.EqualFold(content.Type, "text") && strings.TrimSpace(content.Text) != "" {
			parts = append(parts, strings.TrimSpace(content.Text))
		}
	}

	output := strings.TrimSpace(strings.Join(parts, "\n"))
	if output == "" {
		output = strings.TrimSpace(string(result))
	}
	if parsed.IsError {
		return "", errors.New(output)
	}
	return output, nil
}

func normalizeMCPServerURL(endpoint string) (string, error) {
	endpoint = MCPServerURL(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid MCP server URL %q", endpoint)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/mcp"
	}
	return parsed.String(), nil
}

func singleSchemaProperty(schema json.RawMessage) (string, bool) {
	if len(schema) == 0 {
		return "", false
	}
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil || len(parsed.Properties) != 1 {
		return "", false
	}
	for property := range parsed.Properties {
		return property, property != ""
	}
	return "", false
}
