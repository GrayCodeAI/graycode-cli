# graycode-eco Implementation Roadmap

## Historical product roadmap

**Date:** 2026-07-05
**Source:** Historical comparison document; feature and market claims require
independent revalidation. Architecture status is tracked in
`docs/architecture/hawk-architecture-baseline.md`.

---

## Current Status

Numeric scores are intentionally not used as current architecture evidence.

| Category | Score | Max |
|----------|-------|-----|
| Core Features | 10/10 | 10 |
| Terminal Experience | 10/10 | 10 |
| Security | 10/10 | 10 |
| Architecture | 9/10 | 10 |
| Documentation | 8/10 | 10 |
| Community | 6/10 | 10 |
| IDE Features | 7/10 | 10 |

---

## Phase 1: High Priority (Immediate - Score Impact: +0.5)

### 1. Add VS Code Extension Integration (hawk repo)

**Effort:** Large (2-3 weeks)
**Priority:** HIGH

**What:**
- Create `hawk-vscode` repo for VS Code extension
- Use Hawk SDK for communication
- Add WebSocket transport for real-time updates

**Why:**
- Top 20 agents all have IDE integration
- Differentiate graycode-eco as terminal+IDE agent
- Capture market share from Cursor/Copilot users

**Implementation Plan:**
```bash
# Create new repo
mkdir hawk-vscode
cd hawk-vscode

# Initialize
git init
go mod init github.com/GrayCodeAI/hawk-vscode

# Add extension scaffold
mkdir -p cmd extension/src

# Add dependencies
go get github.com/GrayCodeAI/sparrow

# Add WebSocket transport
go get golang.org/x/net/websocket

# Create extension files
cat > extension/package.json << 'EOF'
{
  "name": "hawk-code",
  "displayName": "Hawk Code",
  "description": "AI coding assistant in terminal",
  "version": "0.1.0",
  "engines": { "vscode": "^1.90.0" },
  "categories": ["Programming Languages"],
  "activationEvents": ["*"],
  "main": "./out/extension.js",
  "contributes": {
    "commands": [
      {
        "command": "hawk.code.activate",
        "title": "Activate Hawk Code"
      }
    ],
    "menus": {
      "commandPalette": [
        {
          "command": "hawk.code.activate",
          "when": "editorLangId"
        }
      ]
    }
  }
}
EOF

# Add extension scaffold
cat > extension/src/extension.ts << 'EOF'
import * as vscode from 'vscode';
import * as hawksdk from '@graycodeai/hawksdk';

export function activate(context: vscode.ExtensionContext) {
  const client = new hawksdk.Client();
  
  // Register completion provider
  vscode.languages.registerCompletionItemProvider(
    'go',
    new HawkCompletionProvider(client),
    'g', 'h'
  );
  
  // Register hover provider
  vscode.languages.registerHoverProvider(
    'go',
    new HawkHoverProvider(client)
  );
}
EOF

# Build and publish
npm install
npm run build
vsce package
vsce publish
```

**Repository Structure:**
```
hawk-vscode/
├── extension/           # VS Code extension code
│   ├── src/
│   │   ├── extension.ts
│   │   ├── completion.ts
│   │   └── hover.ts
│   ├── package.json
│   └── tsconfig.json
├── server/              # Local server for communication
│   └── main.go
├── go.mod
├── Makefile
└── README.md
```

---

### 2. Add Extension Marketplace (hawk repo)

**Effort:** Medium (1-2 weeks)
**Priority:** HIGH

**What:**
- Create marketplace for community extensions
- Add extension discovery endpoint to hawk
- Create `starling` integration
- Add version compatibility checking

**Why:**
- Top agents have 100+ extensions
- Build ecosystem around Hawk
- Increase adoption

