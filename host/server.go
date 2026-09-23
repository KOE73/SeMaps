// The SeMaps host: serves the editor bundle, a workspace of models, and the
// source tree that `codeRef` points into. Three roots, kept apart on purpose:
//
//   - the tool itself: `app/` and `defaults/`, embedded into the binary;
//   - the workspace: catalog, projects, and any override of the defaults;
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
)

//go:embed all:app all:defaults
var bundled embed.FS

// Workspace files the tool ships a default for. A workspace that has its own
// copy wins; otherwise the default is served. Saving always writes to the
// workspace, so the first save of a default turns it into an override.
var overridable = []string{"styles.json", "templates.json", "content/"}

// ProjectExt marks a project file: `<name>.semaps` in the project root. The
// directory holding it is the project root; every path inside is relative to it.
// Opening the file (double click, Enter in a file manager, or `semaps x.semaps`)
// opens the project.
const ProjectExt = ".semaps"

// project is the content of a .semaps file. Every key is optional.
type project struct {
	File       string
	Name       string // shown in the console
	Workspace  string // default: docs/diagrams
	SourceRoot string // default: the project root
	Port       int    // default: 8777
}

// loadProject reads a .semaps file: flat `key: value` lines, `#` comments —
// the YAML subset this needs, without a YAML dependency.
func loadProject(file string) (project, error) {
	p := project{File: file, Workspace: "docs/diagrams", SourceRoot: "."}
	data, err := os.ReadFile(file)
	if err != nil {
		return p, err
	}
	for n, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return p, fmt.Errorf("%s:%d: expected `key: value`", file, n+1)
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "version":
		case "name":
			p.Name = value
		case "workspace":
			p.Workspace = value
		case "source_root":
			p.SourceRoot = value
		case "port":
			if _, err := fmt.Sscan(value, &p.Port); err != nil {
				return p, fmt.Errorf("%s:%d: port must be a number", file, n+1)
			}
		default:
			return p, fmt.Errorf("%s:%d: unknown key %q", file, n+1, key)
		}
	}
	root := filepath.Dir(file)
	p.Workspace = filepath.Join(root, filepath.FromSlash(p.Workspace))
	p.SourceRoot = filepath.Join(root, filepath.FromSlash(p.SourceRoot))
	return p, nil
}

// findProjectFile walks up from dir to the first directory with a .semaps file.
func findProjectFile(dir string) (string, bool) {
	for {
		if m, _ := filepath.Glob(filepath.Join(dir, "*"+ProjectExt)); len(m) > 0 {
			return m[0], true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Where a workspace may sit relative to a directory on the way up, when there
// is no project file.
var workspaceCandidates = []string{".", "docs/diagrams"}

// noCacheHandler makes the browser revalidate every file (HTML, JS, CSS, JSON) it reads.
func noCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// findWorkspace walks up from dir to the first directory that holds
// `catalog.json` itself or in `docs/diagrams/`.
func findWorkspace(dir string) (string, bool) {
	for {
		for _, c := range workspaceCandidates {
			ws := filepath.Join(dir, c)
			if isFile(filepath.Join(ws, "catalog.json")) {
				return ws, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// findSourceRoot is the nearest repository root above the workspace, or the
// workspace itself when there is none.
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

// workspaceHandler serves the workspace, falling back to the tool defaults
// for the overridable files.
func workspaceHandler(workspace string, defaults fs.FS) http.Handler {
	ws := http.FileServer(http.Dir(workspace))
	def := http.FileServer(http.FS(defaults))
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
	var port int
	var workspaceDir, sourceDir string
	var noBrowser bool
	flag.IntVar(&port, "port", 8777, "Port to listen on; the next free one is taken if busy")
	flag.StringVar(&workspaceDir, "workspace", "", "Workspace directory (default: found upward from the current directory)")
	flag.StringVar(&sourceDir, "source-root", "", "Directory codeRef paths resolve against (default: the repository root above the workspace)")
	var here bool
	flag.BoolVar(&noBrowser, "no-browser", false, "Do not open a browser")
	flag.BoolVar(&here, "here", false, "Run the server in this console instead of a new window")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: semaps [flags] [dir | file.semaps]\n\nWith no arguments, finds a *.semaps project file upward from the current directory,\nthen falls back to catalog.json or docs/diagrams/catalog.json.")
		flag.PrintDefaults()
	}
	flag.Parse()

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
		} else if f, ok := findProjectFile(absArg); ok {
			file = f
		}
		if file != "" {
			proj, err := loadProject(file)
			if err != nil {
				log.Fatalf("Project file: %v", err)
			}
			fmt.Printf("Project %s (%s)\n", proj.Name, proj.File)
			workspaceDir = proj.Workspace
			if sourceDir == "" {
				sourceDir = proj.SourceRoot
			}
			if !portSet && proj.Port != 0 {
				port = proj.Port
			}
		} else {
			ws, ok := findWorkspace(absArg)
			if !ok {
				fmt.Fprintf(os.Stderr, "No *%s, catalog.json or docs/diagrams/catalog.json found from %s upward.\n", ProjectExt, absArg)
				os.Exit(2)
			}
			workspaceDir = ws
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

	if !noBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	http.Handle("/app/", noCacheHandler(http.StripPrefix("/app/", http.FileServer(http.FS(appFS)))))
	http.Handle("/", noCacheHandler(workspaceHandler(absWorkspace, defaultsFS)))
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"workspace": absWorkspace, "sourceRoot": absRoot})
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

		cleanPath := filepath.Clean(filepath.FromSlash(filePath))
		if filepath.IsAbs(cleanPath) {
			http.Error(w, "Absolute paths not allowed", http.StatusBadRequest)
			return
		}

		targetPath := filepath.Join(absRoot, cleanPath)
		absTarget, err := filepath.Abs(targetPath)
		if err != nil {
			http.Error(w, "Failed to resolve path", http.StatusBadRequest)
			return
		}

		relToRoot, err := filepath.Rel(absRoot, absTarget)
		if err != nil || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || relToRoot == ".." {
			http.Error(w, "Access denied: file outside root", http.StatusForbidden)
			return
		}

		info, err := os.Stat(absTarget)
		if err != nil || info.IsDir() {
			http.Error(w, fmt.Sprintf("Source file not found: %s", cleanPath), http.StatusNotFound)
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
			cleanPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(f)))
			if cleanPath == "" || cleanPath == "." || filepath.IsAbs(cleanPath) {
				result[f] = false
				continue
			}

			targetPath := filepath.Join(absRoot, cleanPath)
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				result[f] = false
				continue
			}

			relToRoot, err := filepath.Rel(absRoot, absTarget)
			if err != nil || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || relToRoot == ".." {
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

		err = os.WriteFile(target, body, 0644)
		if err != nil {
			http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("Saved %s (%d bytes)\n", fileName, len(body))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Fatal(http.Serve(listener, nil))
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
