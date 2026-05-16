package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAPIScanner(t *testing.T) {
	scanner := NewAPIScanner()
	if scanner == nil {
		t.Fatal("NewAPIScanner returned nil")
	}
}

func TestScanChi(t *testing.T) {
	content := `package main

import "github.com/go-chi/chi/v5"

func main() {
	r := chi.NewRouter()
	r.Get("/api/users", ListUsers)
	r.Post("/api/users", CreateUser)
	r.Put("/api/users/{id}", UpdateUser)
	r.Delete("/api/users/{id}", DeleteUser)
	r.Get("/health", HealthCheck)
}
`
	endpoints := ScanChi(content, "main.go")
	if len(endpoints) != 5 {
		t.Fatalf("expected 5 endpoints, got %d", len(endpoints))
	}

	tests := []struct {
		method  string
		path    string
		handler string
	}{
		{"GET", "/api/users", "ListUsers"},
		{"POST", "/api/users", "CreateUser"},
		{"PUT", "/api/users/{id}", "UpdateUser"},
		{"DELETE", "/api/users/{id}", "DeleteUser"},
		{"GET", "/health", "HealthCheck"},
	}

	for i, tt := range tests {
		if endpoints[i].Method != tt.method {
			t.Errorf("endpoint %d: expected method %s, got %s", i, tt.method, endpoints[i].Method)
		}
		if endpoints[i].Path != tt.path {
			t.Errorf("endpoint %d: expected path %s, got %s", i, tt.path, endpoints[i].Path)
		}
		if endpoints[i].Handler != tt.handler {
			t.Errorf("endpoint %d: expected handler %s, got %s", i, tt.handler, endpoints[i].Handler)
		}
		if endpoints[i].File != "main.go" {
			t.Errorf("endpoint %d: expected file main.go, got %s", i, endpoints[i].File)
		}
		if endpoints[i].Line == 0 {
			t.Errorf("endpoint %d: line should not be 0", i)
		}
	}
}

func TestScanNetHTTP(t *testing.T) {
	content := `package main

import "net/http"

func main() {
	http.HandleFunc("/api/hello", HelloHandler)
	http.Handle("/static/", http.FileServer(http.Dir("./static")))
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", UsersHandler)
}
`
	endpoints := ScanNetHTTP(content, "server.go")
	if len(endpoints) < 3 {
		t.Fatalf("expected at least 3 endpoints, got %d", len(endpoints))
	}

	found := false
	for _, ep := range endpoints {
		if ep.Path == "/api/hello" && ep.Method == "ANY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find /api/hello endpoint")
	}
}

func TestScanNetHTTPMethodPattern(t *testing.T) {
	content := `package main

import "net/http"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users", ListUsers)
	mux.HandleFunc("POST /api/users", CreateUser)
}
`
	endpoints := ScanNetHTTP(content, "server.go")

	foundGet := false
	foundPost := false
	for _, ep := range endpoints {
		if ep.Path == "/api/users" && ep.Method == "GET" {
			foundGet = true
		}
		if ep.Path == "/api/users" && ep.Method == "POST" {
			foundPost = true
		}
	}
	if !foundGet {
		t.Error("expected to find GET /api/users endpoint")
	}
	if !foundPost {
		t.Error("expected to find POST /api/users endpoint")
	}
}

