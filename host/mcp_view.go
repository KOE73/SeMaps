package main

// The MCP tools for working on a view beyond moving boxes: layout_guide (how
// the canvas works and how to use the geometry tools), render_view (a picture
// of a view from the open editor), create_view and create_project. The
// geometry tools themselves are in mcp.go; the numbers of the canvas come from
// canvas.json through core.Canvas and are never typed twice.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"semaps/core"
)

// serverPreamble opens the server Instructions.
const serverPreamble = "SeMaps registry of this repository. Read with list_*/get_*/find_*; write only through these tools. " +
	"Nothing can be deleted; view geometry only with requestedByHuman when a human asked. " +
	"Work on a view only through the tools, never by editing files; call `layout_guide` before you move, resize or place anything, and check the result with `get_view` and `render_view`. " +
	"`create_view` and `create_project` only when a human explicitly asked for a new view or project. "

// viewToolDescriptions: the three description levels (mcp.description) of the
// view tools that are not graph tools. Same rules as mcp_descriptions.go: what
// exists now, no reference to files or repository documents, English.
var viewToolDescriptions = map[string]toolDescriptions{
	"layout_guide": {
		Brief: "How to work on a view: the canvas numbers, what each view tool does, the order of work. Read it before any geometry. No parameters.",
		Standard: "layout_guide returns a Markdown guide for working on a view: how to look at one (`get_view`, `render_view`), what each geometry tool does, the workflow " +
			"(look at the sample, decide, apply in steps, check, say what is unsaved and give the link), when `requestedByHuman` is needed, and the numbers of the canvas " +
			"(units, grid, default and minimum sizes, caption strip, padding, gaps). Call it once before moving, resizing or placing anything. No parameters; it does not depend on the project.",
		Full: "layout_guide returns a Markdown guide for working on a view: how to look at one (`get_view`, `render_view`), what each geometry tool does " +
			"(`move_elements`, `resize_elements`, `set_parent`, `add_container`, `fit_container`, `align_elements`, `place_entities`, `create_view`, `create_project`, `save`, `discard`, `set_text`, `add_entity`, `get_kinds`), " +
			"the workflow (look at the sample, decide, apply in steps, check with `get_view` or `render_view`, say what is unsaved and give the link), when `requestedByHuman` is needed, " +
			"that the host does not snap to the grid, and the numbers of the canvas (units, grid, default and minimum sizes, caption strip, padding, gaps) — the same block that heads every `get_view` answer. " +
			"Call it once before moving, resizing or placing anything. No parameters; it does not depend on the project.",
	},
	"render_view": {
		Brief: "A PNG of a view, a container or a rectangle from the editor open on it, plus the problems the editor finds (overlaps, clipped captions, lines through boxes). Needs the view open in an editor.",
		Standard: "render_view asks the editor that has the project open to draw a view, a container (`ref` like `v_main#e_core`) or a rectangle (`rect`: x, y, width, height in model units) and returns the picture with a list of problems: " +
			"overlaps, clipped captions and rows, lines through boxes, crossing lines, blocks outside their container. It shows the editor's current, unsaved state. " +
			"`scale` (0.25 to 4, default 1) and `maxSize` (pixels of the longer side, default 1600, at most 4096) bound the picture. Without an editor open on the project it fails and says which link to open.",
		Full: "render_view asks the editor that has the project open to draw a view, a container (`ref` like `v_main#e_core`) or a rectangle (`rect`: x, y, width, height in model units) and returns the picture with a list of problems: " +
			"overlap, clipped-caption, clipped-rows, line-through-box, line-crossing, outside-container — each with the ids involved. The problems come from the editor's real router and text measurement, so they match what the human sees. " +
			"It shows the editor's current, unsaved state, including your own unsaved changes. `view` is the view id (or give `ref`); `scale` (0.25 to 4, default 1) and `maxSize` (pixels of the longer side, default 1600, at most 4096) bound the picture. " +
			"The answer states the model rectangle that was drawn. Without an editor open on the project it fails and says which link to open; if the editor does not answer within 20 seconds it fails too — check that the view is open.",
	},
	"create_view": {
		Brief: "Create a new, empty view. Only when a human explicitly asked for a new view; requestedByHuman.",
		Standard: "create_view adds a new empty view to a project: `id` (v_ and lowercase letters, digits, underscore), `name` (its caption, in `lang`, default the project's first language), " +
			"`axis` (default: the project's default axis). Only when a human explicitly asked for a new view: requestedByHuman must be true. The view is written at once and the editor lists it; its name stays unsaved until `save`. " +
			"Refused while the project has unsaved changes. The answer gives the link.",
		Full: "create_view adds a new empty view to a project: `project`, `id` (v_ and lowercase letters, digits, underscore; refused when taken), `name` (its caption, written in `lang`, default the project's first language; " +
			"`names` gives several languages at once, language code to text), `axis` (default: the project's default axis, which the view then inherits; refused when neither is given), `icon`, `theme`, " +
			"`setDefault` (also make it the project's default view). Only when a human explicitly asked for a new view: requestedByHuman must be true. A view is a file of the project, so it is written at once " +
			"and every open editor lists it; the caption is an ordinary unsaved change of the shared model — `save` writes it, `discard` drops it. Refused while the project has unsaved changes: save or discard them first. The answer gives the link.",
	},
	"create_project": {
		Brief: "Create a new project in the workspace. Only when a human explicitly asked for a new project; requestedByHuman.",
		Standard: "create_project creates an empty project in the workspace: `id` (lowercase letters, digits, underscore, starting with a letter; refused when taken), `title`, `language` (default ru). " +
			"Only when a human explicitly asked for a new project: requestedByHuman must be true. Returns the project id.",
		Full: "create_project creates an empty project in the workspace: `id` (lowercase letters, digits, underscore, starting with a letter; refused when taken), `title` (default: the id), `subtitle`, `defaultAxis` (the axis of views that declare none), " +
			"`language` (the project's first text language, default ru), `icon`, `theme`. " +
			"Only when a human explicitly asked for a new project: requestedByHuman must be true. A project is a folder of the workspace, not a change of another project's model, so it is written at once, not held unsaved; " +
			"the answer says so and returns the project id. Add views to it with `create_view`.",
	},
}

