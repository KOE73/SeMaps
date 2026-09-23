package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Self-install for Windows. A downloaded exe can be run from anywhere; the
// install copies it to a stable folder, puts that folder on the user's PATH
// and associates *.semaps with it — no admin rights, everything under HKCU.

const progID = "SeMaps.Project"

func installDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "SeMaps")
}

func installedExe() string { return filepath.Join(installDir(), "semaps.exe") }

// installed reports whether the *.semaps association points at an existing exe.
func installed() bool {
	out, err := exec.Command("reg", "query", `HKCU\Software\Classes\`+progID+`\shell\open\command`, "/ve").Output()
	if err != nil {
		return false
	}
	// The value line looks like:  (Default)    REG_SZ    "C:\...\semaps.exe" "%1"
	s := string(out)
	i := strings.Index(s, `"`)
	j := strings.Index(s[i+1:], `"`)
	if i < 0 || j < 0 {
		return false
	}
	_, err = os.Stat(s[i+1 : i+1+j])
	return err == nil
}

func regAdd(args ...string) error {
	cmd := exec.Command("reg", append([]string{"add"}, append(args, "/f")...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reg add %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// userPath reads HKCU\Environment\Path without expanding it; setx would
// expand and truncate it, so the registry is written directly.
func userPath() string {
	out, err := exec.Command("reg", "query", `HKCU\Environment`, "/v", "Path").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.EqualFold(f[0], "Path") {
			return strings.TrimSpace(strings.Join(f[2:], " "))
		}
	}
	return ""
}

// broadcastEnvChange tells running shells and Explorer that PATH changed, so
// a console opened afterwards sees it without a relogin.
func broadcastEnvChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	env, _ := syscall.UTF16PtrFromString("Environment")
	const HWND_BROADCAST, WM_SETTINGCHANGE, SMTO_ABORTIFHUNG = 0xffff, 0x001A, 0x0002
	proc.Call(HWND_BROADCAST, WM_SETTINGCHANGE, 0, uintptr(unsafe.Pointer(env)), SMTO_ABORTIFHUNG, 5000, 0)
}

// install copies this exe into the install folder (unless it already runs
// from there), adds the folder to the user PATH and registers *.semaps.
func install() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	exe := installedExe()
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(exe)) {
		if err := copyFile(self, exe); err != nil {
			return fmt.Errorf("copy to %s: %w", exe, err)
		}
		fmt.Printf("Copied to %s\n", exe)
	}

	dir := installDir()
	onPath := false
	for _, p := range strings.Split(userPath(), ";") {
		if strings.EqualFold(strings.TrimSpace(p), dir) {
			onPath = true
		}
	}
	if !onPath {
		p := userPath()
		if p != "" && !strings.HasSuffix(p, ";") {
			p += ";"
		}
		if err := regAdd(`HKCU\Environment`, "/v", "Path", "/t", "REG_EXPAND_SZ", "/d", p+dir); err != nil {
			return err
		}
		broadcastEnvChange()
		fmt.Printf("Added to PATH: %s (open a new console to use `semaps`)\n", dir)
	}

	for _, a := range [][]string{
		{`HKCU\Software\Classes\.semaps`, "/ve", "/d", progID},
		{`HKCU\Software\Classes\` + progID, "/ve", "/d", "SeMaps project"},
		{`HKCU\Software\Classes\` + progID + `\DefaultIcon`, "/ve", "/d", exe + ",0"},
		{`HKCU\Software\Classes\` + progID + `\shell\open\command`, "/ve", "/d", `"` + exe + `" "%1"`},
	} {
		if err := regAdd(a...); err != nil {
			return err
		}
	}
	fmt.Println("*.semaps files now open with semaps.exe (Enter or double click).")
	return nil
}

// offerInstall is what a freshly downloaded exe does when started with
// nothing to open: asks, installs, and keeps the window until Enter so that
// a double-click launch does not vanish before it is read.
func offerInstall() {
	fmt.Print("semaps is not installed yet. Install for the current user (copy to\n" + installDir() + ", add to PATH, open *.semaps files)? [Y/n] ")
	var answer string
	fmt.Scanln(&answer)
	if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
		if err := install(); err != nil {
			fmt.Println("Install failed:", err)
		}
	}
	fmt.Print("Press Enter to close.")
	fmt.Scanln()
}
