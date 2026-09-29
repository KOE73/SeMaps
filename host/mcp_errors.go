package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// argumentsError is the start of the text the SDK gives a tool call whose
// arguments do not fit the tool's input schema (mcp.toolForErr).
const argumentsError = `validating "arguments":`

// explainArgumentErrors is a receiving middleware: a `tools/call` that fails
// input validation — `unexpected additional properties ["items"]`, a missing
// required field — gets the tool's accepted parameters appended to the SDK's
// message, e.g. «get_view accepts: view (required), project, lang». The
// caller, an agent that guessed a parameter name, learns the right ones from
// the error itself instead of by trial. The schema is read from the server's
// own tools/list, so it cannot drift from what the tool really takes; a tool
// whose schema cannot be read leaves the message as the SDK wrote it.
func explainArgumentErrors(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if method != "tools/call" || err != nil {
			return res, err
		}
		call, ok := res.(*mcp.CallToolResult)
		if !ok || !call.IsError || len(call.Content) == 0 {
			return res, err
		}
		text, ok := call.Content[0].(*mcp.TextContent)
		if !ok || !strings.HasPrefix(text.Text, argumentsError) {
			return res, err
		}
		cr, ok := req.(*mcp.CallToolRequest)
		if !ok || cr.Params == nil {
			return res, err
		}
		listed, lerr := next(ctx, "tools/list", &mcp.ListToolsRequest{Session: cr.Session, Params: &mcp.ListToolsParams{}})
		if lerr != nil {
			return res, err
		}
		if tools, ok := listed.(*mcp.ListToolsResult); ok {
			for _, t := range tools.Tools {
				if t.Name == cr.Params.Name {
					if accepted := acceptedParameters(t); accepted != "" {
						text.Text += "\n" + accepted
					}
					break
				}
			}
		}
		return res, err
	}
}

// acceptedParameters: «<tool> accepts: a (required), b, c» from the tool's
// input schema — required ones first, the rest alphabetical.
func acceptedParameters(t *mcp.Tool) string {
	raw, err := json.Marshal(t.InputSchema)
	if err != nil {
		return ""
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if json.Unmarshal(raw, &schema) != nil {
		return ""
	}
	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	if len(names) == 0 {
		return t.Name + " accepts no parameters"
	}
	for i, name := range names {
		if required[name] {
			names[i] = name + " (required)"
		}
	}
	return t.Name + " accepts: " + strings.Join(names, ", ")
}