// canvasNow is the canvas of this server's workspace, for tools that do not
// hold a model (layout_guide).
func (s *mcpServer) canvasNow() core.Canvas { return loadCanvas(s.workspace) }

type layoutGuideIn struct{}

func (s *mcpServer) layoutGuide(_ context.Context, _ *mcp.CallToolRequest, _ layoutGuideIn) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: layoutGuideText(s.canvasNow())}}}, nil, nil
}

// layoutGuideText is the Markdown of layout_guide. No file format anywhere in
// it: an agent works on a view only through the tools.
func layoutGuideText(c core.Canvas) string {
	g := fmt.Sprintf("%g", c.Grid)
	var b strings.Builder
	b.WriteString(`# Working on a view

Everything you do to a view goes through the tools below — never by editing files. An accepted step is
applied to the model the editor shares, so it shows up in an open editor at once, unsaved and marked as
yours; the human reviews and saves.

## Seeing

- ` + "`get_view`" + ` — the view as data: placements, containers with what lies in them (children), absolute rectangles, visible lines, what is unsaved.
  A container reference (` + "`v_main#e_core`" + `) reads only that subtree. Every answer starts with the canvas block below.
- ` + "`get_kinds`" + ` — the dictionary: which kinds exist, which of them are containers (a placement of an entity of such a kind is a frame that holds other placements).
- ` + "`render_view`" + ` — a picture (PNG) of the view, a container or a rectangle from the editor open on it, and a list of problems
  (overlaps, clipped captions, lines through boxes, crossing lines, blocks outside their container). It needs the view open in an
  editor and shows the editor's current, unsaved state. Look at it after each group of steps.
- The human points at objects with references: ` + "`<view>#<id>`" + ` (` + "`v_ops#e_undistort`" + ` for a container, ` + "`v_ops#e_op_crop`" + ` for a block),
  ` + "`<project>/<view>#<id>`" + ` when the workspace has several projects; several are joined by commas. Every tool takes them wherever it
  takes an object of a view. A view opens in the editor at ` + "`/app/#<view>?highlight=<id>`" + `.

## The canvas

`)
	b.WriteString(c.Describe())
	fmt.Fprintf(&b, `
Membership is explicit: a block is in a container because it was put there (`+"`set_parent`"+`, the `+"`parent`"+` of its placement), not because it happens to lie inside its rectangle.
A block inside a container still has absolute coordinates. A container is an entity of a container kind (`+"`get_kinds`"+`); `+"`add_container`"+` makes the entity and its frame in one step. Sizes do not follow content: text is not wrapped, a caption longer than its box runs
past the edge, and the rows of a box that lists members simply continue past its bottom if the box is short (roughly 55 + 15 per row + 3 per
section high) — give such boxes room. Lines are not stored: they are routed from the boxes on every repaint; orthogonal lines keep 8 units off
boxes and settle 16 to 24 units apart, so a gap between two boxes narrower than about 16 is not a corridor for a line. A box straight above another
blocks the straight line from the lower one.

## Tools

| To… | Use |
|---|---|
| shift things; a container goes with everything in it | `+"`move_elements`"+` (dx/dy, or x/y for the top-left corner of the common box) |
| change a size (never under the minimum, and a container never under its content) | `+"`resize_elements`"+` |
| put blocks or containers into a container, coordinates untouched | `+"`set_parent`"+` (parent null takes them out) |
| add a container: an entity of a container kind (existing, or new: name and kind, default group) with a rectangle, optional parent, style | `+"`add_container`"+` |
| make a container as tight as its content (caption strip and padding), ancestors grow | `+"`fit_container`"+` |
| line elements up on the first one: left, right, top, bottom, width, height | `+"`align_elements`"+` |
| put entities on a view | `+"`place_entities`"+` |
| an entity no extractor reports (an app, an external system, a database), so that it can be placed | `+"`add_entity`"+`, then `+"`place_entities`"+` |
| caption a view (a text under its id, field name), in every language of the project; a container is captioned by the name of its entity | `+"`set_text`"+` |
| a new view or project — only when a human explicitly asked | `+"`create_view`"+`, `+"`create_project`"+` |
| write what is unsaved / drop it — only when a human explicitly asked | `+"`save`"+`, `+"`discard`"+` |

Each step is one batch: it applies whole or not at all. Geometry you were not asked about stays as it is. There is no tool that lays things out
"like another container": which block stands for which is your judgement; the tools give you eyes (get_view, render_view) and precise hands.

## requestedByHuman

Every geometry tool, `+"`place_entities`"+`, `+"`create_view`"+`, `+"`create_project`"+`, `+"`save`"+` and `+"`discard`"+` refuses unless `+"`requestedByHuman: true`"+`, which you
pass only when a person asked for exactly this. Without such a request do not write geometry at all.

## Grid: the host does not snap

The editor snaps the human's own moves to the grid step (%s); this host does not — a coordinate or size you give is written as given.
Put positions and sizes on multiples of %s yourself, and leave the default gaps between what you place: a row of three default nodes is
3 x %g + 2 x %g = %g wide before any container padding.

## Workflow

1. Look at the sample: `+"`get_view`"+` (and `+"`render_view`"+`) on the reference and on the target — containers, blocks, rectangles.
2. Decide which block stands for which and work out the moves and sizes yourself; say the plan to the person.
3. Apply it in steps (`+"`move_elements`"+`, `+"`resize_elements`"+`, `+"`set_parent`"+`, `+"`fit_container`"+`), fewest steps first.
4. Check after each group: `+"`get_view`"+` for numbers, `+"`render_view`"+` for what a human sees and for problems; fix what it lists.
5. Say what is unsaved — "done, not saved, please check" — and give the link from the last tool answer. Save only if the person asked.
`, g, g, c.Node.Width, c.Gap.Node, 3*c.Node.Width+2*c.Gap.Node)
	return b.String()
}

