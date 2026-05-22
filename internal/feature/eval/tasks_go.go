package eval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// GoTasks returns a BenchmarkSuite with 15 Go coding tasks.
func GoTasks() *BenchmarkSuite {
	return &BenchmarkSuite{
		Name: "Go Coding Tasks",
		Tasks: []BenchmarkTask{
			taskFixNilPointer(),
			taskAddErrorHandling(),
			taskImplementInterface(),
			taskWriteUnitTest(),
			taskRefactorDuplicate(),
			taskFixRaceCondition(),
			taskImplementSort(),
			taskParseJSON(),
			taskAddContextCancellation(),
			taskFixOffByOne(),
			taskImplementRetryBackoff(),
			taskCallbackToChannel(),
			taskAddInputValidation(),
			taskFixGoroutineLeak(),
			taskImplementHTTPHandler(),
		},
	}
}

// helperWriteFile writes content to a file at the given path inside workDir.
func helperWriteFile(workDir, relPath, content string) error {
	fullPath := filepath.Join(workDir, relPath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0o644)
}

// helperInitModule initializes a Go module in the work directory.
func helperInitModule(workDir, modName string) error {
	goMod := "module " + modName + "\n\ngo 1.21\n"
	return helperWriteFile(workDir, "go.mod", goMod)
}

// helperValidateBuildAndTest runs go vet and go test in the work directory.
func helperValidateBuildAndTest(workDir string) (bool, string) {
	// First check it compiles (go vet includes compilation check without requiring main).
	cmd := exec.CommandContext(context.Background(), "go", "vet", "./...")
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, "vet failed: " + string(out)
	}

	// Then run tests.
	cmd = exec.CommandContext(context.Background(), "go", "test", "-v", "./...")
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, "tests failed: " + string(out)
	}

	return true, "all tests passed"
}

// Task 1: Fix a nil pointer dereference
func taskFixNilPointer() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-fix-nil-pointer",
		Description: "Fix a nil pointer dereference in a function that processes user data",
		Prompt:      "Fix the nil pointer dereference in the ProcessUser function. The function should return an error if the user is nil instead of panicking.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "bug-fix", "nil-pointer"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "nilptr"); err != nil {
				return err
			}
			code := `package main

type User struct {
	Name  string
	Email string
}

func ProcessUser(u *User) string {
	// BUG: This will panic if u is nil
	return u.Name + " <" + u.Email + ">"
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestProcessUser(t *testing.T) {
	u := &User{Name: "Alice", Email: "alice@example.com"}
	got := ProcessUser(u)
	if got != "Alice <alice@example.com>" {
		t.Errorf("got %q, want %q", got, "Alice <alice@example.com>")
	}
}

func TestProcessUserNil(t *testing.T) {
	got := ProcessUser(nil)
	if got != "" {
		t.Errorf("expected empty string for nil user, got %q", got)
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 2: Add error handling to a function
func taskAddErrorHandling() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-add-error-handling",
		Description: "Add proper error handling to a file reading function",
		Prompt:      "Add error handling to the ReadConfig function. It should return an error if the file doesn't exist or can't be read, instead of returning an empty string.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "error-handling"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "errhandle"); err != nil {
				return err
			}
			code := `package main

import "os"

// ReadConfig reads a config file and returns its contents.
// BUG: No error handling - should return (string, error)
func ReadConfig(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")
	_ = os.WriteFile(path, []byte("key=value"), 0o644)

	content, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "key=value" {
		t.Errorf("got %q, want %q", content, "key=value")
	}
}

