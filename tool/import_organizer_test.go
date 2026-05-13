package tool

import (
	"strings"
	"testing"
)

func TestOrganizeGo_GroupsCorrectly(t *testing.T) {
	input := `package main

import (
	"github.com/GrayCodeAI/hawk/engine"
	"fmt"
	"github.com/google/uuid"
	"os"
	"github.com/GrayCodeAI/hawk/tool"
	"strings"
)

func main() {
	fmt.Println(os.Getenv("HOME"))
	_ = strings.NewReader("")
	_ = uuid.New()
	_ = engine.New()
	_ = tool.New()
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	// Verify stdlib comes first.
	fmtIdx := strings.Index(result, `"fmt"`)
	osIdx := strings.Index(result, `"os"`)
	stringsIdx := strings.Index(result, `"strings"`)
	uuidIdx := strings.Index(result, `"github.com/google/uuid"`)
	engineIdx := strings.Index(result, `"github.com/GrayCodeAI/hawk/engine"`)
	toolIdx := strings.Index(result, `"github.com/GrayCodeAI/hawk/tool"`)

	if fmtIdx < 0 || osIdx < 0 || stringsIdx < 0 {
		t.Fatal("stdlib imports missing")
	}
	if uuidIdx < 0 {
		t.Fatal("external import missing")
	}
	if engineIdx < 0 || toolIdx < 0 {
		t.Fatal("internal imports missing")
	}

	// Stdlib should come before external.
	if fmtIdx > uuidIdx {
		t.Error("stdlib should come before external imports")
	}
	// External should come before internal.
	if uuidIdx > engineIdx {
		t.Error("external should come before internal imports")
	}
}

func TestOrganizeGo_SortsWithinGroups(t *testing.T) {
	input := `package main

import (
	"strings"
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv("HOME"))
	_ = strings.NewReader("")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	fmtIdx := strings.Index(result, `"fmt"`)
	osIdx := strings.Index(result, `"os"`)
	stringsIdx := strings.Index(result, `"strings"`)

	if fmtIdx > osIdx || osIdx > stringsIdx {
		t.Errorf("imports not sorted alphabetically: fmt@%d, os@%d, strings@%d", fmtIdx, osIdx, stringsIdx)
	}
}

