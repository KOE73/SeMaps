package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPProxyStartsHostAndCallsTool(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the host binary")
	}
	gotmp := os.Getenv("GOTMPDIR")
	if gotmp == "" {
		t.Skip("GOTMPDIR is required for executable test location")
	}
	binDir, err := os.MkdirTemp(gotmp, "semaps-proxy-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(binDir)
	exe := filepath.Join(binDir, "semaps")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	build := exec.Command("go", "build", "-o", exe, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	projectDir := filepath.Join(ws, "projects", "p")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{"project.json": `{"id":"p","languages":["ru"]}`, "entities.json": `{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`} {
		if err := os.WriteFile(filepath.Join(projectDir, file), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	projectFile := filepath.Join(root, "test.semaps")
	config := fmt.Sprintf("version: 1\nname: test\nworkspace: ws\nsource_root: .\nport: %d\n", port)
	if err := os.WriteFile(projectFile, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "proxy-test"}, nil)
	cmd := exec.Command(exe, "mcp", projectFile)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer func() {
		b, err := os.ReadFile(filepath.Join(root, ".semaps", "host.json"))
		if err != nil {
			return
		}
		var info hostInfo
		if json.Unmarshal(b, &info) == nil && info.PID > 0 {
			if p, err := os.FindProcess(info.PID); err == nil {
				_ = p.Kill()
			}
		}
	}()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_entity", Arguments: map[string]any{"id": "e_a"}})
	if err != nil || res.IsError {
		t.Fatalf("proxy call: %v %+v", err, res)
	}
	if !strings.Contains(fmt.Sprint(res.StructuredContent), "e_a") {
		t.Fatalf("unexpected result: %+v", res)
	}
}
