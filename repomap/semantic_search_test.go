package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSemanticSearchIndex(t *testing.T) {
	idx := NewSemanticSearchIndex()
	if idx == nil {
		t.Fatal("NewSemanticSearchIndex returned nil")
	}
	if idx.Documents == nil {
		t.Error("Documents map not initialized")
	}
	if idx.IDF == nil {
		t.Error("IDF map not initialized")
	}
	if idx.TotalDocs != 0 {
		t.Errorf("expected TotalDocs=0, got %d", idx.TotalDocs)
	}
}

func TestIndexFile(t *testing.T) {
	idx := NewSemanticSearchIndex()

	content := `package auth

import "net/http"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ValidateToken(token string) (*Claims, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	claims, err := parseJWT(token)
	if err != nil {
		return nil, err
	}
	return claims, nil
}
`
	idx.IndexFile("src/auth/middleware.go", content)

	if idx.TotalDocs == 0 {
		t.Error("no documents indexed")
	}
	if idx.AvgDocLen == 0 {
		t.Error("average document length is 0")
	}

	// Check that documents were created
	foundFunc := false
	for _, doc := range idx.Documents {
		if doc.Type == "function" {
			foundFunc = true
			break
		}
	}
	if !foundFunc {
		t.Error("expected at least one function document")
	}
}

func TestSearch(t *testing.T) {
	idx := NewSemanticSearchIndex()

	idx.IndexFile("src/auth/middleware.go", `package auth

func AuthMiddleware(next http.Handler) http.Handler {
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	next.ServeHTTP(w, r)
}
`)

	idx.IndexFile("src/handler/api.go", `package handler

func requireAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		handler(w, r)
	}
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	users := db.GetAllUsers()
	json.NewEncoder(w).Encode(users)
}
`)

	idx.IndexFile("src/db/connection.go", `package db

func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}
`)

	hits := idx.Search("authentication middleware", 5)
	if len(hits) == 0 {
		t.Fatal("search returned no results for 'authentication middleware'")
	}

	// The auth middleware file should rank high
	topHit := hits[0]
	if topHit.Score <= 0 {
		t.Error("top hit score should be positive")
	}
	if !strings.Contains(topHit.Document.Path, "auth") && !strings.Contains(topHit.Document.Path, "handler") {
		t.Errorf("expected auth-related file in top hit, got %s", topHit.Document.Path)
	}

	// Database query should not rank high for auth query
	for _, hit := range hits {
		if strings.Contains(hit.Document.Path, "db/connection") && hit.Document.Type == "function" {
			if hit.Score > topHit.Score {
				t.Error("database code should not rank above auth code for auth query")
			}
		}
	}
}

func TestSearchByIntent(t *testing.T) {
	idx := NewSemanticSearchIndex()

	idx.IndexFile("src/auth/token.go", `package auth

func ValidateToken(tokenStr string) (*Claims, error) {
	claims, err := jwt.Parse(tokenStr, getSecretKey)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func GenerateJWT(userID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})
	return token.SignedString(getSecretKey())
}
`)

	idx.IndexFile("src/math/calc.go", `package math

func Add(a, b int) int {
	return a + b
}

func Multiply(a, b int) int {
	return a * b
}
`)

	hits := idx.SearchByIntent("authentication")
	if len(hits) == 0 {
		t.Fatal("SearchByIntent returned no results for 'authentication'")
	}

	// Token/JWT code should appear in results
	foundAuth := false
	for _, hit := range hits {
		if strings.Contains(hit.Document.Path, "auth/token") {
			foundAuth = true
			break
		}
	}
	if !foundAuth {
		t.Error("expected auth/token.go in results for 'authentication' intent")
	}
}

func TestExpandQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected []string // at least these should be present
	}{
		{
			query:    "auth",
			expected: []string{"auth", "authenticate", "authorization", "login", "token", "jwt"},
		},
		{
			query:    "test",
			expected: []string{"test", "spec", "assert", "expect", "verify"},
		},
		{
			query:    "error",
			expected: []string{"error", "err", "fail", "panic", "exception"},
		},
		{
			query:    "database",
			expected: []string{"database", "db", "sql", "query"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			expanded := ExpandQuery(tc.query)
			expandedSet := make(map[string]bool)
			for _, term := range expanded {
				expandedSet[term] = true
			}

			for _, exp := range tc.expected {
				if !expandedSet[exp] {
					t.Errorf("ExpandQuery(%q) missing expected term %q; got %v", tc.query, exp, expanded)
				}
			}
		})
	}
}

