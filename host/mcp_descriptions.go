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

// The block every "standard" and "full" description of get_graph once
// repeated: how to read one line of a `facts` answer, and the traps the
// repeated agent experiment found. Since PLAN_20260928-7 step 4 (narrow
// tools three times cheaper) this is said ONCE, in the server's
// Instructions (serverInstructions below), not in every tool — get_graph's
// own description keeps only its parameters and QUESTION → CALL examples.
// The two consts remain, reused by serverInstructions for the `one` set;
// narrowFactsLineHelp/narrowTrapsHelp below say the same for the `narrow`
// set, in words that name no get_graph parameter.
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

// narrowFactsLineHelp/narrowTrapsHelp: the same two lessons as
// factsLineHelp/trapsHelp, worded for the `narrow` set — no `follow`,
// `lift`, `fanout` or `around` (those tools do not have them); a narrow
// tool's own parameter is `name`, its relation is fixed by which tool was
// called.
const narrowFactsLineHelp = "A line of the default answer reads: " +
	"`<step> <name>  <file>:<lines>  [<relation> <member>:<line> ...] via <node>`. " +
	"`<step>` is how many hops from the focus (0 is the focus itself). " +
	"`<lines>` is `start-end`, or just `start` with no end. " +
	"Inside the brackets, a relation is named looking OUTWARD from the node it was reached from, not from the node the line is about. " +
	"`@31,32` after a relation are the call or construct site lines; `in File.cs` after them names the file of those sites only when it differs from the file already printed on the line. " +
	"`via <node>` (from the second hop on) names the node this line was reached through."

const narrowTrapsHelp = "Traps found in practice: " +
	"asking about a TYPE shows the calls made by its own methods, folded up to the type by default — expect short, summarized lines, not one per method; " +
	"reading or writing a property counts as a call, the same as calling a method — nothing marks it apart; " +
	"an answer that was cut says so in its FIRST line, so check the first line before trusting a count; " +
	"a plain name is usually enough for a tool's `name` — an ambiguous name lists candidates, an unresolved one lists near matches; " +
	"a method carrying a `{dynamic: create @57}` mark means something happens at that line the graph cannot see (reflection-based creation or invocation) — read the code at that line, the graph has no edge for it."

// serverInstructions is the text the server hands a client once, at
// initialisation (mcp.ServerOptions.Instructions) — the SDK gives no way to
// change it on a live server, so it is fixed for the life of the host
// process; a settings change reaches a brand new session only (docs/API.md
// §6). Worded for the tool set actually in use: `one`'s wording names
// get_graph's own parameters (`around`, `follow`, `lift`, `fanout`);
// `narrow`'s does not, since those tools have none of them. This is where
// "how to read a line" and "the traps" live now — said once here, not
// repeated in ten tool descriptions (PLAN_20260928-7 step 4, the narrow set
// existing for small models, whose standard level cost 3x the general tool
// because the same explanation was repeated in each of its ten tools).
func serverInstructions(toolsSet, level string) string {
	return mentalModel(toolsSet == "narrow", level) + " " + serverInstructionsBody(toolsSet, level)
}

// mentalModel: what the system IS, said before anything about how to read
// an answer — an agent that meets the tools cold must not have to guess
// that this is three layers with one cycle between them. brief: two
// sentences; standard and full: the paragraph. The `narrow` wording names
// its own tools (there is no get_graph or graph_formats in that set).
func mentalModel(narrow bool, level string) string {
	if level == "brief" {
		return "SeMaps has three layers: the registry (entities and relations, in git, changed only through these tools and `sync`), the live graph (facts read from the code, never stored), and views (a human's picture of part of the model). " +
			"The graph follows the code by itself; the registry follows it only through `sync`."
	}
	common := "SeMaps has three layers. " +
		"The registry (the model) holds entities `e_*` and the relations between them; it lives in git and changes only through these tools and `sync`. " +
		"The live graph holds the facts an extractor reads from the code — types, methods, calls; it is never stored in git or in the registry. " +
		"Views are a human's projection of part of the model: boxes with geometry, grouped in zones that follow an `axis`. " +
		"When the code changes and the extractor runs, the live graph updates by itself; the registry changes only through `sync` — look with `sync_preview` first, confirm each rename with `confirm_rename`, and `discard` unsaved changes only when a human asked. " +
		"An entity or relation is `present` (found in the code at the last sync) or `missing` (kept in the registry, ids are never deleted, but no longer found in the code); an entity may also be `planned`. "
	if narrow {
		return common + "Which tool answers which question: the structure of the code, who calls whom, all descendants → the `who_*`/`what_*` tools (`who_extends` with a `depth` up to 5 gives all descendants); " +
			"the drawn architecture → `get_view` (a zone reference reads only its subtree); one entity's relations with their evidence → `get_relations`."
	}
	return common + "Which tool answers which question: the structure of the code, who calls whom, all descendants → `get_graph` with `around`, `follow` and `depth` " +
		"(all descendants of a type = `follow` `extended-by` and `implemented-by` with `depth` `all`; `depth` is otherwise 1-5); " +
		"the drawn architecture → `get_view` (a zone reference reads only its subtree); one entity's relations with their evidence → `get_relations`; " +
		"the relation vocabulary, `holds.*` and status terms, and answer formats → `graph_formats`."
}