// ---------------------------------------------------------------- render_view

type renderViewIn struct {
	Project string     `json:"project,omitempty"`
	View    string     `json:"view,omitempty" jsonschema:"view id; may be left out when ref is given"`
	Ref     string     `json:"ref,omitempty" jsonschema:"a view reference view#e_x (a container reference draws that container)"`
	Rect    *core.Rect `json:"rect,omitempty" jsonschema:"rectangle to draw in model units: x, y, width, height"`
	Scale   float64    `json:"scale,omitempty" jsonschema:"0.25 to 4; default 1"`
	MaxSize int        `json:"maxSize,omitempty" jsonschema:"pixels of the longer side; default 1600, at most 4096"`
}

func (s *mcpServer) renderView(_ context.Context, _ *mcp.CallToolRequest, in renderViewIn) (*mcp.CallToolResult, any, error) {
	if s.models == nil {
		return nil, nil, errors.New("render_view needs the running host with an editor")
	}
	target := in.Ref
	if target == "" {
		target = in.View
	}
	if target == "" {
		return nil, nil, errors.New("give view or ref")
	}
	scale := in.Scale
	if scale == 0 {
		scale = 1
	}
	if scale < 0.25 || scale > 4 {
		return nil, nil, fmt.Errorf("scale %g: between 0.25 and 4", scale)
	}
	maxSize := in.MaxSize
	if maxSize <= 0 {
		maxSize = 1600
	}
	maxSize = min(maxSize, 4096)
	if in.Rect != nil && (in.Rect.Width <= 0 || in.Rect.Height <= 0) {
		return nil, nil, errors.New("rect: width and height must be positive")
	}
	m, err := s.modelOfRef(in.Project, target)
	if err != nil {
		return nil, nil, err
	}
	r, err := core.ParseRef(target)
	if err != nil {
		return nil, nil, err
	}
	if in.View != "" && in.Ref != "" && in.View != r.View {
		return nil, nil, fmt.Errorf("view %s and ref %s name different views", in.View, in.Ref)
	}
	info, err := m.GetView(target)
	if err != nil {
		return nil, nil, err
	}
	link := "/app/#" + r.View
	ans, err := s.models.render(m.ProjectID(), renderRequest{View: r.View, Ref: in.Ref, Rect: in.Rect, Scale: scale, MaxSize: maxSize})
	switch {
	case errors.Is(err, errNoRenderClient):
		return nil, nil, fmt.Errorf("no editor is open on this project: open the view in the editor (%s) and repeat", link)
	case err != nil:
		return nil, nil, fmt.Errorf("%w (view %s: %s)", err, r.View, link)
	}
	png, err := base64.StdEncoding.DecodeString(ans.PNG)
	if err != nil || len(png) == 0 {
		return nil, nil, errors.New("the editor sent no usable picture")
	}
	var text strings.Builder
	rect := viewBounds(info)
	what := "the content of " + target + " (the editor may add a margin)"
	if in.Rect != nil {
		rect, what = *in.Rect, "the requested rectangle"
	}
	fmt.Fprintf(&text, "Rendered %s: x=%g y=%g width=%g height=%g in model units, image %dx%d px. This is the editor's current, unsaved state.\n",
		what, rect.X, rect.Y, rect.Width, rect.Height, ans.Width, ans.Height)
	if len(ans.Problems) == 0 {
		text.WriteString("no problems found\n")
	}
	for _, p := range ans.Problems {
		fmt.Fprintf(&text, "%s: %s", p.Kind, p.Text)
		if len(p.IDs) > 0 {
			fmt.Fprintf(&text, " (%s)", strings.Join(p.IDs, ", "))
		}
		text.WriteString("\n")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{Data: png, MIMEType: "image/png"},
		&mcp.TextContent{Text: text.String()},
	}}, nil, nil
}