func TestBM25Score(t *testing.T) {
	idx := NewSemanticSearchIndex()

	// Add a document with known terms
	doc1 := &Document{
		ID:      "doc1",
		Path:    "test.go",
		Content: "func authenticate(user string) error",
		Terms:   map[string]int{"func": 1, "authenticate": 1, "user": 1, "string": 1, "error": 1},
		Length:  5,
		Type:    "function",
	}
	doc2 := &Document{
		ID:      "doc2",
		Path:    "other.go",
		Content: "func processData(data []byte) []byte",
		Terms:   map[string]int{"func": 1, "processdata": 1, "data": 2, "byte": 2},
		Length:  6,
		Type:    "function",
	}

	idx.Documents["doc1"] = doc1
	idx.Documents["doc2"] = doc2
	idx.RebuildIndex()

	queryTerms := []string{"authenticate", "user", "error"}

	score1 := idx.BM25Score(queryTerms, doc1)
	score2 := idx.BM25Score(queryTerms, doc2)

	if score1 <= 0 {
		t.Error("expected positive score for doc1 with matching terms")
	}
	if score2 != 0 {
		t.Errorf("expected zero score for doc2 with no matching terms, got %f", score2)
	}
	if score1 <= score2 {
		t.Error("doc1 should score higher than doc2 for auth-related query")
	}
}

func TestExtractSnippet(t *testing.T) {
	doc := &Document{
		ID:   "test",
		Path: "auth.go",
		Content: `package auth

import "net/http"

// AuthMiddleware validates the authentication token
func AuthMiddleware(next http.Handler) http.Handler {
	token := r.Header.Get("Authorization")
	if token == "" {
		return unauthorized(w)
	}
	next.ServeHTTP(w, r)
}`,
		Terms:  map[string]int{"auth": 1, "middleware": 1, "token": 2},
		Length: 20,
		Type:   "function",
	}

	snippet := ExtractSnippet(doc, []string{"auth", "middleware", "token"}, 120)
	if snippet == "" {
		t.Error("expected non-empty snippet")
	}
	// Should contain auth-related content
	snippetLower := strings.ToLower(snippet)
	if !strings.Contains(snippetLower, "auth") && !strings.Contains(snippetLower, "token") {
		t.Errorf("snippet should contain relevant terms, got: %q", snippet)
	}
}

func TestFormatResults(t *testing.T) {
	hits := []SearchHit{
		{
			Document: &Document{
				ID:   "src/auth/middleware.go:AuthMiddleware",
				Path: "src/auth/middleware.go",
				Type: "function",
			},
			Score:        0.94,
			MatchedTerms: []string{"auth", "middleware"},
			Snippet:      "func AuthMiddleware(next http.Handler) http.Handler {",
		},
		{
			Document: &Document{
				ID:   "src/auth/token.go:ValidateToken",
				Path: "src/auth/token.go",
				Type: "function",
			},
			Score:        0.82,
			MatchedTerms: []string{"token", "validate"},
			Snippet:      "func ValidateToken(token string) (*Claims, error) {",
		},
	}

	result := FormatResults("authentication middleware", hits)

	if !strings.Contains(result, "authentication middleware") {
		t.Error("formatted output should contain the query")
	}
	if !strings.Contains(result, "0.94") {
		t.Error("formatted output should contain first score")
	}
	if !strings.Contains(result, "0.82") {
		t.Error("formatted output should contain second score")
	}
	if !strings.Contains(result, "src/auth/middleware.go") {
		t.Error("formatted output should contain file path")
	}
	if !strings.Contains(result, "AuthMiddleware") {
		t.Error("formatted output should contain function name")
	}

	// Test empty results
	empty := FormatResults("nothing", nil)
	if !strings.Contains(empty, "No results found") {
		t.Error("empty results should show 'No results found'")
	}
}