func TestReadConfigMissing(t *testing.T) {
	_, err := ReadConfig("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 3: Implement a simple interface
func taskImplementInterface() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-implement-interface",
		Description: "Implement a Shape interface with Area and Perimeter methods",
		Prompt:      "Implement the Circle struct so it satisfies the Shape interface. Circle should have a Radius field and correctly compute Area (pi*r^2) and Perimeter (2*pi*r).",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "interface", "implementation"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "shapes"); err != nil {
				return err
			}
			code := `package main

import "math"

// Shape defines an interface for geometric shapes.
type Shape interface {
	Area() float64
	Perimeter() float64
}

// Circle represents a circle with a given radius.
// TODO: Implement the Shape interface for Circle.
type Circle struct {
	Radius float64
}

// Ensure math is used
var _ = math.Pi
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"math"
	"testing"
)

func TestCircleArea(t *testing.T) {
	c := Circle{Radius: 5}
	expected := math.Pi * 25
	got := c.Area()
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("Area() = %f, want %f", got, expected)
	}
}

func TestCirclePerimeter(t *testing.T) {
	c := Circle{Radius: 5}
	expected := 2 * math.Pi * 5
	got := c.Perimeter()
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("Perimeter() = %f, want %f", got, expected)
	}
}

func TestCircleImplementsShape(t *testing.T) {
	var s Shape = Circle{Radius: 1}
	_ = s
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 4: Write a unit test for a function
func taskWriteUnitTest() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-write-unit-test",
		Description: "Write unit tests for a string utility function",
		Prompt:      "Write unit tests for the Reverse function in main_test.go. Test normal strings, empty string, single character, and unicode strings.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "testing"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "strutil"); err != nil {
				return err
			}
			code := `package main

// Reverse returns the reverse of a string.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			// The test file is intentionally incomplete - the LLM should fill it in.
			test := `package main

import "testing"

// TODO: Write tests for the Reverse function.
// Test cases should include:
// - Normal string: "hello" -> "olleh"
// - Empty string: "" -> ""
// - Single char: "a" -> "a"
// - Unicode: "Hello, 世界" -> "界世 ,olleH"

func TestReverse(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "olleh"},
		{"", ""},
		{"a", "a"},
		{"Hello, 世界", "界世 ,olleH"},
	}
	for _, tt := range tests {
		got := Reverse(tt.input)
		if got != tt.want {
			t.Errorf("Reverse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 5: Refactor duplicate code into a helper
func taskRefactorDuplicate() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-refactor-duplicate",
		Description: "Refactor duplicate validation code into a reusable helper function",
		Prompt:      "Refactor the duplicate email validation logic in ValidateUser and ValidateAdmin into a shared helper function called validateEmail. The function should check that the email is non-empty and contains an '@' symbol.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "refactoring"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "refactor"); err != nil {
				return err
			}
			code := `package main

import "strings"

func validateEmail(email string) bool {
	return email != "" && strings.Contains(email, "@")
}

func ValidateUser(name, email string) bool {
	if name == "" {
		return false
	}
	return validateEmail(email)
}

func ValidateAdmin(name, email, role string) bool {
	if name == "" {
		return false
	}
	if role != "admin" {
		return false
	}
	return validateEmail(email)
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestValidateUser(t *testing.T) {
	tests := []struct {
		name, email string
		want        bool
	}{
		{"Alice", "alice@example.com", true},
		{"", "alice@example.com", false},
		{"Alice", "", false},
		{"Alice", "invalid", false},
	}
	for _, tt := range tests {
		got := ValidateUser(tt.name, tt.email)
		if got != tt.want {
			t.Errorf("ValidateUser(%q, %q) = %v, want %v", tt.name, tt.email, got, tt.want)
		}
	}
}

func TestValidateAdmin(t *testing.T) {
	tests := []struct {
		name, email, role string
		want              bool
	}{
		{"Bob", "bob@example.com", "admin", true},
		{"Bob", "bob@example.com", "user", false},
		{"", "bob@example.com", "admin", false},
		{"Bob", "", "admin", false},
		{"Bob", "invalid", "admin", false},
	}
	for _, tt := range tests {
		got := ValidateAdmin(tt.name, tt.email, tt.role)
		if got != tt.want {
			t.Errorf("ValidateAdmin(%q, %q, %q) = %v, want %v", tt.name, tt.email, tt.role, got, tt.want)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{"user@example.com", true},
		{"", false},
		{"nope", false},
	}
	for _, tt := range tests {
		got := validateEmail(tt.email)
		if got != tt.want {
			t.Errorf("validateEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 6: Fix a race condition (add mutex)
func taskFixRaceCondition() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-fix-race-condition",
		Description: "Fix a race condition in a concurrent counter by adding a mutex",
		Prompt:      "Fix the race condition in the Counter struct. Add a sync.Mutex to protect concurrent access to the count field. The Increment and Value methods must be goroutine-safe.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "concurrency", "race-condition"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "racefix"); err != nil {
				return err
			}
			code := `package main

import "sync"

// Counter is a goroutine-safe counter.
type Counter struct {
	mu    sync.Mutex
	count int
}

func (c *Counter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"sync"
	"testing"
)

func TestCounterConcurrent(t *testing.T) {
	c := &Counter{}
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Increment()
		}()
	}

	wg.Wait()
	if c.Value() != 1000 {
		t.Errorf("expected 1000, got %d", c.Value())
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: func(workDir string) (bool, string) {
			// Vet first (includes compile check).
			cmd := exec.CommandContext(context.Background(), "go", "vet", "./...")
			cmd.Dir = workDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return false, "vet failed: " + string(out)
			}

			// Run with race detector.
			cmd = exec.CommandContext(context.Background(), "go", "test", "-race", "./...")
			cmd.Dir = workDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return false, "race test failed: " + string(out)
			}

			return true, "passed with race detector"
		},
	}
}

// Task 7: Implement a sorting algorithm
func taskImplementSort() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-implement-sort",
		Description: "Implement a merge sort algorithm for integer slices",
		Prompt:      "Implement the MergeSort function that sorts an integer slice using the merge sort algorithm. Do not use sort.Slice or any standard library sorting functions.",
		TimeLimit:   3 * time.Minute,
		Tags:        []string{"go", "algorithm", "sorting"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "sorting"); err != nil {
				return err
			}
			code := `package main

// MergeSort sorts an integer slice using merge sort algorithm.
func MergeSort(arr []int) []int {
	if len(arr) <= 1 {
		return arr
	}
	mid := len(arr) / 2
	left := MergeSort(arr[:mid])
	right := MergeSort(arr[mid:])
	return merge(left, right)
}

func merge(left, right []int) []int {
	result := make([]int, 0, len(left)+len(right))
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		if left[i] <= right[j] {
			result = append(result, left[i])
			i++
		} else {
			result = append(result, right[j])
			j++
		}
	}
	result = append(result, left[i:]...)
	result = append(result, right[j:]...)
	return result
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestMergeSort(t *testing.T) {
	tests := []struct {
		input []int
		want  []int
	}{
		{[]int{5, 3, 1, 4, 2}, []int{1, 2, 3, 4, 5}},
		{[]int{1}, []int{1}},
		{[]int{}, []int{}},
		{[]int{3, 3, 1, 1, 2, 2}, []int{1, 1, 2, 2, 3, 3}},
		{[]int{-1, 5, -3, 0, 2}, []int{-3, -1, 0, 2, 5}},
	}
	for _, tt := range tests {
		got := MergeSort(tt.input)
		if len(got) == 0 && len(tt.want) == 0 {
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("MergeSort(%v) length = %d, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("MergeSort(%v) = %v, want %v", tt.input, got, tt.want)
				break
			}
		}
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 8: Parse JSON into a struct
func taskParseJSON() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-parse-json",
		Description: "Parse JSON data into Go structs with proper field tags",
		Prompt:      "Implement the ParsePerson function that takes a JSON byte slice and returns a Person struct. Add the correct JSON struct tags to the Person struct fields.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "json", "parsing"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "jsonparse"); err != nil {
				return err
			}
			code := `package main

import "encoding/json"

// Person represents a person from JSON data.
type Person struct {
	FirstName string ` + "`json:\"first_name\"`" + `
	LastName  string ` + "`json:\"last_name\"`" + `
	Age       int    ` + "`json:\"age\"`" + `
	Email     string ` + "`json:\"email\"`" + `
}

// ParsePerson parses JSON bytes into a Person struct.
func ParsePerson(data []byte) (*Person, error) {
	var p Person
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestParsePerson(t *testing.T) {
	input := []byte(` + "`" + `{"first_name":"John","last_name":"Doe","age":30,"email":"john@example.com"}` + "`" + `)
	p, err := ParsePerson(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.FirstName != "John" {
		t.Errorf("FirstName = %q, want %q", p.FirstName, "John")
	}
	if p.LastName != "Doe" {
		t.Errorf("LastName = %q, want %q", p.LastName, "Doe")
	}
	if p.Age != 30 {
		t.Errorf("Age = %d, want %d", p.Age, 30)
	}
	if p.Email != "john@example.com" {
		t.Errorf("Email = %q, want %q", p.Email, "john@example.com")
	}
}

func TestParsePersonInvalid(t *testing.T) {
	_, err := ParsePerson([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 9: Add context cancellation
func taskAddContextCancellation() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-add-context-cancellation",
		Description: "Add context cancellation support to a long-running operation",
		Prompt:      "Add context.Context support to the Process function so it can be cancelled. The function should check for context cancellation between iterations and return ctx.Err() if cancelled.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "context", "cancellation"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "ctxcancel"); err != nil {
				return err
			}
			code := `package main

import "context"

// Process performs work in a loop, respecting context cancellation.
func Process(ctx context.Context, items []string) ([]string, error) {
	results := make([]string, 0, len(items))
	for _, item := range items {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		results = append(results, "processed: "+item)
	}
	return results, nil
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"context"
	"testing"
)

func TestProcessSuccess(t *testing.T) {
	ctx := context.Background()
	items := []string{"a", "b", "c"}
	results, err := Process(ctx, items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0] != "processed: a" {
		t.Errorf("results[0] = %q, want %q", results[0], "processed: a")
	}
}

func TestProcessCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	items := []string{"a", "b", "c"}
	_, err := Process(ctx, items)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 10: Fix an off-by-one error
func taskFixOffByOne() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-fix-off-by-one",
		Description: "Fix an off-by-one error in a pagination function",
		Prompt:      "Fix the off-by-one error in the Paginate function. It should return the correct slice of items for the given page number (1-based) and page size.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "bug-fix", "off-by-one"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "paginate"); err != nil {
				return err
			}
			code := `package main

// Paginate returns a page of items. Page is 1-based.
func Paginate(items []int, page, pageSize int) []int {
	if page < 1 || pageSize < 1 {
		return nil
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestPaginate(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		page, size int
		want       []int
	}{
		{1, 3, []int{1, 2, 3}},
		{2, 3, []int{4, 5, 6}},
		{3, 3, []int{7, 8, 9}},
		{4, 3, []int{10}},
		{5, 3, nil},
		{1, 10, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{0, 3, nil},
		{1, 0, nil},
	}

	for _, tt := range tests {
		got := Paginate(items, tt.page, tt.size)
		if !intSliceEqual(got, tt.want) {
			t.Errorf("Paginate(items, %d, %d) = %v, want %v", tt.page, tt.size, got, tt.want)
		}
	}
}

func intSliceEqual(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 11: Implement retry with backoff
func taskImplementRetryBackoff() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-implement-retry-backoff",
		Description: "Implement a retry function with exponential backoff",
		Prompt:      "Implement the Retry function that retries a fallible operation with exponential backoff. It should retry up to maxRetries times, doubling the wait between each attempt starting from initialDelay.",
		TimeLimit:   3 * time.Minute,
		Tags:        []string{"go", "retry", "backoff"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "retrybackoff"); err != nil {
				return err
			}
			code := `package main

import "time"

// Retry retries fn up to maxRetries times with exponential backoff.
// initialDelay is doubled after each failed attempt.
func Retry(fn func() error, maxRetries int, initialDelay time.Duration) error {
	var err error
	delay := initialDelay
	for i := 0; i <= maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxRetries {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return err
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"errors"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	calls := 0
	fn := func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	}

	err := Retry(fn, 5, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryExhausted(t *testing.T) {
	fn := func() error {
		return errors.New("always fails")
	}

	err := Retry(fn, 3, 1*time.Millisecond)
	if err == nil {
		t.Error("expected error after retries exhausted")
	}
}

func TestRetryImmediateSuccess(t *testing.T) {
	calls := 0
	fn := func() error {
		calls++
		return nil
	}

	err := Retry(fn, 3, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 12: Convert callback to channel pattern
func taskCallbackToChannel() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-callback-to-channel",
		Description: "Convert a callback-based API to use channels",
		Prompt:      "Implement the StreamResults function that converts the callback-based FetchWithCallback into a channel-based API. It should return a channel that receives results as they arrive.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "channels", "concurrency"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "chanpattern"); err != nil {
				return err
			}
			code := `package main

// FetchWithCallback calls the callback for each result.
func FetchWithCallback(items []string, cb func(string)) {
	for _, item := range items {
		cb("result: " + item)
	}
}

// StreamResults converts callback-based FetchWithCallback into channel-based API.
func StreamResults(items []string) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		FetchWithCallback(items, func(result string) {
			ch <- result
		})
	}()
	return ch
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestStreamResults(t *testing.T) {
	items := []string{"a", "b", "c"}
	ch := StreamResults(items)

	var results []string
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"result: a", "result: b", "result: c"}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("results[%d] = %q, want %q", i, r, expected[i])
		}
	}
}

func TestStreamResultsEmpty(t *testing.T) {
	ch := StreamResults(nil)
	var results []string
	for r := range ch {
		results = append(results, r)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 13: Add input validation
func taskAddInputValidation() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-add-input-validation",
		Description: "Add input validation to a user registration function",
		Prompt:      "Add input validation to the Register function. Validate that: name is non-empty (max 100 chars), email contains '@' and '.', age is between 0 and 150, password is at least 8 chars.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "validation"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "validation"); err != nil {
				return err
			}
			code := `package main

import (
	"errors"
	"strings"
)

// Register validates and registers a user.
func Register(name, email string, age int, password string) error {
	if name == "" || len(name) > 100 {
		return errors.New("invalid name: must be 1-100 characters")
	}
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		return errors.New("invalid email: must contain @ and .")
	}
	if age < 0 || age > 150 {
		return errors.New("invalid age: must be 0-150")
	}
	if len(password) < 8 {
		return errors.New("invalid password: must be at least 8 characters")
	}
	return nil
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			_ = `package main

import "testing"

func TestRegisterValid(t *testing.T) {
	err := Register("Alice", "alice@example.com", 25, "securepass")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterInvalidName(t *testing.T) {
	if err := Register("", "a@b.c", 25, "securepass"); err == nil {
		t.Error("expected error for empty name")
	}
	longName := strings.Repeat("a", 101)
	if err := Register(longName, "a@b.c", 25, "securepass"); err == nil {
		t.Error("expected error for name > 100 chars")
	}
}

func TestRegisterInvalidEmail(t *testing.T) {
	if err := Register("Alice", "invalid", 25, "securepass"); err == nil {
		t.Error("expected error for email without @")
	}
	if err := Register("Alice", "no@dot", 25, "securepass"); err == nil {
		t.Error("expected error for email without .")
	}
}

func TestRegisterInvalidAge(t *testing.T) {
	if err := Register("Alice", "a@b.c", -1, "securepass"); err == nil {
		t.Error("expected error for negative age")
	}
	if err := Register("Alice", "a@b.c", 151, "securepass"); err == nil {
		t.Error("expected error for age > 150")
	}
}

func TestRegisterInvalidPassword(t *testing.T) {
	if err := Register("Alice", "a@b.c", 25, "short"); err == nil {
		t.Error("expected error for password < 8 chars")
	}
}
`
			// Need to add strings import to test
			test := `package main

import (
	"strings"
	"testing"
)

func TestRegisterValid(t *testing.T) {
	err := Register("Alice", "alice@example.com", 25, "securepass")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterInvalidName(t *testing.T) {
	if err := Register("", "a@b.c", 25, "securepass"); err == nil {
		t.Error("expected error for empty name")
	}
	longName := strings.Repeat("a", 101)
	if err := Register(longName, "a@b.c", 25, "securepass"); err == nil {
		t.Error("expected error for name > 100 chars")
	}
}

func TestRegisterInvalidEmail(t *testing.T) {
	if err := Register("Alice", "invalid", 25, "securepass"); err == nil {
		t.Error("expected error for email without @")
	}
	if err := Register("Alice", "no@dot", 25, "securepass"); err == nil {
		t.Error("expected error for email without .")
	}
}

func TestRegisterInvalidAge(t *testing.T) {
	if err := Register("Alice", "a@b.c", -1, "securepass"); err == nil {
		t.Error("expected error for negative age")
	}
	if err := Register("Alice", "a@b.c", 151, "securepass"); err == nil {
		t.Error("expected error for age > 150")
	}
}

func TestRegisterInvalidPassword(t *testing.T) {
	if err := Register("Alice", "a@b.c", 25, "short"); err == nil {
		t.Error("expected error for password < 8 chars")
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 14: Fix a goroutine leak
func taskFixGoroutineLeak() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-fix-goroutine-leak",
		Description: "Fix a goroutine leak in a producer function",
		Prompt:      "Fix the goroutine leak in the Produce function. The goroutine should stop when the done channel is closed, and the returned channel should be properly closed when the goroutine exits.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "concurrency", "goroutine-leak"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "leakfix"); err != nil {
				return err
			}
			code := `package main

// Produce generates sequential numbers until done is closed.
func Produce(done <-chan struct{}) <-chan int {
	ch := make(chan int)
	go func() {
		defer close(ch)
		i := 0
		for {
			select {
			case <-done:
				return
			case ch <- i:
				i++
			}
		}
	}()
	return ch
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import "testing"

func TestProduceAndStop(t *testing.T) {
	done := make(chan struct{})
	ch := Produce(done)

	// Read a few values.
	for i := 0; i < 5; i++ {
		val := <-ch
		if val != i {
			t.Errorf("expected %d, got %d", i, val)
		}
	}

	// Signal done.
	close(done)

	// Channel should eventually be closed.
	// Drain any remaining buffered values.
	drained := false
	for range ch {
		drained = true
		_ = drained
	}
	// If we get here, the channel was closed properly.
}

func TestProduceImmediateStop(t *testing.T) {
	done := make(chan struct{})
	close(done)
	ch := Produce(done)

	// Channel should be closed without producing values.
	count := 0
	for range ch {
		count++
	}
	// Might get 0 or a very small number.
	if count > 1 {
		t.Errorf("expected at most 1 value after immediate close, got %d", count)
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}

// Task 15: Implement a simple HTTP handler
func taskImplementHTTPHandler() BenchmarkTask {
	return BenchmarkTask{
		ID:          "go-implement-http-handler",
		Description: "Implement a simple HTTP handler that returns JSON responses",
		Prompt:      "Implement the HealthHandler function that returns a JSON response with status 200 and body {\"status\":\"ok\",\"service\":\"hawk\"}. Also implement NotFoundHandler that returns 404 with {\"error\":\"not found\"}.",
		TimeLimit:   2 * time.Minute,
		Tags:        []string{"go", "http", "handler"},
		SetupFn: func(workDir string) error {
			if err := helperInitModule(workDir, "httphandler"); err != nil {
				return err
			}
			code := `package main

import (
	"encoding/json"
	"net/http"
)

// HealthHandler returns a JSON health check response.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "hawk",
	})
}

// NotFoundHandler returns a JSON 404 response.
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "not found",
	})
}
`
			if err := helperWriteFile(workDir, "main.go", code); err != nil {
				return err
			}

			test := `package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
	if body["service"] != "hawk" {
		t.Errorf("service = %q, want %q", body["service"], "hawk")
	}
}

func TestNotFoundHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()

	NotFoundHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if body["error"] != "not found" {
		t.Errorf("error = %q, want %q", body["error"], "not found")
	}
}
`
			return helperWriteFile(workDir, "main_test.go", test)
		},
		ValidateFn: helperValidateBuildAndTest,
	}
}
