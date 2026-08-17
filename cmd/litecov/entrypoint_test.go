package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// entrypointSource is entrypoint.sh's own path relative to this package, so
// the test below exercises the real script instead of a copy that could
// drift from it.
const entrypointSource = "../../entrypoint.sh"

// litecovToken is the absolute path entrypoint.sh invokes. buildEntrypoint
// rewrites it to a stub so the test needs neither a real litecov build nor
// a writable "/".
const litecovToken = "/litecov"

// markerToken is a placeholder swapped for a per-test marker file path in
// both an env value and its wantArgs entry, so a payload's expected argv
// and its "did this actually run a command" check share one temp dir.
const markerToken = "%MARKER%"

// buildEntrypoint writes entrypointSource into dir with litecovToken
// replaced by a stub that prints each argv it receives as "ARG:<value>",
// one per line, and returns the patched script's path.
func buildEntrypoint(t *testing.T, dir string) string {
	t.Helper()

	src, err := os.ReadFile(entrypointSource)
	if err != nil {
		t.Fatalf("reading %s: %v", entrypointSource, err)
	}
	if strings.Count(string(src), litecovToken) != 1 {
		t.Fatalf("expected exactly one %q in %s; entrypoint.sh's invocation of the binary changed shape", litecovToken, entrypointSource)
	}

	stub := filepath.Join(dir, "litecov-stub")
	stubBody := "#!/bin/sh\nfor a in \"$@\"; do\n\tprintf 'ARG:%s\\n' \"$a\"\ndone\n"
	if err := os.WriteFile(stub, []byte(stubBody), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}

	script := filepath.Join(dir, "entrypoint.sh")
	patched := strings.Replace(string(src), litecovToken, stub, 1)
	if err := os.WriteFile(script, []byte(patched), 0o755); err != nil {
		t.Fatalf("writing patched entrypoint.sh: %v", err)
	}
	return script
}

// runEntrypoint runs the patched entrypoint.sh with only the given INPUT_*
// variables set on top of the test process's own environment, and returns
// the argv the stub received.
func runEntrypoint(t *testing.T, script string, env map[string]string) []string {
	t.Helper()

	cmd := exec.Command("sh", script)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "INPUT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("entrypoint.sh failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("running entrypoint.sh: %v", err)
	}

	trimmed := strings.TrimRight(string(out), "\n")
	if trimmed == "" {
		return nil
	}
	var args []string
	for _, line := range strings.Split(trimmed, "\n") {
		args = append(args, strings.TrimPrefix(line, "ARG:"))
	}
	return args
}

// TestEntrypointPassesInputsAsArgv guards issue #83: entrypoint.sh used to
// concatenate every INPUT_* value into one string and hand it to `eval`,
// which re-parses that string as shell source. Any input value could run
// commands, e.g. a workflow that interpolates
// `title: "Coverage for ${{ github.event.pull_request.title }}"` on
// pull_request_target handed an attacker a shell. Each case below injects
// the kind of value a real title or path could carry and checks that it
// reaches /litecov unchanged, as one argv entry, with nothing executed.
func TestEntrypointPassesInputsAsArgv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "coverage-file command substitution is passed through, not executed",
			env:  map[string]string{"INPUT_COVERAGE_FILE": "$(touch " + markerToken + ")cov.lcov"},
			want: []string{"-coverage-file=$(touch " + markerToken + ")cov.lcov"},
		},
		{
			name: "show-files semicolon does not start a second command",
			env:  map[string]string{"INPUT_SHOW_FILES": "all; touch " + markerToken},
			want: []string{"-show-files=all; touch " + markerToken},
		},
		{
			name: "threshold semicolon does not start a second command",
			env:  map[string]string{"INPUT_THRESHOLD": "0; touch " + markerToken},
			want: []string{"-threshold=0; touch " + markerToken},
		},
		{
			name: "title command substitution is passed through, not executed",
			env:  map[string]string{"INPUT_TITLE": "Coverage $(id -u)"},
			want: []string{"-title=Coverage $(id -u)"},
		},
		{
			name: "title backticks are passed through, not executed",
			env:  map[string]string{"INPUT_TITLE": "Coverage `id -u`"},
			want: []string{"-title=Coverage `id -u`"},
		},
		{
			name: "title double quotes survive intact",
			env:  map[string]string{"INPUT_TITLE": `He said "hi" to me`},
			want: []string{`-title=He said "hi" to me`},
		},
		{
			name: "coverage-file path with a space stays one argument",
			env:  map[string]string{"INPUT_COVERAGE_FILE": "my reports/coverage.lcov"},
			want: []string{"-coverage-file=my reports/coverage.lcov"},
		},
		{
			name: "annotations still becomes a bare flag",
			env:  map[string]string{"INPUT_ANNOTATIONS": "true"},
			want: []string{"-annotations=true"},
		},
	}

	dir := t.TempDir()
	script := buildEntrypoint(t, dir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "pwned")

			env := make(map[string]string, len(tt.env))
			for k, v := range tt.env {
				env[k] = strings.ReplaceAll(v, markerToken, marker)
			}
			want := make([]string, len(tt.want))
			for i, w := range tt.want {
				want[i] = strings.ReplaceAll(w, markerToken, marker)
			}

			got := runEntrypoint(t, script, env)
			if !slices.Equal(got, want) {
				t.Errorf("argv = %q, want %q", got, want)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Errorf("marker file %s exists: an injected shell command actually ran", marker)
			}
		})
	}
}
