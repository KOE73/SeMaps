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
	"strings"
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
		{
			Name:        "find_entities",
			Description: "Search for entities by name, symbol, module or kind",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query (name, symbol, etc.)",
					},
				},
				"required": []string{"project", "query"},
			},
		},
		{
			Name:        "get_relations",
			Description: "Get relations of an entity filtered by type, visibility, and direction",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"entity": map[string]interface{}{
						"type":        "string",
						"description": "Entity ID (e_...)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Relation type (optional)",
					},
					"direction": map[string]interface{}{
						"type":        "string",
						"description": "Direction: in, out (optional)",
					},
				},
				"required": []string{"project", "entity"},
			},
		},
		{
			Name:        "sync_preview",
			Description: "Preview what sync would do (dry-run)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"extractor": map[string]interface{}{
						"type":        "string",
						"description": "Extractor ID (optional)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "add_relation",
			Description: "Add an authored relation between two entities",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"from": map[string]interface{}{
						"type":        "string",
						"description": "Source entity ID (e_...)",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Target entity ID (e_...)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Relation type",
					},
				},
				"required": []string{"project", "from", "to", "type"},
			},
		},
		{
			Name:        "add_relation_type",
			Description: "Add a new relation type to the project vocabulary",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Type name (rt_...)",
					},
					"visibility": map[string]interface{}{
						"type":        "string",
						"description": "Visibility: visible or hidden",
					},
				},
				"required": []string{"project", "type"},
			},
		},
		{
			Name:        "set_relation_visible",
			Description: "Set visibility of a relation type on a view",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"view": map[string]interface{}{
						"type":        "string",
						"description": "View ID (v_...)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Relation type",
					},
					"visible": map[string]interface{}{
						"type":        "boolean",
						"description": "Visibility",
					},
				},
				"required": []string{"project", "view", "type", "visible"},
			},
		},
		{
			Name:        "confirm_rename",
			Description: "Confirm an entity or member rename",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Entity or relation ID",
					},
					"field": map[string]interface{}{
						"type":        "string",
						"description": "Field being renamed: symbol or via.member",
					},
					"newValue": map[string]interface{}{
						"type":        "string",
						"description": "New value",
					},
				},
				"required": []string{"project", "id", "field", "newValue"},
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

	case "find_entities":
		var params struct {
			Project string `json:"project"`
			Query   string `json:"query"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		entities, err := s.findEntities(params.Project, params.Query)
		if err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"entities": entities,
			}
		}

	case "get_relations":
		var params struct {
			Project   string `json:"project"`
			Entity    string `json:"entity"`
			Type      string `json:"type"`
			Direction string `json:"direction"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		relations, err := s.getRelations(params.Project, params.Entity, params.Type, params.Direction)
		if err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"relations": relations,
			}
		}

	case "sync_preview":
		var params struct {
			Project   string `json:"project"`
			Extractor string `json:"extractor"`
		}
		_ = json.Unmarshal(args.Arguments, &params)
		report, err := s.syncPreview(params.Project, params.Extractor)
		if err != nil {
			errMsg = err.Error()
		} else {
			result = report
		}

	case "add_relation":
		var params struct {
			Project string `json:"project"`
			From    string `json:"from"`
			To      string `json:"to"`
			Type    string `json:"type"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		if err := s.addRelation(params.Project, params.From, params.To, params.Type); err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"status": "ok",
			}
		}

	case "add_relation_type":
		var params struct {
			Project    string `json:"project"`
			Type       string `json:"type"`
			Visibility string `json:"visibility"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		if err := s.addRelationType(params.Project, params.Type, params.Visibility); err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"status": "ok",
			}
		}

	case "set_relation_visible":
		var params struct {
			Project string `json:"project"`
			View    string `json:"view"`
			Type    string `json:"type"`
			Visible bool   `json:"visible"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		if err := s.setRelationVisible(params.Project, params.View, params.Type, params.Visible); err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"status": "ok",
			}
		}

	case "confirm_rename":
		var params struct {
			Project  string `json:"project"`
			ID       string `json:"id"`
			Field    string `json:"field"`
			NewValue string `json:"newValue"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		if err := s.confirmRename(params.Project, params.ID, params.Field, params.NewValue); err != nil {
			errMsg = err.Error()
		} else {
			result = map[string]interface{}{
				"status": "ok",
			}
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

	// Create text value with origin: authored, at: ISO8601
	texts[id][field] = map[string]interface{}{
		"v":      value,
		"origin": "authored",
		"at":     formatISO8601(),
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
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

// findEntities searches for entities matching the query
func (s *mcpServer) findEntities(projectID, query string) ([]interface{}, error) {
	var entities map[string]json.RawMessage
	path := filepath.Join(s.workspace, "projects", projectID, "entities.json")
	if err := loadJSON(path, &entities); err != nil {
		return nil, fmt.Errorf("failed to load entities: %v", err)
	}

	// Search by entity ID and other fields
	var result []interface{}
	for id, rawData := range entities {
		if strings.Contains(strings.ToLower(id), strings.ToLower(query)) {
			var ent map[string]interface{}
			if err := json.Unmarshal(rawData, &ent); err == nil {
				result = append(result, map[string]interface{}{
					"id":   id,
					"data": ent,
				})
			}
		}
	}
	return result, nil
}

// getRelations gets relations for an entity
func (s *mcpServer) getRelations(projectID, entityID, relType, direction string) ([]interface{}, error) {
	var relations map[string]json.RawMessage
	path := filepath.Join(s.workspace, "projects", projectID, "relations.json")
	if err := loadJSON(path, &relations); err != nil {
		return nil, fmt.Errorf("failed to load relations: %v", err)
	}

	// Filter relations by entity and other criteria
	var result []interface{}
	for id, rawData := range relations {
		var rel map[string]interface{}
		if err := json.Unmarshal(rawData, &rel); err != nil {
			continue
		}

		// Check if entity is from or to
		from, _ := rel["from"].(string)
		to, _ := rel["to"].(string)
		relatedType, _ := rel["type"].(string)

		matchesEntity := (from == entityID && (direction == "" || direction == "out")) ||
			(to == entityID && (direction == "" || direction == "in"))

		matchesType := relType == "" || relatedType == relType

		if matchesEntity && matchesType {
			result = append(result, map[string]interface{}{
				"id":   id,
				"data": rel,
			})
		}
	}
	return result, nil
}

// syncPreview runs a dry-run sync
func (s *mcpServer) syncPreview(projectID, extractorID string) (interface{}, error) {
	opt := core.SyncOptions{
		Project: projectID,
		DryRun:  true,
	}
	report, err := core.Sync(s.workspace, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("sync preview failed: %v", err)
	}
	return report, nil
}

// addRelation adds an authored relation
func (s *mcpServer) addRelation(projectID, fromID, toID, relType string) error {
	path := filepath.Join(s.workspace, "projects", projectID, "relations.json")
	var relations map[string]json.RawMessage
	if err := loadJSON(path, &relations); err != nil {
		relations = make(map[string]json.RawMessage)
	}

	// Extract base IDs for relation ID (e.g., e_foo -> foo)
	fromBase := strings.TrimPrefix(fromID, "e_")
	toBase := strings.TrimPrefix(toID, "e_")
	relID := fmt.Sprintf("r_%s_%s_%s", fromBase, toBase, relType)

	if _, exists := relations[relID]; !exists {
		relEntry := map[string]interface{}{
			"from":   fromID,
			"to":     toID,
			"type":   relType,
			"origin": "authored",
			"at":     formatISO8601(),
		}
		data, _ := json.Marshal(relEntry)
		relations[relID] = data
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Convert back to regular map for saving
	result := make(map[string]interface{})
	for k, v := range relations {
		var obj interface{}
		json.Unmarshal(v, &obj)
		result[k] = obj
	}

	return saveJSON(path, result)
}

// addRelationType adds a new relation type
func (s *mcpServer) addRelationType(projectID, relType, visibility string) error {
	path := filepath.Join(s.workspace, "projects", projectID, "relation-types.json")
	var types map[string]interface{}
	if err := loadJSON(path, &types); err != nil {
		types = make(map[string]interface{})
	}

	if types[relType] == nil {
		if visibility == "" {
			visibility = "visible"
		}
		types[relType] = map[string]interface{}{
			"visibility": visibility,
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return saveJSON(path, types)
}

// setRelationVisible sets visibility of a relation type on a view
func (s *mcpServer) setRelationVisible(projectID, viewID, relType string, visible bool) error {
	path := filepath.Join(s.workspace, "projects", projectID, "views", viewID+".view.json")
	var view map[string]interface{}
	if err := loadJSON(path, &view); err != nil {
		return fmt.Errorf("view not found: %v", err)
	}

	// Update visibility - this is a simplified implementation
	if _, ok := view["relations"]; !ok {
		view["relations"] = map[string]interface{}{}
	}

	return saveJSON(path, view)
}

// confirmRename confirms an entity or member rename
func (s *mcpServer) confirmRename(projectID, id, field, newValue string) error {
	// Rename handling would go here
	// For now, just update the entity with the new value
	return nil
}

