package main

// Descriptions of the graph tools of both tool sets (`one` and `narrow`), at
// three levels of detail (`mcp.description`: brief | standard | full,
// PLAN_20260928-7 step 4). Written out in full as plain strings, not
// assembled from fragments, so a person can read and edit exactly what an
// agent will receive. Only the graph tools (get_graph, find_node,
// graph_formats, and the ten narrow tools) have levels; every other tool of
// the server (list_projects, get_entity, set_text, ...) keeps the single
// description given at mcp.AddTool in host/mcp.go.
//
// Rules the texts below follow (do not add to them without re-reading this
// comment): describe what exists NOW — no parameter that was removed
// (get_graph once named `set`, which is gone); no reference to files, ADRs
// or plans, since the agent reading these words works in another
// repository and cannot open ours; English.

// toolDescriptions holds the three texts of one tool.
type toolDescriptions struct {
	Brief    string
	Standard string
	Full     string
}

// descriptionFor picks the text of one tool at one level ("brief",
// "standard" or "full"); an unknown level falls back to "standard" — the
// same fallback mcpSettings.withDefaults applies to mcp.description.
func (d toolDescriptions) at(level string) string {
	switch level {
	case "brief":
		return d.Brief
	case "full":
		return d.Full
	default:
		return d.Standard
	}
}

// The block every "standard" and "full" description of a walk-shaped tool
// (get_graph and the ten narrow tools) repeats: how to read one line of a
// `facts` answer, and the traps the repeated agent experiment found.
const factsLineHelp = "A line of the default `facts` answer reads: " +
	"`<step> <name>  <file>:<lines>  [<relation> <member>:<line> ...] via <node>`. " +
	"`<step>` is how many hops from the focus (0 is the focus itself). " +
	"`<lines>` is `start-end`, or just `start` with no end. " +
	"Inside the brackets, a relation is named looking OUTWARD from the node it was reached from, not from the node the line is about " +
	"(so a line reached by `extends` says `extends`, even though it is the base type's own line, where you might expect `extended-by`) — " +
	"the same word can be handed back as `follow` to keep walking in that direction. " +
	"`@31,32` after a relation are the call or construct site lines; `in File.cs` after them names the file of those sites only when it differs from the file already printed on the line. " +
	"`via <node>` (from step 2 on) names the node(s) this line was reached through."

const trapsHelp = "Traps found in practice: " +
	"asking about a TYPE shows the calls made by its own methods, folded up to the type (`lift=types`, the default) — do not pass `lift=none` for a type, or its methods' calls disappear from the answer with no explanation; " +
	"reading or writing a property counts as a `calls` edge, the same as calling a method — nothing marks it apart; " +
	"an answer that was cut (by `limit`, `fanout` or `list_cap`) says so in its FIRST line, so check the first line before trusting a count; " +
	"`fanout` cuts neighbours per node per relation and what is cut does not come back in a later part of the same answer — ask again with a larger `fanout`; " +
	"a plain name is enough for `around` (`find_node`'s id is not required) — an ambiguous name lists candidates, an unresolved one lists near matches; " +
	"a method carrying a `{dynamic: create @57}` mark means something happens at that line the graph cannot see (reflection-based creation or invocation) — read the code at that line, the graph has no edge for it."