func TestIndexDirectory(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()

	// Create subdirectories
	authDir := filepath.Join(tmpDir, "src", "auth")
	dbDir := filepath.Join(tmpDir, "src", "db")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test files
	authFile := filepath.Join(authDir, "auth.go")
	if err := os.WriteFile(authFile, []byte(`package auth

func Login(username, password string) (*Session, error) {
	user, err := findUser(username)
	if err != nil {
		return nil, err
	}
	if !checkPassword(user, password) {
		return nil, ErrInvalidCredentials
	}
	return createSession(user)
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	dbFile := filepath.Join(dbDir, "db.go")
	if err := os.WriteFile(dbFile, []byte(`package db

func Query(sql string, args ...interface{}) (*Rows, error) {
	return pool.Query(sql, args...)
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	idx := NewSemanticSearchIndex()
	err := idx.IndexDirectory(tmpDir)
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	if idx.TotalDocs == 0 {
		t.Error("expected documents after indexing directory")
	}

	// Search should work after indexing
	hits := idx.Search("login authentication", 5)
	if len(hits) == 0 {
		t.Error("expected results after indexing directory")
	}
}

func TestRebuildIndex(t *testing.T) {
	idx := NewSemanticSearchIndex()

	// Add documents manually
	idx.Documents["doc1"] = &Document{
		ID:      "doc1",
		Path:    "a.go",
		Content: "func hello() string { return \"hello\" }",
		Terms:   map[string]int{"func": 1, "hello": 2, "string": 1, "return": 1},
		Length:  5,
		Type:    "function",
	}
	idx.Documents["doc2"] = &Document{
		ID:      "doc2",
		Path:    "b.go",
		Content: "func world() string { return \"world\" }",
		Terms:   map[string]int{"func": 1, "world": 2, "string": 1, "return": 1},
		Length:  5,
		Type:    "function",
	}

	err := idx.RebuildIndex()
	if err != nil {
		t.Fatalf("RebuildIndex failed: %v", err)
	}

	if idx.TotalDocs != 2 {
		t.Errorf("expected TotalDocs=2, got %d", idx.TotalDocs)
	}
	if idx.AvgDocLen != 5.0 {
		t.Errorf("expected AvgDocLen=5.0, got %f", idx.AvgDocLen)
	}
	if len(idx.IDF) == 0 {
		t.Error("IDF should be populated after rebuild")
	}

	// Terms appearing in both docs should have lower IDF
	// "func" appears in both, "hello" in one
	idfFunc := idx.IDF["func"]
	idfHello := idx.IDF["hello"]
	if idfFunc >= idfHello {
		t.Errorf("IDF of common term 'func' (%f) should be less than rare term 'hello' (%f)", idfFunc, idfHello)
	}
}

func TestTokenizeSearch(t *testing.T) {
	tests := []struct {
		input         string
		mustContain   []string
		mustNotContain []string
	}{
		{
			input:       "AuthMiddleware",
			mustContain: []string{"authmiddleware", "auth", "middleware"},
		},
		{
			input:       "validate_token",
			mustContain: []string{"validate", "token"},
		},
		{
			input:       "func handleRequest(w http.ResponseWriter)",
			mustContain: []string{"func", "http"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := tokenizeSearch(tc.input)
			gotSet := make(map[string]bool)
			for _, tok := range got {
				gotSet[tok] = true
			}

			for _, required := range tc.mustContain {
				if !gotSet[required] {
					t.Errorf("tokenizeSearch(%q) = %v, missing expected token %q", tc.input, got, required)
				}
			}
			for _, excluded := range tc.mustNotContain {
				if gotSet[excluded] {
					t.Errorf("tokenizeSearch(%q) = %v, should not contain %q", tc.input, got, excluded)
				}
			}
		})
	}
}

func TestSearchEmptyIndex(t *testing.T) {
	idx := NewSemanticSearchIndex()
	hits := idx.Search("anything", 5)
	if len(hits) != 0 {
		t.Error("search on empty index should return empty results")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := NewSemanticSearchIndex()
	idx.IndexFile("test.go", "func hello() { }")
	hits := idx.Search("", 5)
	if len(hits) != 0 {
		t.Error("empty query should return empty results")
	}
}

func TestSemanticSearchConcurrentAccess(t *testing.T) {
	idx := NewSemanticSearchIndex()

	// Index some files first
	idx.IndexFile("a.go", `package a
func Alpha() string { return "alpha" }
func Beta() string { return "beta" }
`)

	// Concurrent reads should not panic
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = idx.Search("alpha", 5)
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDocumentTypes(t *testing.T) {
	idx := NewSemanticSearchIndex()

	content := `package example

type UserService struct {
	db *sql.DB
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) GetUser(id string) (*User, error) {
	return s.db.Query("SELECT * FROM users WHERE id = ?", id)
}
`
	idx.IndexFile("service.go", content)

	foundType := false
	foundFunc := false
	for _, doc := range idx.Documents {
		if doc.Type == "type" {
			foundType = true
		}
		if doc.Type == "function" {
			foundFunc = true
		}
	}

	if !foundType {
		t.Error("expected to find a type document")
	}
	if !foundFunc {
		t.Error("expected to find a function document")
	}
}

func TestExpandQueryUnknownTerm(t *testing.T) {
	// Unknown terms should still return at least the original term
	expanded := ExpandQuery("xyzabc123")
	if len(expanded) == 0 {
		t.Error("ExpandQuery should return at least the original term")
	}
	if expanded[0] != "xyzabc123" {
		t.Errorf("first expanded term should be the original, got %q", expanded[0])
	}
}

func TestSearchRelevanceRanking(t *testing.T) {
	idx := NewSemanticSearchIndex()

	// File with high relevance to "error handling"
	idx.IndexFile("errors.go", `package errors

func HandleError(err error) {
	if err != nil {
		log.Printf("error occurred: %v", err)
		panic(err)
	}
}

func WrapError(err error, msg string) error {
	return fmt.Errorf("%s: %w", msg, err)
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
`)

	// File with low relevance
	idx.IndexFile("math.go", `package math

func Add(a, b float64) float64 {
	return a + b
}

func Subtract(a, b float64) float64 {
	return a - b
}
`)

	hits := idx.Search("error handling", 10)
	if len(hits) == 0 {
		t.Fatal("expected results for 'error handling'")
	}

	// The error-related file should dominate top results
	topPath := hits[0].Document.Path
	if !strings.Contains(topPath, "errors.go") {
		t.Errorf("expected errors.go in top result, got %s", topPath)
	}
}
