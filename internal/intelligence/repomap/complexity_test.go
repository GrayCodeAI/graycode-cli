package repomap

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func TestAnalyzeGoAST_SimpleFunction(t *testing.T) {
	src := `package main

func hello() {
	fmt.Println("hello")
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	fn := funcs[0]
	if fn.Name != "hello" {
		t.Errorf("expected name 'hello', got %q", fn.Name)
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("expected cyclomatic=1 for simple function, got %d", fn.Cyclomatic)
	}
	if fn.Cognitive != 0 {
		t.Errorf("expected cognitive=0 for simple function, got %d", fn.Cognitive)
	}
	if fn.Nesting != 0 {
		t.Errorf("expected nesting=0, got %d", fn.Nesting)
	}
}

func TestAnalyzeGoAST_CyclomaticComplexity(t *testing.T) {
	src := `package main

func complex(x int, y string) (bool, error) {
	if x > 0 {
		if y == "a" {
			return true, nil
		} else if y == "b" {
			return false, nil
		}
	}
	for i := 0; i < x; i++ {
		switch i {
		case 1:
			break
		case 2:
			break
		default:
			continue
		}
	}
	if x > 10 || x < -10 {
		return false, fmt.Errorf("out of range")
	}
	return true, nil
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	fn := funcs[0]
	if fn.Name != "complex" {
		t.Errorf("expected name 'complex', got %q", fn.Name)
	}

	// Decision points: if, if, else if (if), for, switch, case 1, case 2, if, ||
	// Base = 1 + decisions
	if fn.Cyclomatic < 8 {
		t.Errorf("expected cyclomatic >= 8 for complex function, got %d", fn.Cyclomatic)
	}

	if fn.Parameters != 2 {
		t.Errorf("expected 2 parameters, got %d", fn.Parameters)
	}

	if fn.Returns != 2 {
		t.Errorf("expected 2 returns, got %d", fn.Returns)
	}
}

func TestAnalyzeGoAST_NestedIfFor_CognitiveComplexity(t *testing.T) {
	src := `package main

func nested(items []int) int {
	total := 0
	for _, item := range items {
		if item > 0 {
			for j := 0; j < item; j++ {
				if j%2 == 0 {
					total += j
				}
			}
		}
	}
	return total
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	fn := funcs[0]

	// Cognitive should be > cyclomatic due to nesting penalties
	if fn.Cognitive <= fn.Cyclomatic-1 {
		t.Errorf("expected cognitive > cyclomatic-1 for nested code, got cognitive=%d, cyclomatic=%d",
			fn.Cognitive, fn.Cyclomatic)
	}

	// Max nesting should be at least 3 (range > if > for > if)
	if fn.Nesting < 3 {
		t.Errorf("expected nesting >= 3, got %d", fn.Nesting)
	}
}

func TestAnalyzeGoAST_MethodReceiver(t *testing.T) {
	src := `package main

type Server struct{}

func (s *Server) Handle(r Request) error {
	return nil
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	if funcs[0].Name != "Server.Handle" {
		t.Errorf("expected 'Server.Handle', got %q", funcs[0].Name)
	}
}

func TestFindHotspots_Sorted(t *testing.T) {
	// Create a temp directory with test files
	dir := t.TempDir()

	simpleFile := `package main

func simple() {
	fmt.Println("hello")
}
`
	complexFile := `package main

func veryComplex(a, b, c int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				for i := 0; i < a; i++ {
					if i%2 == 0 {
						switch b {
						case 1:
							return 1
						case 2:
							return 2
						case 3:
							return 3
						}
					}
				}
			}
		}
	} else if a < -10 {
		return -1
	}
	return 0
}

func medium(x int) int {
	if x > 0 {
		return x * 2
	}
	return x
}
`

	if err := os.WriteFile(filepath.Join(dir, "simple.go"), []byte(simpleFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "complex.go"), []byte(complexFile), 0o644); err != nil {
		t.Fatal(err)
	}

	ca := NewComplexityAnalyzer()
	hotspots := ca.FindHotspots(dir, 3)

	if len(hotspots) < 2 {
		t.Fatalf("expected at least 2 hotspots, got %d", len(hotspots))
	}

	// Should be sorted descending by complexity
	for i := 1; i < len(hotspots); i++ {
		if hotspots[i].Cyclomatic > hotspots[i-1].Cyclomatic {
			t.Errorf("hotspots not sorted: index %d (CC=%d) > index %d (CC=%d)",
				i, hotspots[i].Cyclomatic, i-1, hotspots[i-1].Cyclomatic)
		}
	}

	// Most complex should be veryComplex
	if hotspots[0].Name != "veryComplex" {
		t.Errorf("expected first hotspot to be 'veryComplex', got %q", hotspots[0].Name)
	}
}

func TestSuggestRefactoring(t *testing.T) {
	ca := NewComplexityAnalyzer()

	tests := []struct {
		name     string
		fc       FunctionComplexity
		contains []string
	}{
		{
			name: "high cyclomatic",
			fc: FunctionComplexity{
				Cyclomatic: 15,
				Cognitive:  5,
				Nesting:    2,
				Parameters: 2,
				LOC:        30,
			},
			contains: []string{"Extract method"},
		},
		{
			name: "high nesting",
			fc: FunctionComplexity{
				Cyclomatic: 5,
				Cognitive:  5,
				Nesting:    6,
				Parameters: 2,
				LOC:        30,
			},
			contains: []string{"early returns"},
		},
		{
			name: "high params",
			fc: FunctionComplexity{
				Cyclomatic: 3,
				Cognitive:  3,
				Nesting:    1,
				Parameters: 7,
				LOC:        20,
			},
			contains: []string{"struct"},
		},
		{
			name: "high LOC",
			fc: FunctionComplexity{
				Cyclomatic: 3,
				Cognitive:  3,
				Nesting:    1,
				Parameters: 2,
				LOC:        80,
			},
			contains: []string{"smaller focused functions"},
		},
		{
			name: "multiple issues",
			fc: FunctionComplexity{
				Cyclomatic: 20,
				Cognitive:  25,
				Nesting:    7,
				Parameters: 8,
				LOC:        100,
			},
			contains: []string{"Extract method", "early returns", "struct", "smaller focused"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			suggestions := ca.SuggestRefactoring(tc.fc)
			for _, expected := range tc.contains {
				found := false
				for _, s := range suggestions {
					if strings.Contains(s, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected suggestion containing %q, got: %v", expected, suggestions)
				}
			}
		})
	}
}

func TestSuggestRefactoring_NoSuggestions(t *testing.T) {
	ca := NewComplexityAnalyzer()
	fc := FunctionComplexity{
		Cyclomatic: 3,
		Cognitive:  4,
		Nesting:    2,
		Parameters: 2,
		LOC:        20,
	}
	suggestions := ca.SuggestRefactoring(fc)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for simple function, got: %v", suggestions)
	}
}

func TestFormatReport(t *testing.T) {
	ca := NewComplexityAnalyzer()
	report := &ComplexityReport{
		File: "src/handler.go",
		Functions: []FunctionComplexity{
			{Name: "handleRequest", Cyclomatic: 12, Cognitive: 18, LOC: 65, Nesting: 5},
			{Name: "validateInput", Cyclomatic: 4, Cognitive: 6, LOC: 20, Nesting: 2},
			{Name: "parseHeaders", Cyclomatic: 3, Cognitive: 4, LOC: 15, Nesting: 1},
		},
		FileComplexity: 6.3,
		LOC:            180,
		CLOC:           20,
		BlankLines:     15,
	}

	output := ca.FormatReport(report)

	// Check that output contains expected elements
	if !strings.Contains(output, "src/handler.go") {
		t.Error("expected file path in output")
	}
	if !strings.Contains(output, "handleRequest") {
		t.Error("expected function name in output")
	}
	if !strings.Contains(output, "HIGH") {
		t.Error("expected HIGH warning for handleRequest")
	}
	if !strings.Contains(output, icons.CheckBold()) {
		t.Error("expected checkmark for simple functions")
	}
	if !strings.Contains(output, "Functions=3") {
		t.Error("expected function count in summary")
	}
	if !strings.Contains(output, "Suggestions") {
		t.Error("expected suggestions section for complex functions")
	}
}

func TestFormatReport_Critical(t *testing.T) {
	ca := NewComplexityAnalyzer()
	report := &ComplexityReport{
		File: "src/monster.go",
		Functions: []FunctionComplexity{
			{Name: "processAll", Cyclomatic: 25, Cognitive: 35, LOC: 120, Nesting: 8},
		},
		FileComplexity: 25,
		LOC:            120,
	}

	output := ca.FormatReport(report)
	if !strings.Contains(output, "CRITICAL") {
		t.Error("expected CRITICAL marker for very high complexity")
	}
}

func TestGenericAnalysis_Python(t *testing.T) {
	pythonCode := `
def simple_func():
    return 42

def complex_func(x, y, z):
    if x > 0:
        for i in range(y):
            if i % 2 == 0:
                while z > 0:
                    z -= 1
    elif x < 0:
        try:
            something()
        except Exception:
            pass
    return x
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGeneric(pythonCode, "python")

	if len(funcs) < 2 {
		t.Fatalf("expected at least 2 functions, got %d", len(funcs))
	}

	// simple_func should have CC=1
	var simpleFunc, complexFunc *FunctionComplexity
	for i := range funcs {
		if funcs[i].Name == "simple_func" {
			simpleFunc = &funcs[i]
		}
		if funcs[i].Name == "complex_func" {
			complexFunc = &funcs[i]
		}
	}

	if simpleFunc == nil {
		t.Fatal("simple_func not found")
	}
	if simpleFunc.Cyclomatic != 1 {
		t.Errorf("expected simple_func CC=1, got %d", simpleFunc.Cyclomatic)
	}

	if complexFunc == nil {
		t.Fatal("complex_func not found")
	}
	if complexFunc.Cyclomatic <= 1 {
		t.Errorf("expected complex_func CC > 1, got %d", complexFunc.Cyclomatic)
	}
	if complexFunc.Parameters != 3 {
		t.Errorf("expected 3 parameters, got %d", complexFunc.Parameters)
	}
}

func TestGenericAnalysis_TypeScript(t *testing.T) {
	tsCode := `
function handleRequest(req, res) {
    if (req.method === 'GET') {
        if (req.path === '/users') {
            return getUsers(res);
        } else if (req.path === '/posts') {
            return getPosts(res);
        }
    } else if (req.method === 'POST') {
        for (const key of req.body) {
            if (key === 'name') {
                validate(key);
            }
        }
    }
    return res.status(404);
}

function simple(x) {
    return x * 2;
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGeneric(tsCode, "typescript")

	if len(funcs) < 1 {
		t.Fatalf("expected at least 1 function, got %d", len(funcs))
	}

	// handleRequest should have higher complexity
	var handler *FunctionComplexity
	for i := range funcs {
		if funcs[i].Name == "handleRequest" {
			handler = &funcs[i]
		}
	}

	if handler == nil {
		t.Fatal("handleRequest not found")
	}
	if handler.Cyclomatic < 5 {
		t.Errorf("expected handleRequest CC >= 5, got %d", handler.Cyclomatic)
	}
}

func TestMaintainabilityIndex(t *testing.T) {
	tests := []struct {
		name  string
		fc    FunctionComplexity
		minMI float64
		maxMI float64
	}{
		{
			name:  "simple function",
			fc:    FunctionComplexity{Cyclomatic: 1, LOC: 5},
			minMI: 80,
			maxMI: 100,
		},
		{
			name:  "moderate function",
			fc:    FunctionComplexity{Cyclomatic: 8, LOC: 40},
			minMI: 60,
			maxMI: 100,
		},
		{
			name:  "complex function",
			fc:    FunctionComplexity{Cyclomatic: 20, LOC: 100},
			minMI: 40,
			maxMI: 70,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mi := MaintainabilityIndex(tc.fc)
			if mi < tc.minMI || mi > tc.maxMI {
				t.Errorf("MI=%.2f, expected range [%.1f, %.1f]", mi, tc.minMI, tc.maxMI)
			}
		})
	}
}

func TestMaintainabilityIndex_Decreases(t *testing.T) {
	simple := FunctionComplexity{Cyclomatic: 1, LOC: 10}
	complex := FunctionComplexity{Cyclomatic: 20, LOC: 150}

	miSimple := MaintainabilityIndex(simple)
	miComplex := MaintainabilityIndex(complex)

	if miSimple <= miComplex {
		t.Errorf("expected simple MI (%.2f) > complex MI (%.2f)", miSimple, miComplex)
	}
}

func TestMaintainabilityIndex_ZeroLOC(t *testing.T) {
	fc := FunctionComplexity{Cyclomatic: 1, LOC: 0}
	mi := MaintainabilityIndex(fc)
	if math.IsNaN(mi) || math.IsInf(mi, 0) {
		t.Errorf("MI should not be NaN or Inf for zero LOC, got %f", mi)
	}
}

func TestAnalyzeFile_Go(t *testing.T) {
	src := `package main

// This is a comment
// Another comment

func add(a, b int) int {
	return a + b
}

func divide(a, b int) (int, error) {
	if b == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	return a / b, nil
}
`
	ca := NewComplexityAnalyzer()
	report, err := ca.AnalyzeFile("main.go", src)
	if err != nil {
		t.Fatal(err)
	}

	if report.File != "main.go" {
		t.Errorf("expected file 'main.go', got %q", report.File)
	}
	if len(report.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(report.Functions))
	}
	if report.CLOC < 2 {
		t.Errorf("expected at least 2 comment lines, got %d", report.CLOC)
	}
	if report.BlankLines < 1 {
		t.Errorf("expected at least 1 blank line, got %d", report.BlankLines)
	}

	// add should have CC=1
	if report.Functions[0].Cyclomatic != 1 {
		t.Errorf("expected add CC=1, got %d", report.Functions[0].Cyclomatic)
	}
	// divide should have CC=2 (one if)
	if report.Functions[1].Cyclomatic != 2 {
		t.Errorf("expected divide CC=2, got %d", report.Functions[1].Cyclomatic)
	}
}

func TestAnalyzeFile_NonGo(t *testing.T) {
	pyCode := `# A python module
def greet(name):
    if name:
        print(f"Hello {name}")
    else:
        print("Hello world")
`
	ca := NewComplexityAnalyzer()
	report, err := ca.AnalyzeFile("greet.py", pyCode)
	if err != nil {
		t.Fatal(err)
	}

	if report.File != "greet.py" {
		t.Errorf("expected file 'greet.py', got %q", report.File)
	}
	if len(report.Functions) < 1 {
		t.Fatal("expected at least 1 function")
	}
	if report.CLOC < 1 {
		t.Errorf("expected at least 1 comment line, got %d", report.CLOC)
	}
}

func TestAnalyzeGoAST_ManyBranches(t *testing.T) {
	src := `package main

func manyBranches(x int) string {
	if x == 1 {
		return "one"
	} else if x == 2 {
		return "two"
	} else if x == 3 {
		return "three"
	} else if x == 4 {
		return "four"
	} else if x == 5 {
		return "five"
	} else if x == 6 {
		return "six"
	} else if x == 7 {
		return "seven"
	} else if x == 8 {
		return "eight"
	} else if x == 9 {
		return "nine"
	} else if x == 10 {
		return "ten"
	}
	return "other"
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	fn := funcs[0]
	// 10 if statements + base = at least 11
	if fn.Cyclomatic < 11 {
		t.Errorf("expected cyclomatic >= 11 for many branches, got %d", fn.Cyclomatic)
	}
}

func TestAnalyzeGoAST_LogicalOperators(t *testing.T) {
	src := `package main

func logical(a, b, c, d bool) bool {
	if a && b || c && d {
		return true
	}
	return false
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)

	if len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %d", len(funcs))
	}

	fn := funcs[0]
	// 1 (base) + 1 (if) + 3 (&&, ||, &&) = 5
	if fn.Cyclomatic < 4 {
		t.Errorf("expected cyclomatic >= 4 (if + logical operators), got %d", fn.Cyclomatic)
	}
}

