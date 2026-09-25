// The SeMaps host: serves the editor bundle, a workspace of models, and the
// source tree that `codeRef` points into. Three roots, kept apart on purpose:
//
//   - the tool itself: `app/` and `defaults/`, embedded into the binary;
//   - the workspace: projects, and any override of the defaults;
//   - the source root: code, read-only, for the code viewer and codeRef checks.
//
// Run with no arguments from anywhere inside a project: the workspace and the
// source root are found by walking up from the current directory.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"semaps/core"
)

//go:embed all:app all:defaults
var bundled embed.FS

// Workspace files the tool ships a default for. A workspace that has its own
// copy wins; otherwise the default is served. Saving always writes to the
// workspace, so the first save of a default turns it into an override.
var overridable = []string{"styles.json", "templates.json", "content/"}

// findProjectFile walks up from dir to the first directory with a .semaps
// file. Two of them in one directory is an error, not a silent pick.
func findProjectFile(dir string) (string, bool, error) {
	for {
		if m, _ := filepath.Glob(filepath.Join(dir, "*"+ProjectExt)); len(m) > 1 {
			return "", false, fmt.Errorf("several project files in %s: %s — pass one explicitly", dir, strings.Join(m, ", "))
		} else if len(m) == 1 {
			return m[0], true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

// noCacheHandler makes the browser revalidate every file (HTML, JS, CSS, JSON) it reads.
func noCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// findSourceRoot is the nearest repository root above the workspace, or the
// workspace itself when there is none. Used only with an explicit --workspace:
// a project file names its source root.
func findSourceRoot(workspace string) string {
	for dir := workspace; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return workspace
		}
		dir = parent
	}
}

// runningFor looks for a SeMaps host already serving workspace on port or the
// ports listen would have moved on to.
func runningFor(port int, workspace string) (string, bool) {
	client := http.Client{Timeout: 300 * time.Millisecond}
	for p := port; p < port+20; p++ {
		res, err := client.Get(fmt.Sprintf("http://localhost:%d/api/info", p))
		if err != nil {
			continue
		}
		var info struct{ Workspace string }
		err = json.NewDecoder(res.Body).Decode(&info)
		res.Body.Close()
		if err == nil && strings.EqualFold(filepath.Clean(info.Workspace), filepath.Clean(workspace)) {
			return fmt.Sprintf("http://localhost:%d/app/", p), true
		}
	}
	return "", false
}

// listen takes the requested port, or the next free one after it.
func listen(port int) (net.Listener, error) {
	var err error
	for p := port; p < port+20; p++ {
		var l net.Listener
		if l, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", p)); err == nil {
			return l, nil
		}
	}
	return nil, err
}

// inside resolves a slash-separated relative path under root and refuses
// anything that escapes it.
func inside(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", errors.New("path must be relative")
	}
	target := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, target)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", errors.New("path outside root")
	}
	return target, nil
}

// noListing wraps a file system so that directories are not listed: the
// editor asks for files by name, and a listing of a workspace or of the
// source tree is nobody's business.
type noListing struct{ http.FileSystem }

func (n noListing) Open(name string) (http.File, error) {
	f, err := n.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err == nil && info.IsDir() {
		f.Close()
		return nil, fs.ErrNotExist
	}
	return f, nil
}

// sameOrigin is true when the request was made by a page this host served.
// The host listens on localhost, but any page open in the same browser can
// still POST to localhost; without this, a stray site could rewrite the
// workspace. Non-browser clients (curl, agents) send no Origin and pass.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == "" || r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	return strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://"), r.Host)
}

// workspaceHandler serves the workspace, falling back to the tool defaults
// for the overridable files.
func workspaceHandler(workspace string, defaults fs.FS) http.Handler {
	ws := http.FileServer(noListing{http.Dir(workspace)})
	def := http.FileServer(noListing{http.FS(defaults)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		for _, o := range overridable {
			if rel == o || (strings.HasSuffix(o, "/") && strings.HasPrefix(rel, o)) {
				if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(rel))); err != nil {
					def.ServeHTTP(w, r)
					return
				}
				break
			}
		}
		ws.ServeHTTP(w, r)
	})
}