var graphToolDescriptions = map[string]toolDescriptions{
	"get_graph": {
		Brief: "The live code graph: types, methods, and how they relate (extends, holds, calls, constructs, contains, ...), joined with the code position (file:lines) of each. " +
			"Bound the answer with `around` (a node name) and `depth`, or with `container`, or it is cut to a node limit. " +
			"Example: `get_graph {\"around\": \"OrderService\", \"depth\": 1}` — OrderService and its direct neighbours.",
		Standard: "get_graph answers questions about the structure of the code: what a type extends or implements, what it holds or is held by, what calls or is called, what constructs or is created, what a namespace/assembly contains, where something lies. " +
			"Main parameters: `around` (a node name or id — the neighbourhood to walk; omit for the whole graph), `depth` (1-5 hops from `around`, default 1), `follow` (which directed relations to walk outward from `around`; default: everything but containment — call graph_formats for the full list), `level` (`types` to drop method/function/value nodes, `all` to keep them; default `types` without `around`, `all` with it), `lift` (`types` folds method-level edges up to their containing type, default when `around` is a type; `none` keeps them on the methods, default when `around` is a method), `container` (restrict to one containers.json id and its descendants), `fields` (extra data per node: `members`, `via`, `position`, `memberLines`), `format` (answer shape; default `facts`), `limit`/`listCap` (how much an unbounded answer is cut to). " +
			"By default the answer is text in the `facts` format. " + factsLineHelp + " " +
			"Examples, question → call: " +
			"who extends X two levels deep → `{\"around\":\"X\",\"follow\":[\"extended-by\"],\"depth\":2}`; " +
			"who holds X and through which member → `{\"around\":\"X\",\"follow\":[\"held-by\"]}` (the member name and its declaration line are on each relation); " +
			"who calls the methods of X → `{\"around\":\"X\",\"follow\":[\"called-by\"]}`; " +
			"what a method constructs and calls → `{\"around\":\"X.Method\",\"follow\":[\"constructs\",\"calls\"],\"lift\":\"none\"}`; " +
			"where is X created → `{\"around\":\"X\",\"follow\":[\"constructed-by\"]}`; " +
			"what is inside a namespace and how many → `{\"around\":\"App.Services\",\"follow\":[\"contains\"]}` (the trailing counts line gives the type/method totals); " +
			"in which namespace and assembly does X lie → `{\"around\":\"X\"}` (the node's own line carries `in <namespace> (assembly <assembly>)`). " +
			trapsHelp,
		Full: "get_graph answers questions about the structure of the code: what a type extends or implements, what it holds or is held by, what calls or is called, what constructs or is created, what a namespace/assembly contains, where something lies. " +
			"Main parameters: `around` (a node name or id — the neighbourhood to walk; omit for the whole graph), `depth` (1-5 hops from `around`, default 1), `follow` (which directed relations to walk outward from `around`; default: everything but containment), `level` (`types` to drop method/function/value nodes, `all` to keep them; default `types` without `around`, `all` with it), `kinds` (whole-graph request only: comma list of edge kinds to keep), `lift` (`types` folds method-level edges up to their containing type, default when `around` is a type or omitted with level=types; `none` keeps them on the methods, default when `around` is a method), `container` (restrict to one containers.json id and its descendants), `fields` (list; `members`, `via`, `position`, `memberLines`; omitted/null is the default `via,position`, `[]` is none), `missing` (include model-only nodes/edges whose entity/relation has status missing; default false), `limit` (cut an unbounded answer to this many nodes; default from settings, 200 unless changed), `listCap` (cut names/lines printed for one relation before '+N'; default from settings, 50 unless changed), `format` (a registered answer shape, or call graph_formats; default from settings, `facts` unless changed), `template` (a template of your own instead of a named format's). " +
			"By default the answer is text in the `facts` format. " + factsLineHelp + " " +
			"Examples, question → call: " +
			"who extends X two levels deep → `{\"around\":\"X\",\"follow\":[\"extended-by\"],\"depth\":2}`; " +
			"who holds X and through which member → `{\"around\":\"X\",\"follow\":[\"held-by\"]}`; " +
			"who calls the methods of X → `{\"around\":\"X\",\"follow\":[\"called-by\"]}`; " +
			"what a method constructs and calls → `{\"around\":\"X.Method\",\"follow\":[\"constructs\",\"calls\"],\"lift\":\"none\"}`; " +
			"where is X created → `{\"around\":\"X\",\"follow\":[\"constructed-by\"]}`; " +
			"what is inside a namespace and how many → `{\"around\":\"App.Services\",\"follow\":[\"contains\"]}`; " +
			"in which namespace and assembly does X lie → `{\"around\":\"X\"}`. " +
			trapsHelp + " " +
			"Relation vocabulary (name / inverse): extends / extended-by (a type extends its base type); implements / implemented-by (a type or method implements an interface or interface method); overrides / overridden-by (a method overrides a base virtual method); holds / held-by (a field/property/parameter holds a value of the target type, any cardinality; holds.many / held-by.many narrows to cardinality many); uses / used-by (a member's type refers to the target other than by holding or injecting it); injects / injected-into (a constructor parameter injects the target type); calls / called-by (a method calls another, or reads/writes a property); constructs / constructed-by (a method constructs the target type with `new`); depends / depended-on-by (a module-level dependency); contains / inside (a namespace/assembly/type contains a member). " +
			"`lift`: `types` merges every method-level edge between the methods of two types into one type-level relation, adding `×N`, `fromMethods`, `toMethods` on it (capped at 5 names then '+N'); `none` leaves edges on the methods that made them, which is required to see a specific method's own calls/constructs. " +
			"`fanout` (walk parameter, alongside `depth`): caps how many neighbours one node contributes per relation per step; 0 (default) is unlimited; a cut is reported in the answer's first line and does not come back in the same answer. " +
			"`container`/`limit`/`listCap` as above. " +
			"Answer formats: `facts` (compact text, one line per node, the default), `lines` (text, one line per relation), `tree` (text, indented walk from `around`, requires `around`), `locations` (tab-separated file:line rows), `json` and `json-compact` (structured, also returned as structuredContent). Call graph_formats for the exact list with each format's own template and the full relation table. " +
			"Name resolution (`around` as a name, not an id): resolved by id first, then by name, case-insensitively as a last resort; an ambiguous name answers `{\"error\":\"ambiguous\",\"candidates\":[...]}` instead of picking one; an unresolved name is a tool error naming the nearest matches; a name that only resolves case-insensitively, or through a method's short signature rather than its id, adds a one-line notice at the top of the answer saying so. A node hidden because its entity/relation has status `missing` needs `missing:true` to be reachable at all. " +
			"Template language (`template`, in place of `format`): literal text is copied as is, `{macro}` is replaced by that macro's value or nothing when absent, `[...]` is an optional group dropped whole (with its literal text) when any macro or nested `{relations:...}` block directly inside it is empty, `\\[`/`\\]` escape a literal bracket, and a run of plain spaces at the very start or end of the rendered template is dropped (a run with real content on both sides is kept). `{relations: TEMPLATE | SEPARATOR}` renders TEMPLATE once per relation reaching the node, joined by SEPARATOR. Node macros: step, name, fullName, id, kind, nativeKind, visibility, file, line, endLine, lines, namespace, assembly, containers, presence, status, via, viaFullName. Relation macros (inside a relations block only): relation, member, memberKind, memberLine, modifiers, cardinality, type, text, relationLine, relationLines, relationLinesFile, count, fromMethods, toMethods, injected. `{memberLine}` is where a held/used/injected member is declared; `{relationLine}`/`{relationLines}` are call/construct site line(s) — never both set for the same edge kind. `{count}`/`{fromMethods}`/`{toMethods}` are set only on a lifted relation. `{injected}` is the literal text `(injected)` when the same member's holds and injects were folded into one relation line. A malformed template (unmatched `{`, `[` or `]`, or an unknown macro) is a tool error naming the position and the macro.",
	},
	"find_node": {
		Brief: "Substring search over graph node names and ids, case-insensitive — candidates to pass as `around` (get_graph) or `name` (the narrow tools) when you are unsure of the exact spelling. " +
			"Example: `find_node {\"q\": \"runner\"}`.",
		Standard: "find_node answers \"what is this thing actually called in the graph\": a substring search over every node's name and id, case-insensitive. " +
			"Parameters: `q` (the substring, required), `limit` (default 50). " +
			"The answer is `{candidates: [{id, kind, file?, line?}]}` — pass any candidate's name (or id) straight into `around`/`name`. " +
			"In practice a plain name is usually enough for `around`/`name` directly, without calling find_node first — it is here for the cases where the exact name is not known.",
		Full: "find_node answers \"what is this thing actually called in the graph\": a substring search over every node's name and id, case-insensitive. " +
			"Parameters: `q` (the substring, required), `limit` (default 50). " +
			"The answer is `{candidates: [{id, kind, file?, line?}]}` — pass any candidate's name (or id) straight into `around`/`name`. " +
			"In practice a plain name is usually enough for `around`/`name` directly (name resolution, described under get_graph/the narrow tools, tries an exact match before falling back to a case-insensitive one and reports which it used); find_node is for the remaining cases — an unfamiliar codebase, a typo, or several candidates sharing a short name.",
	},
	"graph_formats": {
		Brief: "The graph's answer formats, the relation vocabulary `follow` accepts, the template macro dictionary, and the current defaults. " +
			"Example: `graph_formats {}`.",
		Standard: "graph_formats lists what get_graph (and the narrow walk tools) can be asked for: the registered answer formats (`facts`, `lines`, `tree`, `locations`, `json`, `json-compact`) with their own template where they have one, the full relation vocabulary (name, inverse, what it means), the default `follow` list, the template macro rules, and the current defaults (default format, default level for a whole-graph vs. a neighbourhood request). " +
			"Call it with no arguments; read its result before writing a `template` of your own, or before assuming a relation name that is not in the vocabulary.",
		Full: "graph_formats lists what get_graph (and the narrow walk tools) can be asked for: the registered answer formats (`facts`, `lines`, `tree`, `locations`, `json`, `json-compact`) with their own template where they have one, the full relation vocabulary (name, inverse, kind, type-match narrowing, description), the default `follow` list, the template macro rules and worked examples (the same rules given in full under get_graph's own description at this level), and the current defaults (default format, default level for a whole-graph vs. a neighbourhood request). " +
			"Call it with no arguments; its result is the authoritative list — prefer it over guessing a relation or format name.",
	},
}