func TestScanGin(t *testing.T) {
	content := `package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/api/users", ListUsers)
	r.POST("/api/users", CreateUser)
	r.PUT("/api/users/:id", UpdateUser)
	r.DELETE("/api/users/:id", DeleteUser)
}
`
	endpoints := ScanGin(content, "main.go")
	if len(endpoints) != 4 {
		t.Fatalf("expected 4 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Method != "GET" {
		t.Errorf("expected GET, got %s", endpoints[0].Method)
	}
	if endpoints[0].Path != "/api/users" {
		t.Errorf("expected /api/users, got %s", endpoints[0].Path)
	}
	if endpoints[0].Handler != "ListUsers" {
		t.Errorf("expected ListUsers, got %s", endpoints[0].Handler)
	}
}

func TestScanGinGroup(t *testing.T) {
	content := `package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	api := r.Group("/api/v1")
	api.GET("/users", ListUsers)
	api.POST("/users", CreateUser)
}
`
	endpoints := ScanGin(content, "main.go")
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	// Group prefix should be applied
	if endpoints[0].Path != "/api/v1/users" {
		t.Errorf("expected /api/v1/users, got %s", endpoints[0].Path)
	}
}

func TestScanEcho(t *testing.T) {
	content := `package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.GET("/users", GetUsers)
	e.POST("/users", CreateUser)
}
`
	endpoints := ScanEcho(content, "main.go")
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Method != "GET" || endpoints[0].Path != "/users" {
		t.Errorf("unexpected first endpoint: %+v", endpoints[0])
	}
}

func TestScanEchoNoImport(t *testing.T) {
	// Without echo import, should return nil
	content := `package main

func main() {
	e.GET("/users", GetUsers)
}
`
	endpoints := ScanEcho(content, "main.go")
	if endpoints != nil {
		t.Errorf("expected nil endpoints when echo not imported, got %d", len(endpoints))
	}
}

func TestScanGorilla(t *testing.T) {
	content := `package main

import "github.com/gorilla/mux"

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/users", ListUsers).Methods("GET")
	r.HandleFunc("/api/users", CreateUser).Methods("POST")
	r.HandleFunc("/api/users/{id}", GetUser).Methods("GET")
}
`
	endpoints := ScanGorilla(content, "main.go")
	if len(endpoints) < 3 {
		t.Fatalf("expected at least 3 endpoints, got %d", len(endpoints))
	}

	foundGetUsers := false
	for _, ep := range endpoints {
		if ep.Path == "/api/users" && ep.Method == "GET" && ep.Handler == "ListUsers" {
			foundGetUsers = true
		}
	}
	if !foundGetUsers {
		t.Error("expected to find GET /api/users -> ListUsers")
	}
}

func TestScanFiber(t *testing.T) {
	content := `package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()
	app.Get("/api/users", ListUsers)
	app.Post("/api/users", CreateUser)
	app.Delete("/api/users/:id", DeleteUser)
}
`
	endpoints := ScanFiber(content, "main.go")
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Method != "GET" || endpoints[0].Path != "/api/users" {
		t.Errorf("unexpected first endpoint: %+v", endpoints[0])
	}
}

func TestScanFiberNoImport(t *testing.T) {
	content := `package main

func main() {
	app.Get("/api/users", ListUsers)
}
`
	endpoints := ScanFiber(content, "main.go")
	if endpoints != nil {
		t.Errorf("expected nil when fiber not imported, got %d", len(endpoints))
	}
}

func TestDetectFramework(t *testing.T) {
	dir := t.TempDir()

	// Create a go.mod with chi dependency
	goMod := `module example.com/test

go 1.21

require github.com/go-chi/chi/v5 v5.0.10
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	framework := DetectFramework(dir)
	if framework != "chi" {
		t.Errorf("expected chi, got %s", framework)
	}
}

func TestDetectFrameworkGin(t *testing.T) {
	dir := t.TempDir()

	goMod := `module example.com/test

go 1.21

require github.com/gin-gonic/gin v1.9.1
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	framework := DetectFramework(dir)
	if framework != "gin" {
		t.Errorf("expected gin, got %s", framework)
	}
}

func TestDetectFrameworkFromSource(t *testing.T) {
	dir := t.TempDir()

	// No go.mod, but source files import echo
	src := `package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	framework := DetectFramework(dir)
	if framework != "echo" {
		t.Errorf("expected echo, got %s", framework)
	}
}

func TestDetectFrameworkUnknown(t *testing.T) {
	dir := t.TempDir()

	framework := DetectFramework(dir)
	if framework != "unknown" {
		t.Errorf("expected unknown, got %s", framework)
	}
}

func TestScanProject(t *testing.T) {
	dir := t.TempDir()

	// Create a simple chi project
	goMod := `module example.com/test

go 1.21

require github.com/go-chi/chi/v5 v5.0.10
`
	mainGo := `package main

import "github.com/go-chi/chi/v5"

func main() {
	r := chi.NewRouter()
	r.Get("/api/users", ListUsers)
	r.Post("/api/users", CreateUser)
	r.Get("/api/users/{id}", GetUser)
	r.Put("/api/users/{id}", UpdateUser)
	r.Delete("/api/users/{id}", DeleteUser)
	r.Get("/health", HealthCheck)
}

func ListUsers() {}
func CreateUser() {}
func GetUser() {}
func UpdateUser() {}
func DeleteUser() {}
func HealthCheck() {}
`

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewAPIScanner()
	apiMap, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("ScanProject failed: %v", err)
	}

	if len(apiMap.Endpoints) != 6 {
		t.Fatalf("expected 6 endpoints, got %d", len(apiMap.Endpoints))
	}

	// Verify sorted output
	if apiMap.Endpoints[0].Path != "/api/users" {
		t.Errorf("expected first path to be /api/users, got %s", apiMap.Endpoints[0].Path)
	}
}

func TestScanProjectSkipsVendor(t *testing.T) {
	dir := t.TempDir()

	// Create vendor directory with routes that should be ignored
	vendorDir := filepath.Join(dir, "vendor", "somelib")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}

	vendorFile := `package somelib

func init() {
	r.Get("/vendor/route", VendorHandler)
}
`
	mainFile := `package main

import "github.com/go-chi/chi/v5"

func main() {
	r := chi.NewRouter()
	r.Get("/api/users", ListUsers)
}
`

	if err := os.WriteFile(filepath.Join(vendorDir, "routes.go"), []byte(vendorFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainFile), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewAPIScanner()
	apiMap, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, ep := range apiMap.Endpoints {
		if strings.Contains(ep.Path, "vendor") {
			t.Errorf("should not include vendor routes, found: %s", ep.Path)
		}
	}
}

func TestScanProjectSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	testFile := `package main

import "github.com/go-chi/chi/v5"

func TestRoutes() {
	r := chi.NewRouter()
	r.Get("/test/route", TestHandler)
}
`
	mainFile := `package main

import "github.com/go-chi/chi/v5"

func main() {
	r := chi.NewRouter()
	r.Get("/api/users", ListUsers)
}
`

	if err := os.WriteFile(filepath.Join(dir, "routes_test.go"), []byte(testFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainFile), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewAPIScanner()
	apiMap, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, ep := range apiMap.Endpoints {
		if strings.Contains(ep.Path, "test") {
			t.Errorf("should not include test file routes, found: %s", ep.Path)
		}
	}
}

func TestFormatAPIMap(t *testing.T) {
	apiMap := &APIMap{
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/api/v1/users", Handler: "handler.ListUsers"},
			{Method: "POST", Path: "/api/v1/users", Handler: "handler.CreateUser"},
			{Method: "GET", Path: "/api/v1/users/:id", Handler: "handler.GetUser"},
			{Method: "DELETE", Path: "/api/v1/users/:id", Handler: "handler.DeleteUser"},
			{Method: "GET", Path: "/health", Handler: "handler.HealthCheck"},
		},
	}

	output := FormatAPIMap(apiMap)

	if !strings.Contains(output, "API Endpoints (5):") {
		t.Errorf("expected header with count 5, got:\n%s", output)
	}
	if !strings.Contains(output, "═") {
		t.Error("expected separator line")
	}
	if !strings.Contains(output, "GET") {
		t.Error("expected GET method in output")
	}
	if !strings.Contains(output, "/api/v1/users") {
		t.Error("expected path in output")
	}
	if !strings.Contains(output, "→") {
		t.Error("expected arrow in output")
	}
	if !strings.Contains(output, "handler.ListUsers") {
		t.Error("expected handler name in output")
	}
}

func TestFormatAPIMapEmpty(t *testing.T) {
	apiMap := &APIMap{}
	output := FormatAPIMap(apiMap)
	if !strings.Contains(output, "No endpoints found") {
		t.Errorf("expected 'No endpoints found', got:\n%s", output)
	}
}

func TestFormatAPIMapNil(t *testing.T) {
	output := FormatAPIMap(nil)
	if !strings.Contains(output, "No endpoints found") {
		t.Errorf("expected 'No endpoints found', got:\n%s", output)
	}
}

func TestGenerateOpenAPI(t *testing.T) {
	apiMap := &APIMap{
		Version: "1.0.0",
		BaseURL: "https://api.example.com",
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/users", Handler: "ListUsers", Description: "List all users"},
			{Method: "POST", Path: "/users", Handler: "CreateUser"},
			{Method: "GET", Path: "/users/{id}", Handler: "GetUser"},
		},
	}

	output := GenerateOpenAPI(apiMap)

	if !strings.Contains(output, `openapi: "3.0.0"`) {
		t.Error("expected openapi version")
	}
	if !strings.Contains(output, `version: "1.0.0"`) {
		t.Error("expected API version")
	}
	if !strings.Contains(output, `url: "https://api.example.com"`) {
		t.Error("expected server URL")
	}
	if !strings.Contains(output, "paths:") {
		t.Error("expected paths section")
	}
	if !strings.Contains(output, "/users:") {
		t.Error("expected /users path")
	}
	if !strings.Contains(output, "get:") {
		t.Error("expected get method")
	}
	if !strings.Contains(output, "post:") {
		t.Error("expected post method")
	}
	if !strings.Contains(output, `summary: "List all users"`) {
		t.Error("expected description as summary")
	}
	if !strings.Contains(output, "responses:") {
		t.Error("expected responses section")
	}
}

func TestGenerateOpenAPINil(t *testing.T) {
	output := GenerateOpenAPI(nil)
	if output != "" {
		t.Errorf("expected empty string for nil map, got: %s", output)
	}
}

func TestFindEndpointByPath(t *testing.T) {
	apiMap := &APIMap{
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/api/users", Handler: "ListUsers"},
			{Method: "POST", Path: "/api/users", Handler: "CreateUser"},
			{Method: "GET", Path: "/api/users/{id}", Handler: "GetUser"},
			{Method: "GET", Path: "/health", Handler: "HealthCheck"},
		},
	}

	ep := apiMap.FindEndpointByPath("/api/users")
	if ep == nil {
		t.Fatal("expected to find /api/users endpoint")
	}
	if ep.Handler != "ListUsers" {
		t.Errorf("expected ListUsers handler, got %s", ep.Handler)
	}

	ep = apiMap.FindEndpointByPath("/health")
	if ep == nil {
		t.Fatal("expected to find /health endpoint")
	}
	if ep.Handler != "HealthCheck" {
		t.Errorf("expected HealthCheck handler, got %s", ep.Handler)
	}

	ep = apiMap.FindEndpointByPath("/nonexistent")
	if ep != nil {
		t.Error("expected nil for nonexistent path")
	}
}

func TestAPIEndpointStruct(t *testing.T) {
	ep := APIEndpoint{
		Method:      "GET",
		Path:        "/api/users",
		Handler:     "handler.ListUsers",
		File:        "routes.go",
		Line:        42,
		Middleware:  []string{"auth", "logging"},
		Description: "List all users",
	}

	if ep.Method != "GET" {
		t.Error("Method field incorrect")
	}
	if ep.Path != "/api/users" {
		t.Error("Path field incorrect")
	}
	if ep.Handler != "handler.ListUsers" {
		t.Error("Handler field incorrect")
	}
	if ep.File != "routes.go" {
		t.Error("File field incorrect")
	}
	if ep.Line != 42 {
		t.Error("Line field incorrect")
	}
	if len(ep.Middleware) != 2 {
		t.Error("Middleware field incorrect")
	}
	if ep.Description != "List all users" {
		t.Error("Description field incorrect")
	}
}

func TestAPIMapStruct(t *testing.T) {
	apiMap := &APIMap{
		BaseURL: "https://api.example.com",
		Version: "2.0.0",
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/test"},
		},
	}

	if apiMap.BaseURL != "https://api.example.com" {
		t.Error("BaseURL field incorrect")
	}
	if apiMap.Version != "2.0.0" {
		t.Error("Version field incorrect")
	}
	if len(apiMap.Endpoints) != 1 {
		t.Error("Endpoints field incorrect")
	}
}

func TestCleanHandler(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ListUsers", "ListUsers"},
		{"  ListUsers  ", "ListUsers"},
		{"ListUsers,", "ListUsers"},
		{"handler.ListUsers", "handler.ListUsers"},
	}

	for _, tt := range tests {
		got := cleanHandler(tt.input)
		if got != tt.expected {
			t.Errorf("cleanHandler(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCountLinesAt(t *testing.T) {
	content := "line1\nline2\nline3\nline4"

	if got := countLinesAt(content, 0); got != 1 {
		t.Errorf("expected line 1, got %d", got)
	}
	if got := countLinesAt(content, 6); got != 2 {
		t.Errorf("expected line 2, got %d", got)
	}
	if got := countLinesAt(content, 12); got != 3 {
		t.Errorf("expected line 3, got %d", got)
	}
}

func TestDeduplicateEndpoints(t *testing.T) {
	endpoints := []APIEndpoint{
		{Method: "GET", Path: "/api/users", Handler: "ListUsers", File: "main.go"},
		{Method: "GET", Path: "/api/users", Handler: "ListUsers", File: "main.go"},
		{Method: "POST", Path: "/api/users", Handler: "CreateUser", File: "main.go"},
	}

	result := deduplicateEndpoints(endpoints)
	if len(result) != 2 {
		t.Errorf("expected 2 unique endpoints, got %d", len(result))
	}
}

func TestScanProjectNonexistentDir(t *testing.T) {
	scanner := NewAPIScanner()
	_, err := scanner.ScanProject("/nonexistent/path/12345")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestScanProjectEmptyDir(t *testing.T) {
	dir := t.TempDir()

	scanner := NewAPIScanner()
	apiMap, err := scanner.ScanProject(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apiMap.Endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(apiMap.Endpoints))
	}
}

func TestScanChiWithPackageQualifiedHandlers(t *testing.T) {
	content := `package main

import "github.com/go-chi/chi/v5"

func main() {
	r := chi.NewRouter()
	r.Get("/api/users", handler.ListUsers)
	r.Post("/api/users", handler.CreateUser)
}
`
	endpoints := ScanChi(content, "main.go")
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Handler != "handler.ListUsers" {
		t.Errorf("expected handler.ListUsers, got %s", endpoints[0].Handler)
	}
}

func TestFormatAPIMapAlignment(t *testing.T) {
	apiMap := &APIMap{
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/short", Handler: "H1"},
			{Method: "DELETE", Path: "/very/long/path/here", Handler: "H2"},
		},
	}

	output := FormatAPIMap(apiMap)
	lines := strings.Split(output, "\n")

	// Should have header, separator, and 2 endpoint lines
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d:\n%s", len(lines), output)
	}
}

func TestGenerateOpenAPINoVersion(t *testing.T) {
	apiMap := &APIMap{
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/test", Handler: "TestHandler"},
		},
	}

	output := GenerateOpenAPI(apiMap)
	if !strings.Contains(output, `version: "1.0.0"`) {
		t.Error("expected default version 1.0.0")
	}
}

func TestGenerateOpenAPINoBaseURL(t *testing.T) {
	apiMap := &APIMap{
		Endpoints: []APIEndpoint{
			{Method: "GET", Path: "/test", Handler: "TestHandler"},
		},
	}

	output := GenerateOpenAPI(apiMap)
	if strings.Contains(output, "servers:") {
		t.Error("should not include servers section when no BaseURL")
	}
}
