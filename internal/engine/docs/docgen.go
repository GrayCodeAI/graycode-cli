// Package docs provides documentation generation utilities.
//
// Deprecated APIs: parser.ParseDir and ast.Package are deprecated since Go 1.25/1.22.
// A future refactor should migrate to golang.org/x/tools/go/packages, but the current
// implementation is functional and the migration is non-trivial.
//
//nolint:staticcheck
package docs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type DocGenerator struct {
	ProjectDir     string
	OutputFormat   string
	IncludePrivate bool
	MaxDepth       int
}

type DocSection struct {
	Title    string
	Content  string
	Children []DocSection
	Level    int
}

type ProjectDoc struct {
	Name         string
	Description  string
	Packages     []PackageDoc
	Architecture string
	QuickStart   string
	GeneratedAt  time.Time
}

type PackageDoc struct {
	Name        string
	Path        string
	Description string
	Functions   []FunctionDoc
	Types       []TypeDoc
	FileCount   int
}

type FunctionDoc struct {
	Name        string
	Signature   string
	Description string
	Parameters  []ParamDoc
	Returns     string
	Example     string
	Exported    bool
}

type ParamDoc struct {
	Name string
	Type string
	Desc string
}

type TypeDoc struct {
	Name        string
	Kind        string
	Fields      []FieldDoc
	Methods     []FunctionDoc
	Description string
}

type FieldDoc struct {
	Name string
	Type string
	Tag  string
	Desc string
}

func NewDocGenerator(projectDir string) *DocGenerator {
	return &DocGenerator{
		ProjectDir:     projectDir,
		OutputFormat:   "markdown",
		IncludePrivate: false,
		MaxDepth:       3,
	}
}

func (dg *DocGenerator) Generate() (*ProjectDoc, error) {
	info, err := os.Stat(dg.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("cannot access project directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", dg.ProjectDir)
	}

	doc := &ProjectDoc{
		Name:        filepath.Base(dg.ProjectDir),
		Description: dg.InferDescription(dg.ProjectDir),
		GeneratedAt: time.Now(),
	}

	packages, err := dg.findPackages(dg.ProjectDir, 0)
	if err != nil {
		return nil, fmt.Errorf("error scanning packages: %w", err)
	}
	doc.Packages = packages

	doc.Architecture = dg.inferArchitecture(packages)

	doc.QuickStart = dg.generateQuickStart(doc)

	return doc, nil
}

func (dg *DocGenerator) findPackages(dir string, depth int) ([]PackageDoc, error) {
	if depth > dg.MaxDepth {
		return nil, nil
	}

	var packages []PackageDoc

	goFiles, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	if len(goFiles) > 0 {
		pkg, err := dg.parseGoPackage(dir)
		if err == nil && pkg != nil {
			packages = append(packages, *pkg)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return packages, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
			continue
		}
		subPkgs, err := dg.findPackages(filepath.Join(dir, name), depth+1)
		if err != nil {
			continue
		}
		packages = append(packages, subPkgs...)
	}

	return packages, nil
}

func (dg *DocGenerator) parseGoPackage(dir string) (*PackageDoc, error) {
	fset := token.NewFileSet()
	//lint:ignore SA1019 parser.ParseDir is deprecated; migration to go/packages is non-trivial
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("error parsing package in %s: %w", dir, err)
	}

	if len(pkgs) == 0 {
		return nil, nil
	}

	//lint:ignore SA1019 ast.Package is deprecated; migration to go/types is non-trivial
	var astPkg *ast.Package
	for _, p := range pkgs {
		if !strings.HasSuffix(p.Name, "_test") {
			astPkg = p
			break
		}
	}
	if astPkg == nil {
		return nil, nil
	}

	relPath, _ := filepath.Rel(dg.ProjectDir, dir)
	if relPath == "." {
		relPath = ""
	}

	pkgDoc := &PackageDoc{
		Name:      astPkg.Name,
		Path:      relPath,
		FileCount: len(astPkg.Files),
	}

	for _, file := range astPkg.Files {
		if file.Doc != nil {
			pkgDoc.Description = strings.TrimSpace(file.Doc.Text())
			break
		}
	}

	for _, file := range astPkg.Files {
		dg.extractFunctions(file, pkgDoc)
		dg.extractTypes(file, pkgDoc)
	}

	return pkgDoc, nil
}