// viewBounds is the box around every placement of what get_view returned.
func viewBounds(v core.ViewInfo) core.Rect {
	var box core.Rect
	found := false
	var walk func(p *core.PlacementInfo)
	walk = func(p *core.PlacementInfo) {
		if found {
			box = box.Union(p.Rect)
		} else {
			box, found = p.Rect, true
		}
		for _, c := range p.Children {
			walk(c)
		}
	}
	for _, p := range v.Placements {
		walk(p)
	}
	if !found {
		return core.Rect{}
	}
	return core.Rect{X: math.Floor(box.X), Y: math.Floor(box.Y), Width: math.Ceil(box.Width), Height: math.Ceil(box.Height)}
}

// ------------------------------------------------- create_view, create_project

type createViewIn struct {
	Project          string            `json:"project,omitempty"`
	ID               string            `json:"id" jsonschema:"v_ and lowercase letters, digits, underscore"`
	Name             string            `json:"name,omitempty" jsonschema:"the view's caption, written in lang"`
	Lang             string            `json:"lang,omitempty" jsonschema:"language of name; default: the project's first language"`
	Names            map[string]string `json:"names,omitempty" jsonschema:"captions in several languages: language code to text"`
	Axis             string            `json:"axis,omitempty" jsonschema:"CONTRACT §8.1; may be left out when the project has a default axis, which the view then inherits"`
	Icon             string            `json:"icon,omitempty"`
	Theme            string            `json:"theme,omitempty"`
	SetDefault       bool              `json:"setDefault,omitempty" jsonschema:"also make it the project's default view"`
	RequestedByHuman bool              `json:"requestedByHuman" jsonschema:"true only when a human explicitly asked for a new view"`
}