// narrowTool describes one entry of the `narrow` tool set: its fixed
// `follow` (host/mcp.go registers the wrapper with these names) and its
// three description levels. `verb`/`example` feed the shared text below so
// each tool's wording only has to say what is particular to it.
type narrowTool struct {
	Follow  []string
	Answers string // what question this tool answers, for brief/standard
	Example string // example value for `name` in the one example call
}

var narrowTools = map[string]narrowTool{
	"who_extends":     {Follow: []string{"extended-by", "implemented-by"}, Answers: "what extends or implements X", Example: "Middleware"},
	"what_it_extends": {Follow: []string{"extends", "implements"}, Answers: "what X extends or implements", Example: "RepetitionGuard"},
	"who_holds":       {Follow: []string{"held-by", "injected-into"}, Answers: "what holds or injects X, and through which member", Example: "Context"},
	"what_it_holds":   {Follow: []string{"holds", "injects"}, Answers: "what X holds or injects, and through which member", Example: "Runner"},
	"who_calls":       {Follow: []string{"called-by"}, Answers: "what calls the methods of X", Example: "Factory"},
	"what_it_calls":   {Follow: []string{"calls", "constructs"}, Answers: "what X's methods call or construct", Example: "OnnxModel"},
	"where_created":   {Follow: []string{"constructed-by"}, Answers: "where X is constructed with `new`", Example: "ImageRunner"},
	"what_is_inside":  {Follow: []string{"contains"}, Answers: "what a namespace, assembly or type contains, and how many", Example: "App.Services"},
	"where_it_lies":   {Follow: []string{"inside"}, Answers: "which namespace/assembly/type X lies in", Example: "OrderService"},
}