func (dg *DocGenerator) extractFunctions(file *ast.File, pkgDoc *PackageDoc) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if funcDecl.Recv != nil {
			continue
		}

		exported := funcDecl.Name.IsExported()
		if !dg.IncludePrivate && !exported {
			continue
		}

		funcDoc := FunctionDoc{
			Name:        funcDecl.Name.Name,
			Signature:   dg.buildFuncSignature(funcDecl),
			Description: extractDocComment(funcDecl.Doc),
			Exported:    exported,
		}

		if funcDecl.Type.Params != nil {
			for _, param := range funcDecl.Type.Params.List {
				typeStr := exprToString(param.Type)
				if len(param.Names) == 0 {
					funcDoc.Parameters = append(funcDoc.Parameters, ParamDoc{
						Type: typeStr,
					})
				} else {
					for _, name := range param.Names {
						funcDoc.Parameters = append(funcDoc.Parameters, ParamDoc{
							Name: name.Name,
							Type: typeStr,
						})
					}
				}
			}
		}

		if funcDecl.Type.Results != nil {
			var returns []string
			for _, result := range funcDecl.Type.Results.List {
				returns = append(returns, exprToString(result.Type))
			}
			funcDoc.Returns = strings.Join(returns, ", ")
		}

		pkgDoc.Functions = append(pkgDoc.Functions, funcDoc)
	}
}

func (dg *DocGenerator) extractTypes(file *ast.File, pkgDoc *PackageDoc) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			exported := typeSpec.Name.IsExported()
			if !dg.IncludePrivate && !exported {
				continue
			}

			typeDoc := TypeDoc{
				Name:        typeSpec.Name.Name,
				Description: extractDocComment(genDecl.Doc),
			}

			switch t := typeSpec.Type.(type) {
			case *ast.StructType:
				typeDoc.Kind = "struct"
				if t.Fields != nil {
					for _, field := range t.Fields.List {
						if len(field.Names) == 0 {
							fieldDoc := FieldDoc{
								Name: exprToString(field.Type),
								Type: exprToString(field.Type),
								Desc: extractDocComment(field.Doc),
							}
							if field.Tag != nil {
								fieldDoc.Tag = field.Tag.Value
							}
							typeDoc.Fields = append(typeDoc.Fields, fieldDoc)
						} else {
							for _, name := range field.Names {
								if !dg.IncludePrivate && !unicode.IsUpper(rune(name.Name[0])) {
									continue
								}
								fieldDoc := FieldDoc{
									Name: name.Name,
									Type: exprToString(field.Type),
									Desc: extractDocComment(field.Doc),
								}
								if field.Tag != nil {
									fieldDoc.Tag = field.Tag.Value
								}
								typeDoc.Fields = append(typeDoc.Fields, fieldDoc)
							}
						}
					}
				}
			case *ast.InterfaceType:
				typeDoc.Kind = "interface"
				if t.Methods != nil {
					for _, method := range t.Methods.List {
						if len(method.Names) > 0 {
							mDoc := FunctionDoc{
								Name:        method.Names[0].Name,
								Description: extractDocComment(method.Doc),
								Exported:    method.Names[0].IsExported(),
							}
							if ft, ok := method.Type.(*ast.FuncType); ok {
								mDoc.Signature = dg.buildMethodSignature(method.Names[0].Name, ft)
							}
							typeDoc.Methods = append(typeDoc.Methods, mDoc)
						}
					}
				}
			default:
				typeDoc.Kind = "type"
			}

			dg.attachMethods(file, &typeDoc)

			pkgDoc.Types = append(pkgDoc.Types, typeDoc)
		}
	}
}

func (dg *DocGenerator) attachMethods(file *ast.File, typeDoc *TypeDoc) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil {
			continue
		}

		recvType := receiverTypeName(funcDecl.Recv)
		if recvType != typeDoc.Name {
			continue
		}

		exported := funcDecl.Name.IsExported()
		if !dg.IncludePrivate && !exported {
			continue
		}

		methodDoc := FunctionDoc{
			Name:        funcDecl.Name.Name,
			Signature:   dg.buildFuncSignature(funcDecl),
			Description: extractDocComment(funcDecl.Doc),
			Exported:    exported,
		}

		if funcDecl.Type.Results != nil {
			var returns []string
			for _, result := range funcDecl.Type.Results.List {
				returns = append(returns, exprToString(result.Type))
			}
			methodDoc.Returns = strings.Join(returns, ", ")
		}

		typeDoc.Methods = append(typeDoc.Methods, methodDoc)
	}
}