func main() {
	// `semaps check ...` takes the same roots as the server and runs the
	// model check on them instead of serving.
	checkMode := len(os.Args) > 1 && os.Args[1] == "check"
	// `semaps sync --facts f.json ...`: the same roots, the registry is
	// reconciled with extractor facts (docs/EXTRACTOR.md §5).
	syncMode := len(os.Args) > 1 && os.Args[1] == "sync"
	// `semaps doctor` and `semaps extract`: the extractors of the .semaps
	// file (ADR_20260924-3).
	doctorMode := len(os.Args) > 1 && os.Args[1] == "doctor"
	extractMode := len(os.Args) > 1 && os.Args[1] == "extract"
	// `semaps mcp`: MCP server over stdio (ADR_20260924-5).
	mcpMode := len(os.Args) > 1 && os.Args[1] == "mcp"
	toolMode := doctorMode || extractMode
	if checkMode || syncMode || toolMode || mcpMode {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	// `semaps install`: copy to a stable folder, PATH, *.semaps association.
	if len(os.Args) > 1 && os.Args[1] == "install" {
		if err := install(); err != nil {
			log.Fatalf("Install: %v", err)
		}
		return
	}

	var port int
	var workspaceDir, sourceDir string
	var noBrowser bool
	flag.IntVar(&port, "port", 8777, "Port to listen on; the next free one is taken if busy")
	flag.StringVar(&workspaceDir, "workspace", "", "Workspace directory (default: found upward from the current directory)")
	flag.StringVar(&sourceDir, "source-root", "", "Directory codeRef paths resolve against (default: the repository root above the workspace)")
	var here bool
	flag.BoolVar(&noBrowser, "no-browser", false, "Do not open a browser")
	flag.BoolVar(&here, "here", false, "Run the server in this console instead of a new window")
	var sync struct {
		facts, project    string
		dryRun, noRenames bool
		extractor, run    string
	}
	if syncMode || extractMode {
		flag.StringVar(&sync.extractor, "extractor", "", "Extractor id from the .semaps file (default: all of them)")
	}
	if mcpMode {
		flag.StringVar(&sync.project, "project", "", "Project id under projects/ (default: the only one there is)")
	}
	if syncMode {
		flag.StringVar(&sync.facts, "facts", "", "Extractor facts (EXTRACTOR.md §2); `-` reads stdin. Default: run the extractors of the .semaps file")
		flag.StringVar(&sync.run, "run", "", "Use the facts of this run (`semaps extract` prints its id) instead of extracting again")
		flag.StringVar(&sync.project, "project", "", "Project id under projects/ (default: the only one there is)")
		flag.BoolVar(&sync.dryRun, "dry-run", false, "Report only, write nothing; exit 1 when anything would change")
		flag.BoolVar(&sync.noRenames, "no-renames", false, "Treat rename candidates as one entity gone and one new")
	}
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: semaps [flags] [dir | file.semaps]\n       semaps check [flags] [dir | file.semaps]\n       semaps sync [--extractor <id>] [--run <id> | --facts <file.json>] [flags] [dir | file.semaps]\n       semaps extract [--extractor <id>] [dir | file.semaps]\n       semaps doctor [dir | file.semaps]\n       semaps mcp [--project <id>] [dir | file.semaps]\n\nWith no arguments, finds a *.semaps project file upward from the current directory.\n`check` reports stale texts, views without an axis, broken codeRef and the like;\nexit code 1 when anything is found.\n`sync` reconciles entities.json and relations.json with extractor facts; without\n--facts it runs the extractors listed in the .semaps file. `extract` only runs them;\n`doctor` shows which extractors and runtimes are found;\n`mcp` serves the registry as MCP tools on stdio (docs/API.md §6). Flags go\nbefore the project argument. `semaps sync --help` lists its flags.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if sync.facts != "" && (sync.run != "" || sync.extractor != "") {
		fmt.Fprintln(os.Stderr, "semaps sync: --facts goes alone, without --run or --extractor")
		os.Exit(2)
	}
	// Extractors are listed in the .semaps file: without --facts there must be one.
	needProject := toolMode || (syncMode && sync.facts == "") || mcpMode
	if needProject && workspaceDir != "" {
		fmt.Fprintln(os.Stderr, "semaps: extractors come from a .semaps file; --workspace has none")
		os.Exit(2)
	}
	var proj project

	portSet := false
	flag.Visit(func(f *flag.Flag) { portSet = portSet || f.Name == "port" })

	if workspaceDir == "" {
		arg := flag.Arg(0)
		if arg == "" {
			arg = "."
		}
		absArg, _ := filepath.Abs(arg)
		file := ""
		if strings.EqualFold(filepath.Ext(absArg), ProjectExt) {
			file = absArg
		} else if f, ok, err := findProjectFile(absArg); err != nil {
			log.Fatalf("Project file: %v", err)
		} else if ok {
			file = f
		}
		if file == "" {
			// A downloaded exe started by a double click lands here: nothing
			// to open, not installed. Offer the install instead of vanishing.
			if flag.NArg() == 0 && !checkMode && !syncMode && !toolMode && !installed() {
				offerInstall()
				return
			}
			fmt.Fprintf(os.Stderr, "No *%s found from %s upward. Create one (see docs/ADOPTING.md) or pass --workspace.\n", ProjectExt, absArg)
			os.Exit(2)
		}
		var err error
		proj, err = loadProject(file)
		if err != nil {
			log.Fatalf("Project file: %v", err)
		}
		// stdout of `semaps mcp` is the protocol.
		if mcpMode {
			fmt.Fprintf(os.Stderr, "Project %s (%s)\n", proj.Name, proj.File)
		} else {
			fmt.Printf("Project %s (%s)\n", proj.Name, proj.File)
		}
		workspaceDir = proj.Workspace
		if sourceDir == "" {
			sourceDir = proj.SourceRoot
		}
		if !portSet && proj.Port != 0 {
			port = proj.Port
		}
	}

	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		log.Fatalf("Failed to resolve workspace: %v", err)
	}
	if sourceDir == "" {
		sourceDir = findSourceRoot(absWorkspace)
	}
	absRoot, err := filepath.Abs(sourceDir)
	if err != nil {
		log.Fatalf("Failed to resolve source root: %v", err)
	}

	if checkMode {
		fmt.Printf("  workspace:   %s\n  source root: %s\n\n", absWorkspace, absRoot)
		os.Exit(core.Report(os.Stdout, core.Check(absWorkspace, absRoot)))
	}
	if doctorMode {
		os.Exit(runDoctor(os.Stdout, proj))
	}
	if extractMode {
		_, code := runExtract(proj, sync.extractor)
		os.Exit(code)
	}
	if mcpMode {
		os.Exit(runMCP(proj, absWorkspace, absRoot, sync.project))
	}
	if syncMode {
		fmt.Printf("  workspace:   %s\n\n", absWorkspace)
		opt := core.SyncOptions{Project: sync.project, DryRun: sync.dryRun, NoRenames: sync.noRenames}
		if sync.facts != "" {
			os.Exit(runSync(absWorkspace, sync.facts, opt))
		}
		os.Exit(runSyncProject(proj, absWorkspace, sync.extractor, sync.run, opt))
	}

	// Already serving this workspace? Just show it.
	if url, ok := runningFor(port, absWorkspace); ok {
		fmt.Printf("Already running: %s\n", url)
		if !noBrowser {
			openBrowser(url)
		}
		return
	}

	// Launched from a file manager or a shell: hand the server to its own
	// console window and give the caller its prompt back.
	if !here {
		if err := detach(os.Args[1:]); err == nil {
			return
		} else {
			log.Printf("Could not open a new window, running here: %v", err)
		}
	}

	appFS, _ := fs.Sub(bundled, "app")
	defaultsFS, _ := fs.Sub(bundled, "defaults")

	listener, err := listen(port)
	if err != nil {
		log.Fatalf("No free port from %d: %v", port, err)
	}
	url := fmt.Sprintf("http://%s/app/", listener.Addr().String())

	fmt.Printf("SeMaps on %s\n", url)
	fmt.Printf("  workspace:   %s\n", absWorkspace)
	fmt.Printf("  source root: %s\n", absRoot)
	fmt.Println("Press Ctrl+C to stop.")
	if !installed() {
		fmt.Println("Hint: `semaps install` puts semaps on PATH and opens *.semaps files by double click.")
	}

	if !noBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	http.Handle("/app/", noCacheHandler(http.StripPrefix("/app/", http.FileServer(http.FS(appFS)))))
	http.Handle("/", noCacheHandler(workspaceHandler(absWorkspace, defaultsFS)))
	registerToolAPI(http.DefaultServeMux, proj.File, absWorkspace)
	// Short addresses of the tool pages (ADR_20260924-3 §4).
	for short, page := range map[string]string{"/setup": "/app/#project", "/extract": "/app/#extract"} {
		http.Handle("GET "+short, http.RedirectHandler(page, http.StatusFound))
	}
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"workspace": absWorkspace, "sourceRoot": absRoot})
	})
	http.HandleFunc("/api/workspace", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(core.Index(absWorkspace))
	})

	http.HandleFunc("/api/source", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filePath := strings.TrimSpace(r.URL.Query().Get("file"))
		if filePath == "" {
			http.Error(w, "Missing file parameter", http.StatusBadRequest)
			return
		}

		absTarget, err := inside(absRoot, filePath)
		if err != nil {
			http.Error(w, "Access denied: "+err.Error(), http.StatusForbidden)
			return
		}

		info, err := os.Stat(absTarget)
		if err != nil || info.IsDir() {
			http.Error(w, fmt.Sprintf("Source file not found: %s", filePath), http.StatusNotFound)
			return
		}

		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			return
		}

		data, err := os.ReadFile(absTarget)
		if err != nil {
			http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	http.HandleFunc("/api/source/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var filesToCheck []string
		if r.Method == http.MethodGet {
			fileParam := strings.TrimSpace(r.URL.Query().Get("file"))
			if fileParam != "" {
				filesToCheck = append(filesToCheck, fileParam)
			}
		} else {
			body, err := io.ReadAll(r.Body)
			if err == nil && len(body) > 0 {
				_ = json.Unmarshal(body, &filesToCheck)
			}
		}

		result := make(map[string]bool, len(filesToCheck))
		for _, f := range filesToCheck {
			absTarget, err := inside(absRoot, f)
			if err != nil {
				result[f] = false
				continue
			}
			info, err := os.Stat(absTarget)
			result[f] = (err == nil && !info.IsDir())
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})

	http.HandleFunc("/api/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "Cross-origin save refused", http.StatusForbidden)
			return
		}

		fileName := strings.TrimSpace(r.URL.Query().Get("file"))
		if fileName == "" {
			http.Error(w, "Missing file parameter", http.StatusBadRequest)
			return
		}

		// Only .json files inside the workspace.
		target, err := inside(absWorkspace, fileName)
		if err != nil || filepath.Ext(target) != ".json" {
			http.Error(w, "Invalid file path (must be a relative .json path within workspace)", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// `create=1`: a new project or view must not land on an existing one.
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if r.URL.Query().Get("create") == "1" {
			flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		}
		f, err := os.OpenFile(target, flags, 0644)
		if errors.Is(err, fs.ErrExist) {
			http.Error(w, "Already exists: "+fileName, http.StatusConflict)
			return
		}
		if err == nil {
			_, err = f.Write(body)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("Saved %s (%d bytes)\n", fileName, len(body))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Renaming a project folder or a view file. Only inside projects/, never
	// onto something that exists: a rename that overwrites is a delete.
	http.HandleFunc("/api/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "Cross-origin move refused", http.StatusForbidden)
			return
		}
		fromRel, toRel := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		projects := filepath.Join(absWorkspace, "projects")
		from, errFrom := inside(projects, strings.TrimPrefix(fromRel, "projects/"))
		to, errTo := inside(projects, strings.TrimPrefix(toRel, "projects/"))
		if errFrom != nil || errTo != nil || !strings.HasPrefix(fromRel, "projects/") || !strings.HasPrefix(toRel, "projects/") {
			http.Error(w, "from and to must be paths inside projects/", http.StatusBadRequest)
			return
		}
		src, err := os.Stat(from)
		if err != nil {
			http.Error(w, "Not found: "+fromRel, http.StatusNotFound)
			return
		}
		if !src.IsDir() && filepath.Ext(to) != ".json" {
			http.Error(w, "A file keeps the .json extension", http.StatusBadRequest)
			return
		}
		// Checked here, not left to Rename: on Windows Rename onto a file replaces it.
		if _, err := os.Stat(to); err == nil {
			http.Error(w, "Already exists: "+toRel, http.StatusConflict)
			return
		}
		if err := os.Rename(from, to); err != nil {
			http.Error(w, "Failed to move: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Printf("Moved %s -> %s\n", fromRel, toRel)
		w.Write([]byte("OK"))
	})

	log.Fatal(http.Serve(listener, nil))
}

// runSync reads the facts and reconciles the registry. Exit codes: 0 in sync
// (or applied with nothing left to decide), 1 findings or unreadable facts,
// 2 a usage mistake.
func runSync(workspace, factsPath string, opt core.SyncOptions) int {
	var in io.Reader = os.Stdin
	if factsPath != "-" {
		f, err := os.Open(factsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "semaps sync: %v\n", err)
			return 2
		}
		defer f.Close()
		in = f
	}
	facts, err := core.ReadFacts(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semaps sync: %v\n", err)
		return 1
	}
	report, err := core.Sync(workspace, facts, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semaps sync: %v\n", err)
		var usage *core.UsageError
		if errors.As(err, &usage) {
			return 2
		}
		return 1
	}
	report.Print(os.Stdout)
	return report.ExitCode()
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Printf("Could not open a browser: %v\n", err)
	}
}