**Implementation Plan:**
```go
// Add to hawk
package marketplace

// Extension represents a community extension
type Extension struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Version     string            `json:"version"`
    Author      string            `json:"author"`
    Category    string            `json:"category"`
    Commands    []MarketplaceCommand `json:"commands"`
    Tools       []string          `json:"tools"`
    Config      map[string]string  `json:"config"`
}

// MarketplaceCommand represents an extension command
type MarketplaceCommand struct {
    Command     string `json:"command"`
    Title       string `json:"title"`
    Description string `json:"description"`
}

// Extension Discovery Endpoint
func (s *Session) MarketplaceEndpoints() []MarketplaceEndpoint {
    return []MarketplaceEndpoint{
        {
            Path:    "/api/marketplace/extensions",
            Method:  "GET",
            Handler: s.handleListExtensions,
        },
        {
            Path:    "/api/marketplace/extensions/{id}",
            Method:  "GET",
            Handler: s.handleGetExtension,
        },
        {
            Path:    "/api/marketplace/install",
            Method:  "POST",
            Handler: s.handleInstallExtension,
        },
    }
}

// Handle list extensions
func (s *Session) handleListExtensions(w http.ResponseWriter, r *http.Request) {
    extensions := []Extension{
        {
            ID: "community-skills-python",
            Name: "Python Community Skills",
            Description: "Essential Python skills for AI coding agents",
            Version: "1.0.0",
            Author: "GrayCodeAI",
            Category: "Language Support",
            Commands: []MarketplaceCommand{
                {Command: "python.run", Title: "Run Python", Description: "Execute Python code"},
                {Command: "python.debug", Title: "Debug Python", Description: "Debug with breakpoints"},
                {Command: "python.lint", Title: "Lint Python", Description: "Run linter on file"},
            },
            Tools: []string{"bash", "edit", "read", "write"},
            Config: map[string]string{
                "maxPythonVersion": "3.12",
            },
        },
        {
            ID: "community-skills-javascript",
            Name: "JavaScript Community Skills",
            Description: "Essential JavaScript skills for AI coding agents",
            Version: "1.0.0",
            Author: "GrayCodeAI",
            Category: "Language Support",
            Commands: []MarketplaceCommand{
                {Command: "javascript.run", Title: "Run JavaScript", Description: "Execute JavaScript code"},
                {Command: "javascript.test", Title: "Test JavaScript", Description: "Run tests"},
            },
            Tools: []string{"bash", "edit", "read", "write"},
            Config: map[string]string{
                "maxJSVersion": "ES2022",
            },
        },
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(extensions)
}

// Handle get extension
func (s *Session) handleGetExtension(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    
    extension := getExtensionByID(id)
    if extension == nil {
        http.Error(w, "Extension not found", http.StatusNotFound)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(extension)
}

// Handle install extension
func (s *Session) handleInstallExtension(w http.ResponseWriter, r *http.Request) {
    var req InstallRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // Install extension
    err := installExtension(req.ID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "installed"})
}
```

**Update starling:**
```bash
# Add extension manifest
mkdir -p .extensions/python

cat > .extensions/python/manifest.json << 'EOF'
{
  "id": "community-skills-python",
  "name": "Python Community Skills",
  "version": "1.0.0",
  "description": "Essential Python skills for AI coding agents",
  "category": "Language Support",
  "commands": [
    {
      "id": "python.run",
      "title": "Run Python",
      "description": "Execute Python code"
    }
  ],
  "tools": ["bash", "edit", "read", "write"]
}
EOF

# Update community-skills to expose extensions
cat >> SKILL.md << 'EOF'

## Extension Discovery

To discover available extensions, use:
```
hawk extensions list
```

To install an extension:
```
hawk extensions install <extension-id>
```
EOF
```

---

## Phase 2: Medium Priority (Next Sprint - Score Impact: +0.3)

### 3. Add Documentation Site (graycode-platform)

**Effort:** Large (2-3 weeks)
**Priority:** MEDIUM

**What:**
- Create documentation website
- Document all APIs and protocols
- Add tutorials and guides

**Why:**
- Professional appearance
- Easier onboarding
- Better adoption

**Implementation Plan:**
```bash
# Create docs site
mkdir docs-site
cd docs-site

# Initialize with Docusaurus or Next.js
npx create-docusaurus-app@latest . --template classic

# Add API documentation
mkdir -p docs/api

# Add tutorials
mkdir -p docs/tutorials

# Add architecture docs
mkdir -p docs/architecture

# Build and deploy
npm run build
npm run deploy
```

---

### 4. Add Community Forum (graycode-platform)

**Effort:** Large (2-3 weeks)
**Priority:** MEDIUM

**What:**
- Create forum for discussions
- Moderate discussions
- Share updates and roadmap

**Why:**
- Improve community engagement
- Better support
- Knowledge sharing