// service is the host's model service; without a host (tests) a private one
// over the same workspace, so every path runs the same code.
func (s *mcpServer) service() *modelService {
	if s.models == nil {
		s.models, _ = newModelService(s.workspace)
	}
	return s.models
}

// createView and createProject are the editor's create path (POST /api/projects,
// POST /api/model/{project}/views, ADR_20260926) with the agent's policy in front:
// only on a human's request.
func (s *mcpServer) createView(_ context.Context, _ *mcp.CallToolRequest, in createViewIn) (*mcp.CallToolResult, any, error) {
	if !in.RequestedByHuman {
		return nil, nil, errors.New("create_view requires requestedByHuman: true, and only when a human explicitly asked for a new view")
	}
	dir, err := core.ProjectDir(s.workspace, s.pick(in.Project))
	if err != nil {
		return nil, nil, err
	}
	project := filepath.Base(dir)
	names := map[string]string{}
	for lang, name := range in.Names {
		names[lang] = name
	}
	if in.Name != "" {
		names[in.Lang] = in.Name // an empty language is the project's first: the service resolves it
	}
	v := core.NewView{ID: in.ID, Axis: in.Axis, Icon: in.Icon, Theme: in.Theme, SetDefault: in.SetDefault}
	if err := s.service().createView(project, v, names, "agent"); err != nil {
		return nil, nil, err
	}
	link := "/app/#" + url.PathEscape(in.ID)
	text := fmt.Sprintf("view %s created in project %s and written at once: %s", in.ID, project, link)
	unsaved := false
	for _, name := range names {
		unsaved = unsaved || strings.TrimSpace(name) != ""
	}
	if unsaved {
		text = fmt.Sprintf("view %s created in project %s; its name is not saved, review and Save: %s", in.ID, project, link)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}},
		map[string]any{"view": in.ID, "project": project, "saved": !unsaved, "link": link}, nil
}

type createProjectIn struct {
	ID               string `json:"id" jsonschema:"lowercase letters, digits, underscore, starting with a letter"`
	Title            string `json:"title,omitempty" jsonschema:"shown in the catalog, not translated; default: the id"`
	Subtitle         string `json:"subtitle,omitempty"`
	DefaultAxis      string `json:"defaultAxis,omitempty" jsonschema:"axis of the views that declare none (CONTRACT §8.1)"`
	Language         string `json:"language,omitempty" jsonschema:"language of the project's texts: ru, en...; default ru"`
	Icon             string `json:"icon,omitempty"`
	Theme            string `json:"theme,omitempty"`
	RequestedByHuman bool   `json:"requestedByHuman" jsonschema:"true only when a human explicitly asked for a new project"`
}

func (s *mcpServer) createProject(_ context.Context, _ *mcp.CallToolRequest, in createProjectIn) (*mcp.CallToolResult, any, error) {
	if !in.RequestedByHuman {
		return nil, nil, errors.New("create_project requires requestedByHuman: true, and only when a human explicitly asked for a new project")
	}
	p := core.NewProject{ID: in.ID, Title: orStr(in.Title, in.ID), Subtitle: in.Subtitle, DefaultAxis: in.DefaultAxis,
		Language: in.Language, Icon: in.Icon, Theme: in.Theme}
	if err := s.service().createProject(p); err != nil {
		return nil, nil, err
	}
	text := fmt.Sprintf("project %s created and written to the workspace at once (a project is a folder, not an unsaved change); add a view with create_view", in.ID)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}},
		map[string]any{"project": in.ID, "saved": true}, nil
}
