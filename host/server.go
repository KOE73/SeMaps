// The SeMaps host: serves the editor bundle, a workspace of models, and the
// source tree that `codeRef` points into. Three roots, kept apart on purpose:
//
//   - the tool folder (next to the binary): `app/` and `defaults/`;
//   - --workspace: catalog, projects, and any override of the defaults;
//   - --source-root: code, read-only, for the code viewer and codeRef checks.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Workspace files the tool ships a default for. A workspace that has its own
// copy wins; otherwise the default is served. Saving always writes to the
// workspace, so the first save of a default turns it into an override.
var overridable = []string{"styles.json", "templates.json", "content/"}

// noCacheHandler makes the browser revalidate every file (HTML, JS, CSS, JSON) it reads.
func noCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// toolDir is where `app/` and `defaults/` live: next to the binary, or — under
// `go run`, where the binary sits in a temp folder — the current directory.
func toolDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if info, err := os.Stat(filepath.Join(dir, "app")); err == nil && info.IsDir() {
			return dir
		}
	}
	dir, _ := os.Getwd()
	return dir
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
func workspaceHandler(workspace, defaults string) http.Handler {
	ws := http.FileServer(http.Dir(workspace))
	def := http.FileServer(http.Dir(defaults))
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
	var port, workspaceDir, sourceDir string
	flag.StringVar(&port, "port", "8777", "Port to listen on")
	flag.StringVar(&workspaceDir, "workspace", "", "Workspace directory: catalog.json, projects/, overrides (required)")
	flag.StringVar(&sourceDir, "source-root", "", "Directory codeRef paths resolve against (default: the workspace)")
	flag.Parse()

	if workspaceDir == "" {
		fmt.Fprintln(os.Stderr, "--workspace is required")
		flag.Usage()
		os.Exit(2)
	}
	if sourceDir == "" {
		sourceDir = workspaceDir
	}

	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		log.Fatalf("Failed to resolve workspace: %v", err)
	}
	absRoot, err := filepath.Abs(sourceDir)
	if err != nil {
		log.Fatalf("Failed to resolve source root: %v", err)
	}
	tool := toolDir()

	url := fmt.Sprintf("http://localhost:%s/app/", port)

	fmt.Printf("SeMaps on %s\n", url)
	fmt.Printf("  tool:        %s\n", tool)
	fmt.Printf("  workspace:   %s\n", absWorkspace)
	fmt.Printf("  source root: %s\n", absRoot)
	fmt.Println("Press Ctrl+C to stop.")

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	http.Handle("/app/", noCacheHandler(http.StripPrefix("/app/", http.FileServer(http.Dir(filepath.Join(tool, "app"))))))
	http.Handle("/", noCacheHandler(workspaceHandler(absWorkspace, filepath.Join(tool, "defaults"))))

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

	log.Fatal(http.ListenAndServe(":"+port, nil))
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
