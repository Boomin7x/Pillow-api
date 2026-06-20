package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type openapiDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

var kycPathRE = regexp.MustCompile(`^/kyc/`)

func TestKYCRoutesDocumented(t *testing.T) {
	doc := readOpenAPI(t)
	docPaths := collectDocumentedPaths(doc)

	routesFile := filepath.Join("..", "internal", "app", "router.go")
	codePaths := collectRouterPaths(t, routesFile)

	for _, p := range codePaths {
		if !hasPath(docPaths, p) {
			t.Errorf("route %s %s is registered in router.go but not documented in openapi.yaml", p.method, p.path)
		}
	}

	for _, p := range docPaths {
		if !hasPath(codePaths, p) {
			t.Errorf("path %s %s is documented in openapi.yaml but not registered in router.go", p.method, p.path)
		}
	}
}

type route struct {
	method string
	path   string
}

func readOpenAPI(t *testing.T) *openapiDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var doc openapiDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	return &doc
}

func collectDocumentedPaths(doc *openapiDoc) []route {
	var out []route
	for path, methods := range doc.Paths {
		if !kycPathRE.MatchString(path) {
			continue
		}
		normalized := normalizePath(path)
		for method := range methods {
			out = append(out, route{method: strings.ToUpper(method), path: normalized})
		}
	}
	return out
}

var pathInRoute = regexp.MustCompile(`"([^"]*)"`)

func collectRouterPaths(t *testing.T, filename string) []route {
	t.Helper()
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	var out []route
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "kycGroup.") && !strings.HasPrefix(trimmed, "f.") {
			continue
		}
		if !strings.Contains(trimmed, "/kyc") && !strings.HasPrefix(trimmed, "kycGroup.") {
			continue
		}
		method := extractMethod(trimmed)
		path := extractPath(trimmed)
		if path == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "kycGroup.") {
			path = "/kyc" + path
		}
		out = append(out, route{method: method, path: normalizePath(path)})
	}
	return out
}

func extractMethod(line string) string {
	switch {
	case strings.HasPrefix(line, "f.Get("), strings.HasPrefix(line, "kycGroup.Get("):
		return "GET"
	case strings.HasPrefix(line, "f.Post("), strings.HasPrefix(line, "kycGroup.Post("):
		return "POST"
	case strings.HasPrefix(line, "f.Delete("), strings.HasPrefix(line, "kycGroup.Delete("):
		return "DELETE"
	case strings.HasPrefix(line, "f.Put("), strings.HasPrefix(line, "kycGroup.Put("):
		return "PUT"
	case strings.HasPrefix(line, "f.Patch("), strings.HasPrefix(line, "kycGroup.Patch("):
		return "PATCH"
	}
	return ""
}

func extractPath(line string) string {
	m := pathInRoute.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

var paramRE = regexp.MustCompile(`:(\w+)`)

func normalizePath(p string) string {
	return paramRE.ReplaceAllString(p, "{$1}")
}

func hasPath(routes []route, r route) bool {
	for _, existing := range routes {
		if existing.method == r.method && existing.path == r.path {
			return true
		}
	}
	return false
}
