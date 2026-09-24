package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"semaps/core"
)

// runMCP starts the MCP server over stdio (JSON-RPC 2.0).
// Workspace is the models root; sourceRoot is for codeRef resolution.
func runMCP(workspace, sourceRoot string) int {
	srv := newMCPServer(workspace, sourceRoot)
	srv.run()
	return 0
}

// mcpServer implements MCP over JSON-RPC 2.0 on stdio.
type mcpServer struct {
	workspace  string
	sourceRoot string
}

func newMCPServer(workspace, sourceRoot string) *mcpServer {
	return &mcpServer{
		workspace:  workspace,
		sourceRoot: sourceRoot,
	}
}

// run reads requests from stdin and writes responses to stdout.
func (s *mcpServer) run() {
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := jsonRPCErrorResponse(nil, -32700, "Parse error")
			_ = enc.Encode(resp)
			continue
		}

		resp := s.handleRequest(&req)
		if resp != nil {
			_ = enc.Encode(resp)
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("stdin error: %v", err)
		os.Exit(1)
	}
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      *json.Number    `json:"id"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      *json.Number `json:"id,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func jsonRPCErrorResponse(id *json.Number, code int, msg string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		Error: jsonRPCError{
			Code:    code,
			Message: msg,
		},
		ID: id,
	}
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
func (s *mcpServer) handleRequest(req *jsonRPCRequest) *jsonRPCResponse {
	if req.JSONRPC != "2.0" {
		return jsonRPCErrorResponse(req.ID, -32600, "Invalid Request")
	}

	switch req.Method {
	// Lifecycle
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil // No response for notification

	// Discovery
	case "tools/list":
		return s.handleToolsList(req)

	// Read tools
	case "tools/call":
		return s.handleToolCall(req)

	default:
		return jsonRPCErrorResponse(req.ID, -32601, "Method not found")
	}
}

// handleInitialize responds to the initialize request.
func (s *mcpServer) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "semaps",
			"version": "1.0",
		},
	}
	resp := &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
	return resp
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// handleToolsList lists available tools.
func (s *mcpServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	tools := []Tool{
		{
			Name:        "list_projects",
			Description: "List all projects in the workspace",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []string{},
			},
		},
		{
			Name:        "list_views",
			Description: "List views of a project",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
				},
				"required": []string{"project"},
			},
		},
		{
			Name:        "get_entity",
			Description: "Get an entity by ID, name, or symbol",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID (e_...)",
					},
				},
				"required": []string{"project", "id"},
			},
		},
		{
			Name:        "get_text",
			Description: "Get a text value (name, description, etc.) for an entity or relation",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID (e_...) or relation type ID (rt_...)",
					},
					"field": map[string]interface{}{
						"type":        "string",
						"description": "Field name: name, description, etc.",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": "Language code (e.g., ru, en)",
					},
				},
				"required": []string{"project", "id", "field", "language"},
			},
		},
		{
			Name:        "set_text",
			Description: "Set a text value for an entity",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID (e_...)",
					},
					"field": map[string]interface{}{
						"type":        "string",
						"description": "Field name: name, description, etc.",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Text value",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": "Language code (e.g., ru, en)",
					},
				},
				"required": []string{"project", "id", "field", "value", "language"},
			},
		},
		{
			Name:        "doctor",
			Description: "Run diagnostic checks on the project",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
				},
				"required": []string{},
			},
		},
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	resp := &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
	return resp
}

// handleToolCall invokes a tool with the given arguments.
func (s *mcpServer) handleToolCall(req *jsonRPCRequest) *jsonRPCResponse {
	var args struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &args); err != nil {
		return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
	}

	var result interface{}
	var errMsg string

	switch args.Name {
	case "list_projects":
		index := core.Index(s.workspace)
		result = map[string]interface{}{
			"projects": index.Projects,
		}

	case "list_views":
		var params struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		index := core.Index(s.workspace)
		for _, p := range index.Projects {
			if p.ID == params.Project {
				result = map[string]interface{}{
					"views": p.Views,
				}
				break
			}
		}
		if result == nil {
			errMsg = "Project not found"
		}

	case "get_entity":
		var params struct {
			Project string `json:"project"`
			ID      string `json:"id"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		ent, err := s.getEntity(params.Project, params.ID)
		if err != nil {
			errMsg = err.Error()
		} else {
			result = ent
		}

	case "get_text":
		var params struct {
			Project  string `json:"project"`
			ID       string `json:"id"`
			Field    string `json:"field"`
			Language string `json:"language"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		text, err := s.getText(params.Project, params.ID, params.Field, params.Language)
		if err != nil {
			errMsg = err.Error()
		} else {
			result = text
		}

	case "set_text":
		var params struct {
			Project  string `json:"project"`
			ID       string `json:"id"`
			Field    string `json:"field"`
			Value    string `json:"value"`
			Language string `json:"language"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		if err := s.setText(params.Project, params.ID, params.Field, params.Value, params.Language); err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"status": "ok",
			}
		}

	case "doctor":
		var params struct {
			Project string `json:"project"`
		}
		_ = json.Unmarshal(args.Arguments, &params)
		report := core.Check(s.workspace, s.sourceRoot)
		result = map[string]interface{}{
			"findings": report,
		}

	default:
		return jsonRPCErrorResponse(req.ID, -32601, "Method not found")
	}

	if errMsg != "" {
		return jsonRPCErrorResponse(req.ID, -32000, errMsg)
	}

	resp := &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
	return resp
}

// Helper methods to access entities and text data

func (s *mcpServer) getEntity(projectID, entityID string) (interface{}, error) {
	_, err := s.loadProject(projectID)
	if err != nil {
		return nil, err
	}

	var entities map[string]interface{}
	path := filepath.Join(s.workspace, "projects", projectID, "entities.json")
	if err := loadJSON(path, &entities); err != nil {
		return nil, fmt.Errorf("failed to load entities: %v", err)
	}

	if data, ok := entities[entityID]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("entity %s not found", entityID)
}

func (s *mcpServer) getText(projectID, id, field, language string) (interface{}, error) {
	path := filepath.Join(s.workspace, "projects", projectID, "text."+language+".json")
	var texts map[string]map[string]interface{}
	if err := loadJSON(path, &texts); err != nil {
		return nil, fmt.Errorf("failed to load texts: %v", err)
	}

	if textData, ok := texts[id]; ok {
		if fieldData, ok := textData[field]; ok {
			return fieldData, nil
		}
	}
	return nil, fmt.Errorf("text for %s.%s not found", id, field)
}

func (s *mcpServer) setText(projectID, id, field, value, language string) error {
	path := filepath.Join(s.workspace, "projects", projectID, "text."+language+".json")
	var texts map[string]map[string]interface{}
	if err := loadJSON(path, &texts); err != nil {
		// Create if doesn't exist
		texts = make(map[string]map[string]interface{})
	}

	if texts[id] == nil {
		texts[id] = make(map[string]interface{})
	}

	// Create text value with origin: authored
	texts[id][field] = map[string]interface{}{
		"v":      value,
		"origin": "authored",
		"at":     formatISO8601(),
	}

	return saveJSON(path, texts)
}

func (s *mcpServer) loadProject(projectID string) (interface{}, error) {
	path := filepath.Join(s.workspace, "projects", projectID, "project.json")
	var proj interface{}
	if err := loadJSON(path, &proj); err != nil {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	return proj, nil
}

// Helper to read JSON files from disk

func loadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Helper to write JSON files to disk

func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// formatISO8601 returns the current time in ISO 8601 format with Z suffix
func formatISO8601() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