**Implementation Plan:**
```bash
# Create forum
mkdir forum
cd forum

# Use Discourse or custom solution
# Option 1: Discourse (self-hosted)
git clone https://github.com/discourse/discourse.git

# Option 2: Custom lightweight forum
cat > forum.go << 'EOF'
package main

import (
    "net/http"
    "html/template"
)

type Forum struct {
    posts []Post
}

func NewForum() *Forum {
    return &Forum{posts: []Post{}}
}

func (f *Forum) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/":
        f.handleIndex(w, r)
    case "/post":
        f.handleCreatePost(w, r)
    case "/post/{id}":
        f.handleGetPost(w, r)
    }
}

func (f *Forum) handleIndex(w http.ResponseWriter, r *http.Request) {
    tmpl.Execute(w, f.posts)
}

func (f *Forum) handleCreatePost(w http.ResponseWriter, r *http.Request) {
    post := Post{}
    json.NewDecoder(r.Body).Decode(&post)
    f.posts = append(f.posts, post)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}
EOF

# Build and deploy
go build forum.go
./forum
```

---

### 5. Add More Extension Points (hawk)

**Effort:** Small (3-5 days)
**Priority:** MEDIUM

**What:**
- Add filesystem MCP server
- Add git MCP server
- Add code search MCP server

**Why:**
- Increase ecosystem value
- More integrations
- Better extensibility

**Implementation Plan:**
```go
// Create new MCP server for filesystem
package main

import (
    "github.com/GrayCodeAI/falcon"
    "github.com/mark3labs/mcp-go/mcp"
)

func main() {
    s := mcpkit.New("filesystem", "0.1.0")
    
    s.AddTool(
        mcp.NewTool("read",
            mcp.WithDescription("Read file contents"),
            mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
        ),
        func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            path := mcpkit.StrArg(req, "path")
            content, err := os.ReadFile(path)
            if err != nil {
                return mcpkit.JSONResult(map[string]string{"error": err.Error()}), nil
            }
            return mcpkit.JSONResult(map[string]string{"content": string(content)}), nil
        },
    )
    
    s.AddTool(
        mcp.NewTool("write",
            mcp.WithDescription("Write file contents"),
            mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
            mcp.WithString("content", mcp.Required(), mcp.Description("File contents")),
        ),
        func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            path := mcpkit.StrArg(req, "path")
            content := mcpkit.StrArg(req, "content")
            err := os.WriteFile(path, []byte(content), 0644)
            if err != nil {
                return mcpkit.JSONResult(map[string]string{"error": err.Error()}), nil
            }
            return mcpkit.JSONResult(map[string]string{"status": "ok"}), nil
        },
    )
    
    _ = s.ServeStdio()
}
```

---

## Phase 3: Low Priority (Backlog - Score Impact: +0.2)

### 6. Add AI Code Completion (eyrie)

**Effort:** Medium (1-2 weeks)
**Priority:** LOW

**What:**
- Add completion endpoint
- Integrate with editor protocols

**Why:**
- Match IDE agent capabilities
- Complete terminal experience

---

### 7. Add Debugging Support (hawk)

**Effort:** Large (2-3 weeks)
**Priority:** LOW

**What:**
- Add debug MCP server
- Support breakpoints
- Add variable inspection

**Why:**
- Complete IDE-like experience
- More features than competitors

---

### 8. Add Web UI for Monitoring (hawk)

**Effort:** Small (3-5 days)
**Priority:** LOW

**What:**
- Add WebSocket monitoring
- Create dashboard UI
- Add metrics visualization

**Why:**
- Better developer experience
- More professional appearance

---

### 9. Add SDK Analytics (both SDKs)

**Effort:** Small (2-3 days)
**Priority:** LOW

**What:**
- Track SDK usage
- Add metrics endpoints
- Create public dashboard

**Why:**
- Measure adoption
- Improve SDK quality

---

### 10. Add Completion Endpoint to eyrie

**Effort:** Medium (1 week)
**Priority:** LOW

**What:**
- Add completion endpoint
- Optimize for speed

**Why:**
- Enable AI completion
- Match top agents

---

## Detailed Repo-Specific Roadmap

### **hawk** (Main Repo) - Primary Product

| Phase | Improvement | Effort | Impact | Status |
|-------|--------------|--------|--------|--------|
| 1 | Add VS Code extension integration | Large | +0.5 | Planned |
| 1 | Add extension marketplace | Medium | +0.4 | Planned |
| 2 | Add debugging support | Large | +0.3 | Planned |
| 2 | Add more MCP servers | Small | +0.2 | Planned |
| 3 | Add Web UI for monitoring | Small | +0.2 | Planned |
| 3 | Add SDK analytics | Small | +0.1 | Planned |

Current architecture status: see `docs/architecture/hawk-architecture-baseline.md`.

---

### **graycode-platform** (Web and Cloud Platform)

| Phase | Improvement | Effort | Impact | Status |
|-------|--------------|--------|--------|--------|
| 1 | Add documentation site | Large | +0.3 | Planned |
| 2 | Add community forum | Large | +0.2 | Planned |
| 3 | Add API analytics | Medium | +0.2 | Planned |

