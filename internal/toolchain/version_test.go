// Package toolchain contains a regression test for issue #91: the Go
// version used to be declared four different ways (go.mod, two workflow
// files, and the Dockerfile) and had drifted apart, so CI tested one
// toolchain while the binary that shipped was built with another.
package toolchain

import (
	"os"
	"regexp"
	"testing"
)

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

var goDirectiveRe = regexp.MustCompile(`(?m)^go (\d+\.\d+)`)

// TestGoModVersionMatchesDockerfile covers issue #91: go.mod and the
// Dockerfile's builder image must agree on the Go major.minor version, or
// CI tests a toolchain different from the one that produces the shipped
// binary.
func TestGoModVersionMatchesDockerfile(t *testing.T) {
	goMod := readRepoFile(t, "../../go.mod")
	m := goDirectiveRe.FindStringSubmatch(goMod)
	if m == nil {
		t.Fatal(`go.mod has no "go X.Y" directive`)
	}
	wantVersion := m[1]

	dockerfile := readRepoFile(t, "../../Dockerfile")
	dm := regexp.MustCompile(`golang:(\d+\.\d+)`).FindStringSubmatch(dockerfile)
	if dm == nil {
		t.Fatal("Dockerfile has no golang:X.Y builder image")
	}

	if dm[1] != wantVersion {
		t.Errorf("Dockerfile builds with golang:%s, go.mod declares go %s", dm[1], wantVersion)
	}
}

// TestWorkflowsReadVersionFromGoMod covers issue #91: ci.yml and
// release.yml used to hardcode their own go-version instead of reading
// go.mod, so bumping the toolchain meant editing four files and it was
// easy to miss one.
func TestWorkflowsReadVersionFromGoMod(t *testing.T) {
	hardcoded := regexp.MustCompile(`go-version:\s*['"]?\d`)
	fromGoMod := regexp.MustCompile(`go-version-file:\s*go\.mod`)

	tests := []struct {
		name string
		path string
	}{
		{"ci.yml", "../../.github/workflows/ci.yml"},
		{"release.yml", "../../.github/workflows/release.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readRepoFile(t, tt.path)
			if hardcoded.MatchString(content) {
				t.Errorf("%s hardcodes a go-version instead of reading go.mod", tt.name)
			}
			if !fromGoMod.MatchString(content) {
				t.Errorf("%s does not set go-version-file: go.mod", tt.name)
			}
		})
	}
}

// TestCIHasFormatAndVetGate covers issue #91: ci.yml ran go test and go
// build and nothing else, so unformatted code and go vet failures could
// land on main without CI ever noticing.
func TestCIHasFormatAndVetGate(t *testing.T) {
	content := readRepoFile(t, "../../.github/workflows/ci.yml")

	for _, want := range []string{"gofmt -l", "go vet ./...", "go test -race"} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(content) {
			t.Errorf("ci.yml does not run %q", want)
		}
	}
}
