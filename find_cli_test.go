package claude

import (
	"errors"
	"strings"
	"testing"
)

func fakeFiles(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

func noLookPath(string) (string, error) { return "", errors.New("not found") }

func TestFindCLIFor_WindowsPrefersNativeExeInAbsolutePathEntries(t *testing.T) {
	lookPathCalled := false
	lookPath := func(string) (string, error) {
		lookPathCalled = true
		return `C:\npm\claude.cmd`, nil
	}
	pathEnv := `.;bin;C:drive-relative;C:\npm;D:\tools\;\\server\share`
	isFile := fakeFiles(
		`.\claude.exe`, `bin\claude.exe`, `C:drive-relative\claude.exe`, // relative: never used
		`D:\tools\claude.exe`,
		`\\server\share\claude.exe`,
	)
	got, err := findCLIFor("windows", pathEnv, `C:\Users\me`, lookPath, isFile)
	if err != nil {
		t.Fatal(err)
	}
	if got != `D:\tools\claude.exe` {
		t.Errorf("got %q, want the first claude.exe in an absolute PATH entry", got)
	}
	if lookPathCalled {
		t.Error("exec.LookPath must not be used on Windows (it can return the claude.cmd shim)")
	}
}

func TestFindCLIFor_WindowsUNCAndHomeFallback(t *testing.T) {
	got, err := findCLIFor("windows", `\\server\share`, `C:\Users\me`, noLookPath, fakeFiles(`\\server\share\claude.exe`))
	if err != nil || got != `\\server\share\claude.exe` {
		t.Fatalf("UNC entry: got %q, %v", got, err)
	}
	got, err = findCLIFor("windows", `C:\npm`, `C:\Users\me`, noLookPath, fakeFiles(`C:\Users\me\.local\bin\claude.exe`))
	if err != nil || got != `C:\Users\me\.local\bin\claude.exe` {
		t.Fatalf("home fallback: got %q, %v", got, err)
	}
}

func TestFindCLIFor_WindowsNotFoundSkipsPOSIXLocations(t *testing.T) {
	_, err := findCLIFor("windows", `C:\npm`, `C:\Users\me`, noLookPath, fakeFiles("/usr/local/bin/claude", `C:\npm\claude.cmd`))
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want *NotFoundError, got %v", err)
	}
	if !strings.Contains(err.Error(), "native claude.exe") || !strings.Contains(err.Error(), "claude.cmd shim") {
		t.Errorf("want Windows install guidance, got %q", err.Error())
	}
}

func TestFindCLIFor_POSIX(t *testing.T) {
	got, err := findCLIFor("linux", "", "/home/me", func(string) (string, error) { return "/opt/bin/claude", nil }, fakeFiles())
	if err != nil || got != "/opt/bin/claude" {
		t.Fatalf("PATH hit: got %q, %v", got, err)
	}
	got, err = findCLIFor("darwin", "", "/home/me", noLookPath, fakeFiles("/home/me/.local/bin/claude"))
	if err != nil || got != "/home/me/.local/bin/claude" {
		t.Fatalf("fallback: got %q, %v", got, err)
	}
	_, err = findCLIFor("linux", "", "/home/me", noLookPath, fakeFiles())
	if err == nil || !strings.Contains(err.Error(), "npm install -g") {
		t.Fatalf("want POSIX not-found guidance, got %v", err)
	}
}

func TestIsWindowsAbsPath(t *testing.T) {
	for p, want := range map[string]bool{
		`C:\x`: true, `c:/x`: true, `\\srv\share`: true, `//srv/share`: true,
		`C:x`: false, `\x`: false, `x\y`: false, ``: false, `.`: false, `1:\x`: false,
	} {
		if got := isWindowsAbsPath(p); got != want {
			t.Errorf("isWindowsAbsPath(%q) = %v, want %v", p, got, want)
		}
	}
}