func serverInstructionsBody(toolsSet, level string) string {
	brief := instructionsBrief["narrow"]
	standard := instructionsStandard["narrow"]
	if toolsSet != "narrow" {
		brief = instructionsBrief["one"]
		standard = instructionsStandard["one"]
	}
	switch level {
	case "brief":
		return brief
	case "full":
		if toolsSet != "narrow" {
			return standard + " " + oneOnlyFullAddendum
		}
		return standard
	default:
		return standard
	}
}

var instructionsBrief = map[string]string{
	"one": "SeMaps keeps a live graph of this codebase: types, methods, and how they relate — extends, holds, calls, constructs, contains, and more. " +
		"A plain name is usually enough to find a node; you rarely need its full id. " +
		"When an answer is cut, its first line says so and how to see more.",
	"narrow": "SeMaps keeps a live graph of this codebase: types and methods and how they relate. " +
		"A plain name is usually enough for `name`. " +
		"A cut answer says so in its first line.",
}

var instructionsStandard = map[string]string{
	"one":    instructionsBrief["one"] + " " + factsLineHelp + " " + trapsHelp,
	"narrow": instructionsBrief["narrow"] + " " + narrowFactsLineHelp + " " + narrowTrapsHelp,
}

// oneOnlyFullAddendum: what belongs to the `one` set only, appended to its
// `full`-level instructions — the vocabulary, `lift`, `fanout` and the
// template language, none of which the `narrow` tools have a parameter for.
const oneOnlyFullAddendum = "Relation vocabulary (name / inverse): extends/extended-by, implements/implemented-by, overrides/overridden-by, holds/held-by, uses/used-by, injects/injected-into, calls/called-by, constructs/constructed-by, depends/depended-on-by, contains/inside. " +
	"`lift=types` (the default when asking about a type) merges the method-level edges between two types into one type-level relation, adding `fromMethods`/`toMethods`; `lift=none` leaves edges on the methods that made them. " +
	"`fanout` caps how many neighbours one node contributes per relation per step; a cut is reported in the answer's first line and does not come back in the same answer. " +
	"The template language shapes an answer of your own: `{macro}` inserts a value or nothing; `[...]` drops an empty optional group whole; `{relations: TEMPLATE | SEPARATOR}` repeats TEMPLATE once per relation, joined by SEPARATOR. " +
	"Call graph_formats for the full macro dictionary and worked examples."