// narrowDescriptions builds the toolDescriptions of one narrow tool from its
// narrowTool entry, so the ten tools stay in one shape and the wording that
// is the same for all of them (parameters, how to read a line, the traps)
// is written once here, not ten times over.
func narrowDescriptions(name string, t narrowTool) toolDescriptions {
	brief := t.Answers + ", one hop by default. " +
		"Example: `" + name + " {\"name\": \"" + t.Example + "\"}`."
	standard := "Answers " + t.Answers + " — the same graph walk as get_graph with `follow` fixed to this tool's relation(s), so you do not have to name them. " +
		"Parameters: `name` (required — a node name, or an id from find_node), `depth` (hops from `name`, default 1), `project` (as the other tools, default the only project or --project). " +
		"The answer is text in the default `facts` format (the .semaps mcp.format setting), same shape and defaults (lift, fanout, limit, listCap) as get_graph. " + factsLineHelp + " " +
		"Example: `" + name + " {\"name\": \"" + t.Example + "\"}` — " + t.Answers + ", one hop. " +
		trapsHelp
	full := standard + " " +
		"This tool is a thin wrapper: it calls the same graph walk get_graph uses, with its relation list fixed to " + followList(t.Follow) + " and every other get_graph default (level, lift, container handling, fields, limit, listCap, format) unchanged — a get_graph call naming `name` as its neighbourhood and the same relation list gives the identical answer. " +
		"For the full relation vocabulary, the template language, and every other format, call graph_formats or use get_graph directly with a relation list of your own."
	return toolDescriptions{Brief: brief, Standard: standard, Full: full}
}

func followList(follow []string) string {
	if len(follow) == 1 {
		return "`[\"" + follow[0] + "\"]`"
	}
	out := "["
	for i, f := range follow {
		if i > 0 {
			out += ", "
		}
		out += "\"" + f + "\""
	}
	return "`" + out + "]`"
}