func TestNewComplexityAnalyzer_Defaults(t *testing.T) {
	ca := NewComplexityAnalyzer()

	if ca.HighCyclomatic != 10 {
		t.Errorf("expected HighCyclomatic=10, got %d", ca.HighCyclomatic)
	}
	if ca.HighCognitive != 15 {
		t.Errorf("expected HighCognitive=15, got %d", ca.HighCognitive)
	}
	if ca.HighNesting != 4 {
		t.Errorf("expected HighNesting=4, got %d", ca.HighNesting)
	}
	if ca.HighLOC != 50 {
		t.Errorf("expected HighLOC=50, got %d", ca.HighLOC)
	}
}

func TestFindHotspots_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ca := NewComplexityAnalyzer()
	hotspots := ca.FindHotspots(dir, 10)
	if len(hotspots) != 0 {
		t.Errorf("expected 0 hotspots for empty dir, got %d", len(hotspots))
	}
}

func TestFindHotspots_Limit(t *testing.T) {
	dir := t.TempDir()
	code := `package main

func f1() { if true {} }
func f2() { if true { if true {} } }
func f3() { if true { if true { if true {} } } }
func f4() { for i := 0; i < 10; i++ { if true {} } }
func f5() { switch x { case 1: case 2: case 3: } }
`
	if err := os.WriteFile(filepath.Join(dir, "funcs.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	ca := NewComplexityAnalyzer()
	hotspots := ca.FindHotspots(dir, 3)

	if len(hotspots) != 3 {
		t.Errorf("expected 3 hotspots with limit=3, got %d", len(hotspots))
	}
}

func TestFormatReport_NilReport(t *testing.T) {
	ca := NewComplexityAnalyzer()
	output := ca.FormatReport(nil)
	if output != "" {
		t.Errorf("expected empty output for nil report, got %q", output)
	}
}

func TestAnalyzeGoAST_EmptyBody(t *testing.T) {
	src := `package main

type MyInterface interface {
	DoSomething(x int) error
}
`
	ca := NewComplexityAnalyzer()
	funcs := ca.AnalyzeGoAST(src)
	// Interface methods don't have bodies, no functions expected
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions for interface, got %d", len(funcs))
	}
}
