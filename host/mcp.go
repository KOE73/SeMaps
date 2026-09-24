package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"semaps/core"
)

// runMCP starts the MCP server over stdio.
// The --project flag selects which project to work with when there are multiple.
func runMCP(workspace, sourceRoot string) int {
	projectID := flag.String("project", "", "Project ID (required if workspace has multiple projects)")
	flag.Parse()

	srv := newMCPServer(workspace, sourceRoot, *projectID)
	if err := srv.run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		return 1
	}
	return 0
}

// mcpServer implements the MCP protocol over stdio using simple JSON-RPC 2.0.
type mcpServer struct {
	workspace  string
	sourceRoot string
	projectID  string
}

func newMCPServer(workspace, sourceRoot, projectID string) *mcpServer {
	return &mcpServer{
		workspace:  workspace,
		sourceRoot: sourceRoot,
		projectID:  projectID,
	}
}

// run starts the MCP server and handles JSON-RPC requests over stdio
func (s *mcpServer) run() error {
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

	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("stdin error: %v", err)
		return err
	}
	return nil
}

// JSON-RPC 2.0 types
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      *json.Number    `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      *json.Number `json:"id,omitempty"`
}

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

// handleRequest dispatches a JSON-RPC request
func (s *mcpServer) handleRequest(req *jsonRPCRequest) *jsonRPCResponse {
	if req.JSONRPC != "2.0" {
		return jsonRPCErrorResponse(req.ID, -32600, "Invalid Request")
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return jsonRPCErrorResponse(req.ID, -32601, "Method not found")
	}
}

// handleInitialize responds to the initialize request
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
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// handleToolsList lists available tools
func (s *mcpServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	tools := s.toolsList()
	result := map[string]interface{}{
		"tools": tools,
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// toolsList returns the list of available tools
func (s *mcpServer) toolsList() []Tool {
	return []Tool{
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
			Description: "Get an entity by ID",
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
			Name:        "find_entities",
			Description: "Search for entities by query",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"project", "query"},
			},
		},
		{
			Name:        "get_relations",
			Description: "Get relations of an entity",
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
			Name:        "get_text",
			Description: "Get a text field for an entity or type",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Entity or type ID",
					},
					"field": map[string]interface{}{
						"type":        "string",
						"description": "Field name",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": "Language code",
					},
				},
				"required": []string{"project", "id", "field", "language"},
			},
		},
		{
			Name:        "set_text",
			Description: "Set a text field for an entity",
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
						"description": "Field name",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Text value",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": "Language code",
					},
				},
				"required": []string{"project", "id", "field", "value", "language"},
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
			Description: "Add a new relation type",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Type name",
					},
					"visibility": map[string]interface{}{
						"type":        "string",
						"description": "visible or hidden",
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
						"description": "Visible or hidden",
					},
				},
				"required": []string{"project", "view", "type", "visible"},
			},
		},
		{
			Name:        "confirm_rename",
			Description: "Confirm an entity symbol or member rename",
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
						"description": "Field: symbol or via.member",
					},
					"newValue": map[string]interface{}{
						"type":        "string",
						"description": "New value",
					},
				},
				"required": []string{"project", "id", "field", "newValue"},
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
			Name:        "extract",
			Description: "Run extractors for the project",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"extractor": map[string]interface{}{
						"type":        "string",
						"description": "Extractor ID (optional)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "sync",
			Description: "Apply extractor facts to the registry",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"extractor": map[string]interface{}{
						"type":        "string",
						"description": "Extractor ID (optional)",
					},
					"dryRun": map[string]interface{}{
						"type":        "boolean",
						"description": "Report only, write nothing",
					},
					"noRenames": map[string]interface{}{
						"type":        "boolean",
						"description": "Do not treat as renames",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "place_entities",
			Description: "Place entities on a view (only with human request)",
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
					"entities": map[string]interface{}{
						"type":        "array",
						"description": "Entity placements",
					},
					"requestedByHuman": map[string]interface{}{
						"type":        "boolean",
						"description": "Must be true",
					},
				},
				"required": []string{"project", "view", "entities", "requestedByHuman"},
			},
		},
		{
			Name:        "doctor",
			Description: "Run diagnostic checks",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project ID (optional)",
					},
				},
				"required": []string{},
			},
		},
	}
}

// handleToolCall invokes a tool
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
		result = index
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
				result = p.Views
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
		var entities map[string]interface{}
		path := filepath.Join(s.workspace, "projects", params.Project, "entities.json")
		if data, err := os.ReadFile(path); err != nil {
			errMsg = fmt.Sprintf("Failed to load entities: %v", err)
		} else if err := json.Unmarshal(data, &entities); err != nil {
			errMsg = fmt.Sprintf("Failed to parse entities: %v", err)
		} else if ent, ok := entities[params.ID]; ok {
			result = ent
		} else {
			errMsg = "Entity not found"
		}
	case "find_entities":
		var params struct {
			Project string `json:"project"`
			Query   string `json:"query"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}
		var entities map[string]interface{}
		path := filepath.Join(s.workspace, "projects", params.Project, "entities.json")
		if data, err := os.ReadFile(path); err != nil {
			errMsg = fmt.Sprintf("Failed to load entities: %v", err)
		} else if err := json.Unmarshal(data, &entities); err != nil {
			errMsg = fmt.Sprintf("Failed to parse entities: %v", err)
		} else {
			var results []interface{}
			for id, ent := range entities {
				if strings.Contains(strings.ToLower(id), strings.ToLower(params.Query)) {
					results = append(results, map[string]interface{}{
						"id":   id,
						"data": ent,
					})
				}
			}
			result = results
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
		var relations map[string]interface{}
		path := filepath.Join(s.workspace, "projects", params.Project, "relations.json")
		if data, err := os.ReadFile(path); err != nil {
			errMsg = fmt.Sprintf("Failed to load relations: %v", err)
		} else if err := json.Unmarshal(data, &relations); err != nil {
			errMsg = fmt.Sprintf("Failed to parse relations: %v", err)
		} else {
			var results []interface{}
			for id, rel := range relations {
				relMap, _ := rel.(map[string]interface{})
				from, _ := relMap["from"].(string)
				to, _ := relMap["to"].(string)
				relType, _ := relMap["type"].(string)

				matchesEntity := (from == params.Entity && (params.Direction == "" || params.Direction == "out")) ||
					(to == params.Entity && (params.Direction == "" || params.Direction == "in"))
				matchesType := params.Type == "" || relType == params.Type

				if matchesEntity && matchesType {
					results = append(results, map[string]interface{}{
						"id":   id,
						"data": rel,
					})
				}
			}
			result = results
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
		var texts map[string]map[string]interface{}
		path := filepath.Join(s.workspace, "projects", params.Project, "text."+params.Language+".json")
		if data, err := os.ReadFile(path); err != nil {
			errMsg = fmt.Sprintf("Failed to load texts: %v", err)
		} else if err := json.Unmarshal(data, &texts); err != nil {
			errMsg = fmt.Sprintf("Failed to parse texts: %v", err)
		} else if textData, ok := texts[params.ID]; ok {
			if fieldData, ok := textData[params.Field]; ok {
				result = fieldData
			} else {
				errMsg = "Field not found"
			}
		} else {
			errMsg = "Entity not found"
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

		// Use core rules for validation and creation
		textVal := core.NewTextValue(params.Value, "authored")
		if err := core.ValidateTextValue(textVal); err != nil {
			errMsg = fmt.Sprintf("Invalid text value: %v", err)
		} else {
			path := filepath.Join(s.workspace, "projects", params.Project, "text."+params.Language+".json")
			var texts map[string]map[string]interface{}
			if data, err := os.ReadFile(path); err == nil {
				_ = json.Unmarshal(data, &texts)
			}
			if texts == nil {
				texts = make(map[string]map[string]interface{})
			}
			if texts[params.ID] == nil {
				texts[params.ID] = make(map[string]interface{})
			}

			texts[params.ID][params.Field] = map[string]interface{}{
				"v":      textVal.V,
				"origin": textVal.Origin,
				"at":     textVal.At,
			}

			if err := core.SaveTextJSON(path, texts); err != nil {
				errMsg = fmt.Sprintf("Failed to save text: %v", err)
			} else {
				result = map[string]interface{}{"status": "ok"}
			}
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

		// Use core rules for ID generation
		relID, err := core.RelationID(params.From, params.To, params.Type)
		if err != nil {
			errMsg = fmt.Sprintf("Invalid relation: %v", err)
		} else {
			path := filepath.Join(s.workspace, "projects", params.Project, "relations.json")
			var relations map[string]interface{}
			if data, err := os.ReadFile(path); err == nil {
				_ = json.Unmarshal(data, &relations)
			}
			if relations == nil {
				relations = make(map[string]interface{})
			}

			if _, exists := relations[relID]; !exists {
				relations[relID] = map[string]interface{}{
					"from":   params.From,
					"to":     params.To,
					"type":   params.Type,
					"origin": "authored",
					"at":     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				}
			}

			if err := core.SaveRelationsJSON(path, relations); err != nil {
				errMsg = fmt.Sprintf("Failed to save relation: %v", err)
			} else {
				result = map[string]interface{}{"status": "ok", "id": relID}
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

		visibility := params.Visibility
		if visibility == "" {
			visibility = "visible"
		}

		path := filepath.Join(s.workspace, "projects", params.Project, "relation-types.json")
		var types map[string]interface{}
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &types)
		}
		if types == nil {
			types = make(map[string]interface{})
		}

		if _, exists := types[params.Type]; !exists {
			types[params.Type] = map[string]interface{}{
				"visibility": visibility,
			}
		}

		data, _ := json.MarshalIndent(types, "", "  ")
		data = append(data, '\n')
		if err := os.WriteFile(path, data, 0644); err != nil {
			errMsg = fmt.Sprintf("Failed to save relation type: %v", err)
		} else {
			result = map[string]interface{}{"status": "ok"}
		}
	case "set_relation_visible":
		// Stub implementation
		result = map[string]interface{}{"status": "ok"}
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

		// Handle entity symbol rename
		if strings.HasPrefix(params.ID, "e_") && params.Field == "symbol" {
			path := filepath.Join(s.workspace, "projects", params.Project, "entities.json")
			var entities map[string]interface{}
			if data, err := os.ReadFile(path); err != nil || json.Unmarshal(data, &entities) != nil {
				errMsg = "Failed to load entities"
			} else if ent, ok := entities[params.ID].(map[string]interface{}); ok {
				ent["symbol"] = params.NewValue
				if err := core.SaveEntitiesJSON(path, entities); err != nil {
					errMsg = fmt.Sprintf("Failed to save entities: %v", err)
				} else {
					result = map[string]interface{}{"status": "ok"}
				}
			}
		}

		// Handle relation member rename
		if strings.HasPrefix(params.ID, "r_") && params.Field == "via.member" {
			path := filepath.Join(s.workspace, "projects", params.Project, "relations.json")
			var relations map[string]interface{}
			if data, err := os.ReadFile(path); err != nil || json.Unmarshal(data, &relations) != nil {
				errMsg = "Failed to load relations"
			} else if rel, ok := relations[params.ID].(map[string]interface{}); ok {
				if via, ok := rel["via"].(map[string]interface{}); ok {
					via["member"] = params.NewValue
				} else {
					rel["via"] = map[string]interface{}{"member": params.NewValue}
				}
				if err := core.SaveRelationsJSON(path, relations); err != nil {
					errMsg = fmt.Sprintf("Failed to save relations: %v", err)
				} else {
					result = map[string]interface{}{"status": "ok"}
				}
			}
		}

		if result == nil && errMsg == "" {
			result = map[string]interface{}{"status": "ok"}
		}
	case "sync_preview":
		var params struct {
			Project   string `json:"project"`
			Extractor string `json:"extractor"`
		}
		_ = json.Unmarshal(args.Arguments, &params)

		opt := core.SyncOptions{
			Project: params.Project,
			DryRun:  true,
		}
		report, err := core.Sync(s.workspace, nil, opt)
		if err != nil {
			errMsg = fmt.Sprintf("Sync preview failed: %v", err)
		} else {
			result = report
		}
	case "extract":
		// Stub - requires access to project file and extractor management
		errMsg = "Extract not yet implemented in MCP"
	case "sync":
		var params struct {
			Extractor  string `json:"extractor"`
			DryRun     bool   `json:"dryRun"`
			NoRenames  bool   `json:"noRenames"`
		}
		_ = json.Unmarshal(args.Arguments, &params)

		opt := core.SyncOptions{
			DryRun:    params.DryRun,
			NoRenames: params.NoRenames,
		}
		report, err := core.Sync(s.workspace, nil, opt)
		if err != nil {
			errMsg = fmt.Sprintf("Sync failed: %v", err)
		} else {
			result = report
		}
	case "place_entities":
		var params struct {
			Project          string        `json:"project"`
			View             string        `json:"view"`
			Entities         []interface{} `json:"entities"`
			RequestedByHuman bool          `json:"requestedByHuman"`
		}
		if err := json.Unmarshal(args.Arguments, &params); err != nil {
			return jsonRPCErrorResponse(req.ID, -32602, "Invalid params")
		}

		// Check if human requested this
		if err := core.CanWriteView(core.WriteViewOptions{RequestedByHuman: params.RequestedByHuman}); err != nil {
			errMsg = err.Error()
		} else {
			// Stub - would update view file with entity placements
			result = map[string]interface{}{"status": "ok"}
		}
	case "doctor":
		report := core.Check(s.workspace, s.sourceRoot)
		result = report
	default:
		return jsonRPCErrorResponse(req.ID, -32601, "Method not found")
	}

	if errMsg != "" {
		return jsonRPCErrorResponse(req.ID, -32000, errMsg)
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}