**Historical self-assessment; no current numeric score is assigned.**

---

### **sparrow** (Go SDK)

| Phase | Improvement | Effort | Impact | Status |
|-------|--------------|--------|--------|--------|
| 3 | Add SDK analytics | Small | +0.1 | Planned |
| 3 | Add IDE integration examples | Small | +0.2 | Planned |

**Historical self-assessment; no current numeric score is assigned.**

---

### **robin** (Python SDK)

| Phase | Improvement | Effort | Impact | Status |
|-------|--------------|--------|--------|--------|
| 3 | Add deprecation warnings | Small | +0.1 | Planned |
| 3 | Add type stubs | Small | +0.1 | Planned |

**Historical self-assessment; no current numeric score is assigned.**

---

### **eyrie** (LLM Runtime)

| Phase | Improvement | Effort | Impact | Status |
|-------|--------------|--------|--------|--------|
| 3 | Add completion endpoint | Medium | +0.2 | Planned |
| 3 | Add streaming optimizations | Small | +0.1 | Planned |

**Historical self-assessment; no current numeric score is assigned.**

---

### **Other Repos (shrike, swift, harrier, kestrel, merlin)**

| Repo | Phase | Improvement | Effort | Impact |
|------|-------|--------------|--------|--------|
| shrike | 3 | Add token usage prediction | Small | +0.1 |
| swift | 3 | Add swift sharing | Small | +0.1 |
| harrier | 3 | Add memory analytics | Small | +0.1 |
| kestrel | 3 | Add review templates | Small | +0.1 |
| merlin | 3 | Add verification templates | Small | +0.1 |

**Historical self-assessment; no current numeric score is assigned.**

---

## Roadmap sequencing

The roadmap is sequenced by product value and implementation effort. It does
not assign target architecture scores. Current architecture status is tracked
in `docs/architecture/hawk-architecture-baseline.md`.

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Marketplace competition | High | Focus on quality extensions |
| Community building | Medium | Active engagement, good docs |
| Development velocity | Medium | Prioritize hawk SDK first |
| Adoption curve | Medium | VS Code extension is key |

---

## Success Metrics

| Metric | Target |
|--------|--------|
| GitHub Stars | 10k+ |
| VS Code Extension Installs | 50k+ |
| Extension Count | 100+ |
| Community Forum Posts | 10k+ |
| SDK Downloads | 1M+ |

---

## Implementation Order

### 1. VS Code Extension (Critical Path)
- Foundation for all IDE features
- Must be first

### 2. Extension Marketplace (High Value)
- Builds ecosystem
- Drives adoption

### 3. Documentation Site (Foundation)
- Supports all other features
- Improves developer experience

### 4. Community Forum (Engagement)
- Increases community participation
- Supports marketplace adoption

### 5. Debugging Support (Feature Parity)
- Competes with IDE agents
- High effort, medium priority

### 6. AI Code Completion (Feature Parity)
- Competes with IDE agents
- High effort, medium priority

### 7. Web UI (Developer Experience)
- Nice to have
- Low effort, low priority

### 8. SDK Analytics (Measurement)
- Data-driven improvements
- Low effort, low priority

---

## Budget

| Phase | Developer Weeks | Cost |
|-------|-----------------|------|
| 1 | 3 weeks | $30k |
| 2 | 2 weeks | $20k |
| 3 | 1 week | $10k |
| **Total** | **6 weeks** | **$60k** |

---

## Timeline

| Phase | Start Date | End Date | Duration |
|-------|------------|----------|----------|
| 1 | 2026-07-06 | 2026-07-20 | 2 weeks |
| 2 | 2026-07-21 | 2026-08-04 | 2 weeks |
| 3 | 2026-08-05 | 2026-08-12 | 1 week |
| **Total** | **2026-07-06** | **2026-08-12** | **5 weeks** |

---

## Conclusion

The **implementation roadmap** focuses on:
1. **VS Code extension integration** (highest impact)
2. **Extension marketplace** (builds ecosystem)
3. **Documentation site** (foundation)
4. **Community forum** (engagement)
5. **Debugging and AI completion** (feature parity)

**Timeline:** 5 weeks for complete implementation
**Budget:** $60k for 6 developer weeks
**Target:** 10/10 score, compete with top coding agents

---

*Roadmap based on analysis of top 20 coding agents in GitHub Topics, AI coding benchmarks, and feature comparisons.*
