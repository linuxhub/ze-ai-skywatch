package main

import "testing"

func TestRedactUserPaths(t *testing.T) {
	for path, want := range map[string]string{
		"/Users/alice/bin/tool":       "/Users/<user>/bin/tool",
		`C:\Users\alice\tool.exe`:     `C:\Users\<user>\tool.exe`,
		`c:/users/alice/tool.exe`:     `c:/users/<user>/tool.exe`,
		`C:\Windows\System32\cmd.exe`: `C:\Windows\System32\cmd.exe`,
	} {
		if got := redactUserPath(path); got != want {
			t.Errorf("redacted %q = %q, want %q", path, got, want)
		}
	}
}