func (dg *DocGenerator) buildFuncSignature(funcDecl *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")

	if funcDecl.Recv != nil {
		sb.WriteString("(")
		for i, field := range funcDecl.Recv.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			if len(field.Names) > 0 {
				sb.WriteString(field.Names[0].Name)
				sb.WriteString(" ")
			}
			sb.WriteString(exprToString(field.Type))
		}
		sb.WriteString(") ")
	}

	sb.WriteString(funcDecl.Name.Name)
	sb.WriteString("(")

	if funcDecl.Type.Params != nil {
		params := []string{}
		for _, field := range funcDecl.Type.Params.List {
			typeStr := exprToString(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeStr)
			} else {
				for _, name := range field.Names {
					params = append(params, name.Name+" "+typeStr)
				}
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}

	sb.WriteString(")")

	if funcDecl.Type.Results != nil {
		results := []string{}
		for _, field := range funcDecl.Type.Results.List {
			typeStr := exprToString(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					results = append(results, name.Name+" "+typeStr)
				}
			} else {
				results = append(results, typeStr)
			}
		}
		if len(results) == 1 {
			sb.WriteString(" ")
			sb.WriteString(results[0])
		} else if len(results) > 1 {
			sb.WriteString(" (")
			sb.WriteString(strings.Join(results, ", "))
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func (dg *DocGenerator) buildMethodSignature(name string, ft *ast.FuncType) string {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("(")

	if ft.Params != nil {
		params := []string{}
		for _, field := range ft.Params.List {
			typeStr := exprToString(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeStr)
			} else {
				for _, n := range field.Names {
					params = append(params, n.Name+" "+typeStr)
				}
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}

	sb.WriteString(")")

	if ft.Results != nil {
		results := []string{}
		for _, field := range ft.Results.List {
			results = append(results, exprToString(field.Type))
		}
		if len(results) == 1 {
			sb.WriteString(" ")
			sb.WriteString(results[0])
		} else if len(results) > 1 {
			sb.WriteString(" (")
			sb.WriteString(strings.Join(results, ", "))
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func RenderMarkdown(doc *ProjectDoc) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(doc.Name)
	sb.WriteString("\n\n")

	if doc.Description != "" {
		sb.WriteString(doc.Description)
		sb.WriteString("\n\n")
	}

	if doc.Architecture != "" {
		sb.WriteString("## Architecture\n\n")
		sb.WriteString(doc.Architecture)
		sb.WriteString("\n\n")
	}

	if doc.QuickStart != "" {
		sb.WriteString("## Quick Start\n\n")
		sb.WriteString(doc.QuickStart)
		sb.WriteString("\n\n")
	}

	if len(doc.Packages) > 0 {
		sb.WriteString("## Packages\n\n")

		for _, pkg := range doc.Packages {
			sb.WriteString("### package ")
			sb.WriteString(pkg.Name)
			sb.WriteString("\n\n")

			if pkg.Path != "" {
				sb.WriteString("**Path:** `")
				sb.WriteString(pkg.Path)
				sb.WriteString("`\n\n")
			}

			if pkg.Description != "" {
				sb.WriteString(pkg.Description)
				sb.WriteString("\n\n")
			}

			if len(pkg.Functions) > 0 {
				sb.WriteString("#### Functions\n\n")
				for _, fn := range pkg.Functions {
					sb.WriteString("##### `")
					sb.WriteString(fn.Signature)
					sb.WriteString("`\n\n")
					if fn.Description != "" {
						sb.WriteString(fn.Description)
						sb.WriteString("\n\n")
					}
					if fn.Example != "" {
						sb.WriteString("**Example:**\n\n```go\n")
						sb.WriteString(fn.Example)
						sb.WriteString("\n```\n\n")
					}
				}
			}

			if len(pkg.Types) > 0 {
				sb.WriteString("#### Types\n\n")
				for _, t := range pkg.Types {
					sb.WriteString("##### `type ")
					sb.WriteString(t.Name)
					sb.WriteString(" ")
					sb.WriteString(t.Kind)
					sb.WriteString("`\n\n")

					if t.Description != "" {
						sb.WriteString(t.Description)
						sb.WriteString("\n\n")
					}

					if len(t.Fields) > 0 {
						sb.WriteString("| Field | Type | Description |\n")
						sb.WriteString("|-------|------|-------------|\n")
						for _, f := range t.Fields {
							desc := f.Desc
							if desc == "" {
								desc = "-"
							}
							sb.WriteString("| ")
							sb.WriteString(f.Name)
							sb.WriteString(" | ")
							sb.WriteString(f.Type)
							sb.WriteString(" | ")
							sb.WriteString(desc)
							sb.WriteString(" |\n")
						}
						sb.WriteString("\n")
					}

					if len(t.Methods) > 0 {
						sb.WriteString("**Methods:**\n\n")
						for _, m := range t.Methods {
							sb.WriteString("- `")
							sb.WriteString(m.Signature)
							sb.WriteString("`")
							if m.Description != "" {
								sb.WriteString(" - ")
								sb.WriteString(m.Description)
							}
							sb.WriteString("\n")
						}
						sb.WriteString("\n")
					}
				}
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("*Generated at: ")
	sb.WriteString(doc.GeneratedAt.Format(time.RFC3339))
	sb.WriteString("*\n")

	return sb.String()
}

func RenderHTML(doc *ProjectDoc) string {
	const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Documentation</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; display: flex; }
        nav { width: 250px; background: #f5f5f5; padding: 20px; height: 100vh; overflow-y: auto; position: fixed; }
        nav ul { list-style: none; padding: 0; }
        nav li { margin: 5px 0; }
        nav a { text-decoration: none; color: #333; }
        nav a:hover { color: #0066cc; }
        main { margin-left: 290px; padding: 40px; max-width: 800px; }
        h1 { color: #333; border-bottom: 2px solid #0066cc; padding-bottom: 10px; }
        h2 { color: #444; margin-top: 40px; }
        h3 { color: #555; }
        code { background: #f0f0f0; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
        pre { background: #f8f8f8; padding: 15px; border-radius: 5px; overflow-x: auto; }
        table { border-collapse: collapse; width: 100%; margin: 10px 0; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background: #f0f0f0; }
        .description { color: #666; margin: 10px 0; }
        .signature { font-family: monospace; background: #f8f8f8; padding: 8px; border-left: 3px solid #0066cc; }
    </style>
</head>
<body>
    <nav>
        <h2>Navigation</h2>
        <ul>
            <li><a href="#overview">Overview</a></li>
{{- range .Packages}}
            <li><a href="#pkg-{{.Name}}">{{.Name}}</a></li>
{{- end}}
        </ul>
    </nav>
    <main>
        <h1 id="overview">{{.Name}}</h1>
        <p class="description">{{.Description}}</p>
{{- if .Architecture}}
        <h2>Architecture</h2>
        <p>{{.Architecture}}</p>
{{- end}}
{{- if .QuickStart}}
        <h2>Quick Start</h2>
        <pre><code>{{.QuickStart}}</code></pre>
{{- end}}
{{- range .Packages}}
        <h2 id="pkg-{{.Name}}">Package {{.Name}}</h2>
        <p class="description">{{.Description}}</p>
{{- range .Functions}}
        <h3>{{.Name}}</h3>
        <div class="signature">{{.Signature}}</div>
        <p class="description">{{.Description}}</p>
{{- end}}
{{- range .Types}}
        <h3>{{.Name}} ({{.Kind}})</h3>
        <p class="description">{{.Description}}</p>
{{- if .Fields}}
        <table>
            <tr><th>Field</th><th>Type</th><th>Description</th></tr>
{{- range .Fields}}
            <tr><td>{{.Name}}</td><td><code>{{.Type}}</code></td><td>{{.Desc}}</td></tr>
{{- end}}
        </table>
{{- end}}
{{- end}}
{{- end}}
    </main>
</body>
</html>`

	tmpl, err := template.New("doc").Parse(htmlTemplate)
	if err != nil {
		return "<html><body><p>Error generating documentation</p></body></html>"
	}

	var sb strings.Builder
	err = tmpl.Execute(&sb, doc)
	if err != nil {
		return "<html><body><p>Error generating documentation</p></body></html>"
	}

	return sb.String()
}

func GenerateREADME(doc *ProjectDoc) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(doc.Name)
	sb.WriteString("\n\n")

	if doc.Description != "" {
		sb.WriteString(doc.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\ngo get ")
	sb.WriteString(doc.Name)
	sb.WriteString("\n```\n\n")

	if doc.QuickStart != "" {
		sb.WriteString("## Quick Start\n\n")
		sb.WriteString("```go\n")
		sb.WriteString(doc.QuickStart)
		sb.WriteString("\n```\n\n")
	}

	if len(doc.Packages) > 0 {
		sb.WriteString("## API Overview\n\n")
		for _, pkg := range doc.Packages {
			sb.WriteString("### ")
			sb.WriteString(pkg.Name)
			sb.WriteString("\n\n")
			if pkg.Description != "" {
				sb.WriteString(pkg.Description)
				sb.WriteString("\n\n")
			}
			if len(pkg.Functions) > 0 {
				sb.WriteString("**Functions:**\n\n")
				for _, fn := range pkg.Functions {
					sb.WriteString("- `")
					sb.WriteString(fn.Name)
					sb.WriteString("` - ")
					if fn.Description != "" {
						sb.WriteString(fn.Description)
					} else {
						sb.WriteString("(no description)")
					}
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
			if len(pkg.Types) > 0 {
				sb.WriteString("**Types:**\n\n")
				for _, t := range pkg.Types {
					sb.WriteString("- `")
					sb.WriteString(t.Name)
					sb.WriteString("` (")
					sb.WriteString(t.Kind)
					sb.WriteString(")")
					if t.Description != "" {
						sb.WriteString(" - ")
						sb.WriteString(t.Description)
					}
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## License\n\n")
	sb.WriteString("See [LICENSE](LICENSE) for details.\n")

	return sb.String()
}

func (dg *DocGenerator) InferDescription(projectDir string) string {
	readmeNames := []string{"README.md", "README", "README.txt", "readme.md"}
	for _, name := range readmeNames {
		readmePath := filepath.Join(projectDir, name)
		data, err := os.ReadFile(readmePath)
		if err != nil {
			continue
		}
		desc := extractDescriptionFromREADME(string(data))
		if desc != "" {
			return desc
		}
	}

	goFiles, _ := filepath.Glob(filepath.Join(projectDir, "*.go"))
	if len(goFiles) > 0 {
		fset := token.NewFileSet()
		//lint:ignore SA1019 parser.ParseDir is deprecated; migration to go/packages is non-trivial
		pkgs, err := parser.ParseDir(fset, projectDir, nil, parser.ParseComments)
		if err == nil {
			for _, pkg := range pkgs {
				for _, file := range pkg.Files {
					if file.Doc != nil {
						doc := strings.TrimSpace(file.Doc.Text())
						if doc != "" {
							return doc
						}
					}
				}
			}
		}
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "module ") {
				moduleName := strings.TrimPrefix(line, "module ")
				return fmt.Sprintf("Go module: %s", strings.TrimSpace(moduleName))
			}
		}
	}

	return ""
}

func (dg *DocGenerator) inferArchitecture(packages []PackageDoc) string {
	if len(packages) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The project is organized into the following packages:\n\n")
	for _, pkg := range packages {
		sb.WriteString("- **")
		sb.WriteString(pkg.Name)
		sb.WriteString("**")
		if pkg.Path != "" {
			sb.WriteString(" (`")
			sb.WriteString(pkg.Path)
			sb.WriteString("`)")
		}
		if pkg.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(pkg.Description)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (dg *DocGenerator) generateQuickStart(doc *ProjectDoc) string {
	for _, pkg := range doc.Packages {
		if pkg.Name == "main" {
			return fmt.Sprintf("go run %s", pkg.Path)
		}
	}

	if len(doc.Packages) > 0 {
		return fmt.Sprintf("import \"%s\"", doc.Name)
	}

	return ""
}

func extractDescriptionFromREADME(content string) string {
	lines := strings.Split(content, "\n")
	var descLines []string
	pastTitle := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !pastTitle {
			if strings.HasPrefix(trimmed, "# ") || trimmed == "" {
				if strings.HasPrefix(trimmed, "# ") {
					pastTitle = true
				}
				continue
			}
			pastTitle = true
		}

		if pastTitle {
			if trimmed == "" {
				if len(descLines) > 0 {
					break
				}
				continue
			}
			if strings.HasPrefix(trimmed, "[![") || strings.HasPrefix(trimmed, "![") {
				continue
			}
			if strings.HasPrefix(trimmed, "## ") {
				break
			}
			descLines = append(descLines, trimmed)
		}
	}

	return strings.Join(descLines, " ")
}

func extractDocComment(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprToString(t.Elt)
		}
		return "[...]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + exprToString(t.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(t.Elt)
	default:
		return "unknown"
	}
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