func TestOrganizeGo_RemovesUnused(t *testing.T) {
	input := `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Println("hello")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	if strings.Contains(result, `"os"`) {
		t.Error("unused import 'os' should have been removed")
	}
	if strings.Contains(result, `"strings"`) {
		t.Error("unused import 'strings' should have been removed")
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("used import 'fmt' should be preserved")
	}
}

func TestOrganizeGo_PreservesAliases(t *testing.T) {
	input := `package main

import (
	f "fmt"
	_ "net/http/pprof"
)

func main() {
	f.Println("hello")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	if !strings.Contains(result, `f "fmt"`) {
		t.Error("alias 'f' for fmt should be preserved")
	}
	if !strings.Contains(result, `_ "net/http/pprof"`) {
		t.Error("blank import should be preserved")
	}
}

func TestOrganizeTypeScript_GroupsCorrectly(t *testing.T) {
	input := `import { readFile } from 'node:fs';
import express from 'express';
import { helper } from './utils';
import { join } from 'node:path';
import React from 'react';
import { Component } from '../components';

const app = express();
readFile('x', () => {});
join('a', 'b');
helper();
React.createElement('div');
Component();
`

	organizer := NewImportOrganizer("typescript")
	result, err := organizer.OrganizeTypeScript(input)
	if err != nil {
		t.Fatalf("OrganizeTypeScript failed: %v", err)
	}

	fsIdx := strings.Index(result, "node:fs")
	pathIdx := strings.Index(result, "node:path")
	expressIdx := strings.Index(result, "express")
	reactIdx := strings.Index(result, "'react'")
	utilsIdx := strings.Index(result, "./utils")
	componentsIdx := strings.Index(result, "../components")

	if fsIdx < 0 || pathIdx < 0 {
		t.Fatal("builtin imports missing")
	}
	if expressIdx < 0 || reactIdx < 0 {
		t.Fatal("external imports missing")
	}
	if utilsIdx < 0 || componentsIdx < 0 {
		t.Fatal("internal imports missing")
	}

	// Builtin before external.
	if fsIdx > expressIdx {
		t.Error("builtin imports should come before external")
	}
	// External before internal.
	if expressIdx > utilsIdx {
		t.Error("external imports should come before internal")
	}
	// Sorted within builtin.
	if fsIdx > pathIdx {
		t.Error("builtin imports should be sorted alphabetically")
	}
}

func TestOrganizeTypeScript_PreservesTypeImports(t *testing.T) {
	input := `import type { FC } from 'react';
import { useState } from 'react';
import type { Config } from './types';

const x: FC = () => null;
useState();
const c: Config = {};
`

	organizer := NewImportOrganizer("typescript")
	result, err := organizer.OrganizeTypeScript(input)
	if err != nil {
		t.Fatalf("OrganizeTypeScript failed: %v", err)
	}

	if !strings.Contains(result, "import type") {
		t.Error("type imports should be preserved")
	}
	if !strings.Contains(result, "{ FC }") {
		t.Error("type import names should be preserved")
	}
}

func TestAddMissingImport_Go(t *testing.T) {
	input := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.AddMissingImport(input, "os")
	if err != nil {
		t.Fatalf("AddMissingImport failed: %v", err)
	}

	if !strings.Contains(result, `"os"`) {
		t.Error("should contain the new import 'os'")
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("should still contain existing import 'fmt'")
	}
}

func TestAddMissingImport_GoNoExistingImports(t *testing.T) {
	input := `package main

func main() {
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.AddMissingImport(input, "fmt")
	if err != nil {
		t.Fatalf("AddMissingImport failed: %v", err)
	}

	if !strings.Contains(result, `"fmt"`) {
		t.Error("should contain the new import 'fmt'")
	}
	if !strings.Contains(result, "import") {
		t.Error("should have an import statement")
	}
}

func TestRemoveImport_Go(t *testing.T) {
	input := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.RemoveImport(input, "os")
	if err != nil {
		t.Fatalf("RemoveImport failed: %v", err)
	}

	if strings.Contains(result, `"os"`) {
		t.Error("removed import 'os' should not appear")
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("remaining import 'fmt' should be preserved")
	}
}

func TestRemoveImport_TS(t *testing.T) {
	input := `import { readFile } from 'node:fs';
import express from 'express';
import { helper } from './utils';

express();
helper();
`

	organizer := NewImportOrganizer("typescript")
	result, err := organizer.RemoveImport(input, "node:fs")
	if err != nil {
		t.Fatalf("RemoveImport failed: %v", err)
	}

	if strings.Contains(result, "node:fs") {
		t.Error("removed import should not appear")
	}
	if !strings.Contains(result, "express") {
		t.Error("other imports should be preserved")
	}
}

func TestEmptyImportBlock_Go(t *testing.T) {
	input := `package main

func main() {
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	// Should return content unchanged.
	if result != input {
		t.Errorf("content should be unchanged when no imports exist")
	}
}

func TestSingleImportNoParens_Go(t *testing.T) {
	input := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	organizer := NewImportOrganizer("go")
	result, err := organizer.OrganizeGo(input)
	if err != nil {
		t.Fatalf("OrganizeGo failed: %v", err)
	}

	if !strings.Contains(result, `"fmt"`) {
		t.Error("single import should be preserved")
	}
	// Single import should stay as single line (no parens).
	if strings.Contains(result, "import (") {
		t.Error("single import should not be wrapped in parens")
	}
}

func TestDetectUnusedGo(t *testing.T) {
	content := `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Println("hello")
}
`
	imports := []ImportEntry{
		{Path: "fmt", Alias: ""},
		{Path: "os", Alias: ""},
		{Path: "strings", Alias: ""},
	}

	organizer := NewImportOrganizer("go")
	unused := organizer.DetectUnusedGo(content, imports)

	if len(unused) != 2 {
		t.Fatalf("expected 2 unused imports, got %d", len(unused))
	}

	unusedPaths := make(map[string]bool)
	for _, u := range unused {
		unusedPaths[u.Path] = true
	}
	if !unusedPaths["os"] {
		t.Error("'os' should be detected as unused")
	}
	if !unusedPaths["strings"] {
		t.Error("'strings' should be detected as unused")
	}
}

func TestDetectUnusedTS(t *testing.T) {
	content := `import { useState, useEffect } from 'react';
import { helper } from './utils';

function App() {
	const [x, setX] = useState(0);
	return <div>{x}</div>;
}
`
	imports := []ImportEntry{
		{Path: "react", Alias: "{ useState, useEffect }"},
		{Path: "./utils", Alias: "{ helper }"},
	}

	organizer := NewImportOrganizer("typescript")
	unused := organizer.DetectUnusedTS(content, imports)

	// helper is unused, but useEffect is unused too - however both are in
	// combined imports. The detection checks if ANY name is used.
	// useState IS used, so react import is not "fully unused".
	// helper is NOT used, so ./utils is unused.
	if len(unused) != 1 {
		t.Fatalf("expected 1 unused import, got %d: %+v", len(unused), unused)
	}
	if unused[0].Path != "./utils" {
		t.Errorf("expected './utils' to be unused, got '%s'", unused[0].Path)
	}
}

func TestFormatImportBlock_Go(t *testing.T) {
	groups := []ImportGroup{
		{
			Name: "stdlib",
			Imports: []ImportEntry{
				{Path: "fmt"},
				{Path: "os"},
			},
		},
		{
			Name: "external",
			Imports: []ImportEntry{
				{Path: "github.com/google/uuid"},
			},
		},
	}

	organizer := NewImportOrganizer("go")
	result := organizer.FormatImportBlock(groups, "go")

	if !strings.Contains(result, "import (") {
		t.Error("should have grouped import block")
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("should contain fmt")
	}
	if !strings.Contains(result, `"github.com/google/uuid"`) {
		t.Error("should contain uuid")
	}

	// Should have a blank line between groups.
	lines := strings.Split(result, "\n")
	foundBlank := false
	for i, line := range lines {
		if line == "" && i > 0 && i < len(lines)-1 {
			foundBlank = true
			break
		}
	}
	if !foundBlank {
		t.Error("should have blank line between groups")
	}
}

func TestFormatImportBlock_TS(t *testing.T) {
	groups := []ImportGroup{
		{
			Name: "builtin",
			Imports: []ImportEntry{
				{Path: "node:fs", Alias: "{ readFile }"},
			},
		},
		{
			Name: "external",
			Imports: []ImportEntry{
				{Path: "react", Alias: "React"},
			},
		},
	}

	organizer := NewImportOrganizer("typescript")
	result := organizer.FormatImportBlock(groups, "typescript")

	if !strings.Contains(result, "import { readFile } from 'node:fs'") {
		t.Error("should contain builtin import")
	}
	if !strings.Contains(result, "import React from 'react'") {
		t.Error("should contain external import")
	}
}

func TestNewImportOrganizer(t *testing.T) {
	goOrg := NewImportOrganizer("go")
	if goOrg.Language != "go" {
		t.Errorf("expected language 'go', got '%s'", goOrg.Language)
	}
	if len(goOrg.GroupOrder) != 3 {
		t.Errorf("expected 3 group orders, got %d", len(goOrg.GroupOrder))
	}
	if goOrg.GroupOrder[0] != "stdlib" {
		t.Errorf("expected first group 'stdlib', got '%s'", goOrg.GroupOrder[0])
	}

	tsOrg := NewImportOrganizer("typescript")
	if tsOrg.Language != "typescript" {
		t.Errorf("expected language 'typescript', got '%s'", tsOrg.Language)
	}
	if tsOrg.GroupOrder[0] != "builtin" {
		t.Errorf("expected first group 'builtin', got '%s'", tsOrg.GroupOrder[0])
	}
}

func TestImportOrganizerTool_Interface(t *testing.T) {
	tool := ImportOrganizerTool{}
	if tool.Name() != "OrganizeImports" {
		t.Errorf("expected name 'OrganizeImports', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Error("parameters should not be nil")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["path"]; !ok {
		t.Error("parameters should have 'path' property")
	}
}
