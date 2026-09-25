package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type hostInfo struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func hostEndpoint(proj project, workspace string) (string, string, error) {
	file := filepath.Join(proj.Root, ".semaps", "host.json")
	read := func() (string, string, bool) {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", "", false
		}
		var h hostInfo
		if json.Unmarshal(b, &h) != nil || h.Port == 0 || h.Key == "" {
			return "", "", false
		}
		base := fmt.Sprintf("http://localhost:%d", h.Port)
		c := http.Client{Timeout: 500 * time.Millisecond}
		res, err := c.Get(base + "/api/info")
		if err != nil {
			return "", "", false
		}
		defer res.Body.Close()
		var info struct{ Workspace string }
		if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&info) != nil || !strings.EqualFold(filepath.Clean(info.Workspace), filepath.Clean(workspace)) {
			return "", "", false
		}
		return base + "/mcp", h.Key, true
	}
	if endpoint, key, ok := read(); ok {
		return endpoint, key, nil
	}
	if err := startBackgroundHost(proj.File); err != nil {
		return "", "", err
	}
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if endpoint, key, ok := read(); ok {
			return endpoint, key, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", "", fmt.Errorf("host did not start within 15 seconds")
}

type bearerTransport struct {
	key  string
	next http.RoundTripper
}

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.key)
	return t.next.RoundTrip(r)
}

func runMCPProxy(proj project, workspace, projectID string) int {
	endpoint, key, err := hostEndpoint(proj, workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	remote := mcp.NewClient(&mcp.Implementation{Name: "semaps-stdio-proxy", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearerTransport{key: key, next: http.DefaultTransport}}, DisableStandaloneSSE: true}
	session, err := remote.Connect(context.Background(), transport, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	defer session.Close()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	local := mcp.NewServer(&mcp.Implementation{Name: "semaps", Version: "1"}, nil)
	for _, tool := range listed.Tools {
		tool := tool
		local.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			if projectID != "" && projectTool(tool.Name) {
				if args == nil {
					args = map[string]any{}
				}
				if _, ok := args["project"]; !ok {
					args["project"] = projectID
				}
			}
			return session.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: args})
		})
	}
	if err := local.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "semaps mcp:", err)
		return 1
	}
	return 0
}

func projectTool(name string) bool {
	switch name {
	case "list_views", "get_entity", "find_entities", "get_relations", "get_relation_types", "get_view", "get_text", "set_text", "add_relation", "add_relation_type", "set_relation_visible", "confirm_rename", "place_entities", "move_elements", "resize_elements", "set_zone", "add_zone", "fit_zone", "align_elements", "save", "discard":
		return true
	}
	return false
}