var graphToolDescriptions = map[string]toolDescriptions{
	"get_graph": {
		Brief: "The live code graph: types, methods, and how they relate (extends, holds, calls, constructs, contains, ...), joined with the code position (file:lines) of each. " +
			"Bound the answer with `around` (a node name) and `depth`, or with `container`, or it is cut to a node limit. " +
			"Example: `get_graph {\"around\": \"OrderService\", \"depth\": 1}` — OrderService and its direct neighbours.",
		Standard: "get_graph answers questions about the structure of the code: what a type extends or implements, what it holds or is held by, what calls or is called, what constructs or is created, what a namespace/assembly contains, where something lies. " +
			"Main parameters: `around` (a node name or id — the neighbourhood to walk; omit for the whole graph), `depth` (1-5 hops from `around`, default 1), `follow` (which directed relations to walk outward from `around`; default: everything but containment — call graph_formats for the full list), `level` (`types` to drop method/function/value nodes, `all` to keep them; default `types` without `around`, `all` with it), `lift` (`types` folds method-level edges up to their containing type, default when `around` is a type; `none` keeps them on the methods, default when `around` is a method), `container` (restrict to one containers.json id and its descendants), `fields` (extra data per node: `members`, `via`, `position`, `memberLines`, `dynamic` — a node's marks of blind spots, on by default), `format` (answer shape; default `facts`), `limit`/`listCap` (how much an unbounded answer is cut to). " +
			"By default the answer is text in the `facts` format — see the server's own instructions for how to read a line and the traps found in practice. " +
			"Examples, question → call: " +
			"who extends X two levels deep → `{\"around\":\"X\",\"follow\":[\"extended-by\"],\"depth\":2}`; " +
			"who holds X and through which member → `{\"around\":\"X\",\"follow\":[\"held-by\"]}` (the member name and its declaration line are on each relation); " +
			"who calls the methods of X → `{\"around\":\"X\",\"follow\":[\"called-by\"]}`; " +
			"what a method constructs and calls → `{\"around\":\"X.Method\",\"follow\":[\"constructs\",\"calls\"],\"lift\":\"none\"}`; " +
			"where is X created → `{\"around\":\"X\",\"follow\":[\"constructed-by\"]}`; " +
			"what is inside a namespace and how many → `{\"around\":\"App.Services\",\"follow\":[\"contains\"]}` (the trailing counts line gives the type/method totals); " +
			"in which namespace and assembly does X lie → `{\"around\":\"X\"}` (the node's own line carries `in <namespace> (assembly <assembly>)`).",
		Full: "get_graph answers questions about the structure of the code: what a type extends or implements, what it holds or is held by, what calls or is called, what constructs or is created, what a namespace/assembly contains, where something lies. " +
			"Main parameters: `around` (a node name or id — the neighbourhood to walk; omit for the whole graph), `depth` (1-5 hops from `around`, default 1), `follow` (which directed relations to walk outward from `around`; default: everything but containment), `level` (`types` to drop method/function/value nodes, `all` to keep them; default `types` without `around`, `all` with it), `kinds` (whole-graph request only: comma list of edge kinds to keep), `lift` (`types` folds method-level edges up to their containing type, default when `around` is a type or omitted with level=types; `none` keeps them on the methods, default when `around` is a method), `container` (restrict to one containers.json id and its descendants), `fields` (list; `members`, `via`, `position`, `memberLines`, `dynamic` — a node's marks of blind spots; omitted/null is the default `via,position,dynamic`, `[]` is none), `missing` (include model-only nodes/edges whose entity/relation has status missing; default false), `limit` (cut an unbounded answer to this many nodes; default from settings, 200 unless changed), `listCap` (cut names/lines printed for one relation before '+N'; default from settings, 50 unless changed), `format` (a registered answer shape, or call graph_formats; default from settings, `facts` unless changed), `template` (a template of your own instead of a named format's). " +
			"By default the answer is text in the `facts` format — see the server's own instructions for how to read a line and the traps found in practice. " +
			"Examples, question → call: " +
			"who extends X two levels deep → `{\"around\":\"X\",\"follow\":[\"extended-by\"],\"depth\":2}`; " +
			"who holds X and through which member → `{\"around\":\"X\",\"follow\":[\"held-by\"]}`; " +
			"who calls the methods of X → `{\"around\":\"X\",\"follow\":[\"called-by\"]}`; " +
			"what a method constructs and calls → `{\"around\":\"X.Method\",\"follow\":[\"constructs\",\"calls\"],\"lift\":\"none\"}`; " +
			"where is X created → `{\"around\":\"X\",\"follow\":[\"constructed-by\"]}`; " +
			"what is inside a namespace and how many → `{\"around\":\"App.Services\",\"follow\":[\"contains\"]}`; " +
			"in which namespace and assembly does X lie → `{\"around\":\"X\"}`. " +
			"Relation vocabulary (name / inverse): extends / extended-by (a type extends its base type); implements / implemented-by (a type or method implements an interface or interface method); overrides / overridden-by (a method overrides a base virtual method); holds / held-by (a field/property/parameter holds a value of the target type, any cardinality; holds.many / held-by.many narrows to cardinality many); uses / used-by (a member's type refers to the target other than by holding or injecting it); injects / injected-into (a constructor parameter injects the target type); calls / called-by (a method calls another, or reads/writes a property); constructs / constructed-by (a method constructs the target type with `new`); depends / depended-on-by (a module-level dependency); contains / inside (a namespace/assembly/type contains a member). " +
			"`lift`: `types` merges every method-level edge between the methods of two types into one type-level relation, adding `×N`, `fromMethods`, `toMethods` on it (capped at 5 names then '+N'); `none` leaves edges on the methods that made them, which is required to see a specific method's own calls/constructs. " +
			"`fanout` (walk parameter, alongside `depth`): caps how many neighbours one node contributes per relation per step; 0 (default) is unlimited; a cut is reported in the answer's first line and does not come back in the same answer. " +
			"`container`/`limit`/`listCap` as above. " +
			"Answer formats: `facts` (compact text, one line per node, the default), `lines` (text, one line per relation), `tree` (text, indented walk from `around`, requires `around`), `locations` (tab-separated file:line rows), `json` and `json-compact` (structured, also returned as structuredContent). Call graph_formats for the exact list with each format's own template and the full relation table. " +
			"Name resolution (`around` as a name, not an id): resolved by id first, then by name, case-insensitively as a last resort; an ambiguous name answers `{\"error\":\"ambiguous\",\"candidates\":[...]}` instead of picking one; an unresolved name is a tool error naming the nearest matches; a name that only resolves case-insensitively, or through a method's short signature rather than its id, adds a one-line notice at the top of the answer saying so. A node hidden because its entity/relation has status `missing` needs `missing:true` to be reachable at all. " +
			"Template language (`template`, in place of `format`): literal text is copied as is, `{macro}` is replaced by that macro's value or nothing when absent, `[...]` is an optional group dropped whole (with its literal text) when any macro or nested `{relations:...}` block directly inside it is empty, `\\[`/`\\]` escape a literal bracket, and a run of plain spaces at the very start or end of the rendered template is dropped (a run with real content on both sides is kept). `{relations: TEMPLATE | SEPARATOR}` renders TEMPLATE once per relation reaching the node, joined by SEPARATOR. Node macros: step, name, fullName, id, kind, nativeKind, visibility, file, line, endLine, lines, namespace, assembly, containers, presence, status, via, viaFullName. Relation macros (inside a relations block only): relation, member, memberKind, memberLine, modifiers, cardinality, type, text, relationLine, relationLines, relationLinesFile, count, fromMethods, toMethods, injected. `{memberLine}` is where a held/used/injected member is declared; `{relationLine}`/`{relationLines}` are call/construct site line(s) — never both set for the same edge kind. `{count}`/`{fromMethods}`/`{toMethods}` are set only on a lifted relation. `{injected}` is the literal text `(injected)` when the same member's holds and injects were folded into one relation line. A malformed template (unmatched `{`, `[` or `]`, or an unknown macro) is a tool error naming the position and the macro.",
	},
	"find_node": {
		Brief: "Substring search over graph node names and ids, for the exact spelling of a name. " +
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
// `follow` (host/mcp.go registers the wrapper with these names) and enough
// to build its three description levels without repeating, in every tool,
// what get_graph's own description already says once (PLAN_20260928-7 step
// 4: the narrow set exists for small models, and repeating the same long
// explanation in each of its ten tools defeats that purpose — how to read a
// line and the traps live in the server's instructions instead, see
// serverInstructions above). Brief is one sentence naming what the tool
// lists; Line1/Line2 are the first two lines a one-hop example call would
// print, for `standard`; Line2Deep is the second line of a two-hop example,
// ending in `via`, for `full`.
type narrowTool struct {
	Follow       []string
	Brief        string // one sentence: what the tool lists
	Example      string // example value for `name` in the one example call
	Line1, Line2 string // the answer's first two lines, one hop
	Line2Deep    string // a deeper line (depth 2), ending "via <node>"
}

var narrowTools = map[string]narrowTool{
	"who_extends": {Follow: []string{"extended-by", "implemented-by"}, Example: "Middleware",
		Brief:     "Lists the types that extend or implement the named type.",
		Line1:     "0 Middleware  src/Middleware.cs:1-60",
		Line2:     "1 App.Guards.SubGuard  src/Guards/SubGuard.cs:1-5  [extended-by]",
		Line2Deep: "2 App.Guards.SubSubGuard  src/Guards/SubSubGuard.cs:1-5  [extended-by] via App.Guards.SubGuard"},
	"what_it_extends": {Follow: []string{"extends", "implements"}, Example: "RepetitionGuard",
		Brief:     "Lists what the named type extends or implements.",
		Line1:     "0 RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40",
		Line2:     "1 App.Middleware  src/Middleware.cs:1-60  [extends]",
		Line2Deep: "2 App.Core.Base  src/Core/Base.cs:1-20  [extends] via App.Middleware"},
	"who_holds": {Follow: []string{"held-by", "injected-into"}, Example: "Context",
		Brief:     "Lists what holds or injects the named type, and through which member.",
		Line1:     "0 Context  src/Context.cs:5-30",
		Line2:     "1 App.Guards.RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40  [held-by Log:15]",
		Line2Deep: "2 App.Runner  src/Runner.cs:1-25  [held-by Guard:9] via App.Guards.RepetitionGuard"},
	"what_it_holds": {Follow: []string{"holds", "injects"}, Example: "Runner",
		Brief:     "Lists what the named type holds or injects, and through which member.",
		Line1:     "0 Runner  src/Runner.cs:1-25",
		Line2:     "1 App.Guards.RepetitionGuard  src/Guards/RepetitionGuard.cs:10-40  [holds Guard:9]",
		Line2Deep: "2 App.Context  src/Context.cs:5-30  [holds Log:15] via App.Guards.RepetitionGuard"},
	"who_calls": {Follow: []string{"called-by"}, Example: "Factory",
		Brief:     "Lists what calls the methods of the named type.",
		Line1:     "0 Factory  src/Factory.cs:1-15",
		Line2:     "1 App.Widgets.Widget  src/Widgets/Widget.cs:3-20  [called-by from Create]",
		Line2Deep: "2 App.Runner  src/Runner.cs:1-25  [called-by from Start] via App.Widgets.Widget"},
	"what_it_calls": {Follow: []string{"calls", "constructs"}, Example: "OnnxModel",
		Brief:     "Lists what the named type's methods call or construct.",
		Line1:     "0 OnnxModel  src/OnnxModel.cs:14-164",
		Line2:     "1 App.Session  src/Session.cs:1-40  [calls from CreateRunner]",
		Line2Deep: "2 App.Tensor  src/Tensor.cs:1-10  [constructs from Session] via App.Session"},
	"where_created": {Follow: []string{"constructed-by"}, Example: "ImageRunner",
		Brief:     "Lists where the named type is constructed with `new`.",
		Line1:     "0 ImageRunner  src/ImageRunner.cs:1-30",
		Line2:     "1 App.Factory  src/Factory.cs:1-15  [constructed-by from Create]",
		Line2Deep: "2 App.Startup  src/Startup.cs:1-20  [constructed-by from Configure] via App.Factory"},
	"what_is_inside": {Follow: []string{"contains"}, Example: "App.Services",
		Brief:     "Lists what a namespace, assembly, or type contains, and how many.",
		Line1:     "0 App.Services",
		Line2:     "1 App.Services.OrderService  src/Services/OrderService.cs:1-50  [contains]",
		Line2Deep: "2 App.Services.OrderService.Create  src/Services/OrderService.cs:12  [contains] via App.Services.OrderService"},
	"where_it_lies": {Follow: []string{"inside"}, Example: "OrderService",
		Brief:     "Lists which namespace, assembly, or type the named thing lies in.",
		Line1:     "0 OrderService  src/Services/OrderService.cs:1-50  in App.Services (assembly App)",
		Line2:     "1 App.Services  [inside]",
		Line2Deep: "2 App  [inside] via App.Services"},
}

// narrowDescriptions builds the toolDescriptions of one narrow tool from its
// narrowTool entry. `standard` names only this tool's own parameters
// (`name`, `depth`) and shows one example call with the first two lines of
// its answer; `full` adds `depth` and what a deeper answer looks like. No
// vocabulary, no `follow`/`lift`/`fanout`/`around` — those belong to
// get_graph, which this tool does not expose (PLAN_20260928-7 step 4).
func narrowDescriptions(name string, t narrowTool) toolDescriptions {
	call := "`" + name + " {\"name\": \"" + t.Example + "\"}`"
	brief := t.Brief
	standard := t.Brief + " Give `name` — a type's short name is enough. " +
		"Example: " + call + " returns:\n" + t.Line1 + "\n" + t.Line2
	full := standard + "\n\n" +
		"`depth` widens the walk beyond one hop (1-5, default 1). A line reached at a deeper hop ends with `via <node>`, naming the neighbour it came through, e.g.:\n" +
		t.Line2Deep
	return toolDescriptions{Brief: brief, Standard: standard, Full: full}
}
