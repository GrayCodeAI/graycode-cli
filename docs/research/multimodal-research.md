# Multi-Modal Capabilities for Coding Agents -- State of the Art Research

Research compiled 2026-05-03. Sources: GitHub repos, API documentation, arXiv papers, and project documentation for 50+ systems.

---

## Table of Contents

1. [Screenshot-to-Code Tools](#1-screenshot-to-code-tools)
2. [Vision-Based UI Testing](#2-vision-based-ui-testing)
3. [Design-to-Code Pipelines](#3-design-to-code-pipelines)
4. [OCR for Code Extraction](#4-ocr-for-code-extraction)
5. [Diagram Understanding](#5-diagram-understanding)
6. [Visual Debugging](#6-visual-debugging)
7. [Whiteboard-to-Code](#7-whiteboard-to-code)
8. [Multi-Modal RAG](#8-multi-modal-rag)
9. [Vision Models for UI Analysis](#9-vision-models-for-ui-analysis)
10. [Web Agent Vision](#10-web-agent-vision)
11. [Mobile App Screenshot Analysis](#11-mobile-app-screenshot-analysis)
12. [PDF/Document Understanding](#12-pdfdocument-understanding)
13. [Chart/Graph Data Extraction](#13-chartgraph-data-extraction)
14. [Terminal Screenshot Understanding](#14-terminal-screenshot-understanding)
15. [Git Diff Visualization](#15-git-diff-visualization)
16. [Code Visualization](#16-code-visualization)
17. [Automated Screenshot Capture](#17-automated-screenshot-capture)
18. [Image Generation for Documentation](#18-image-generation-for-documentation)
19. [SVG/Diagram Generation](#19-svgdiagram-generation)
20. [Accessibility Testing via Screenshot](#20-accessibility-testing-via-screenshot)
21. [Hawk Current State and Gaps](#21-hawk-current-state-and-gaps)
22. [Recommended Implementation Plan](#22-recommended-implementation-plan)

---

## 1. Screenshot-to-Code Tools

### screenshot-to-code (Abi) -- 72.4k stars

**What it does.** Converts screenshots, mockups, and Figma designs into clean, functional code. Users upload an image and receive working code in their chosen framework.

**How well it works.** The tool produces usable code for static UIs with good accuracy for common layouts. It supports HTML+Tailwind, React+Tailwind, Vue+Tailwind, Bootstrap, Ionic+Tailwind, and SVG output. Complex interactive UIs, custom animations, and deeply nested state management remain weak areas. Experimental video-to-code support exists but is unreliable.

**Models.** Claude Opus 4.5, GPT-5.3/5.2/4.1, Gemini 3 Flash/Pro. Uses DALL-E 3 or Flux Schnell for image generation within mockups.

**Architecture.** React/Vite frontend, FastAPI backend. The pipeline sends the screenshot to a vision model with a detailed system prompt describing the target framework, then iterates on the result.

### v0 (Vercel)

**What it does.** Vercel's generative UI system that takes text descriptions or screenshots and produces React/Next.js components using shadcn/ui and Tailwind.

**How well it works.** Strong for component-level generation (buttons, cards, forms, dashboards). Weaker for full application architecture. The generated code uses modern React patterns and is production-quality for UI components. It integrates tightly with the Vercel deployment ecosystem.

**Key insight.** v0 shows that constraining output to a specific design system (shadcn/ui) dramatically improves quality versus open-ended code generation.

### bolt.new (StackBlitz) -- open source

**What it does.** AI-powered web development agent running entirely in the browser via WebContainers. Can prompt, run, edit, and deploy full-stack JavaScript applications.

**How well it works.** Strong for JavaScript/Node.js ecosystems. The browser-based sandbox means full environment control (filesystem, npm, terminal) without local setup. Limited to frameworks compatible with StackBlitz WebContainers.

### Napkins.dev -- open source

**What it does.** Wireframe-to-app generator. Takes hand-drawn sketches or screenshots and generates functional applications.

**Models.** Uses Kimi K2.5 on Together AI inference. Code runs in Sandpack sandbox.

### micro-agent (Builder.io)

**What it does.** CLI tool that generates code from prompts, then iterates via test-driven development until tests pass. Includes experimental visual matching -- provide a design screenshot alongside code, and it generates code to match the visual.

**How well it works.** Deliberately narrow scope (single file, won't install deps or modify multiple files). The visual matching requires an Anthropic API key for Claude vision feedback while using OpenAI for code generation. The focused approach avoids compounding errors common in broader agents.

### How a coding agent should use it

A coding agent should offer a `/screenshot-to-code` or `/mockup` command that:
1. Accepts an image path or clipboard screenshot
2. Detects the project's framework from package.json/go.mod
3. Sends the screenshot to a vision model with framework-specific prompts
4. Generates component code in the correct framework
5. Writes files and optionally runs the dev server to verify

---

## 2. Vision-Based UI Testing

### Applitools Eyes

**What it does.** Visual AI testing platform that captures application screens and uses AI to detect visual regressions. Replicates human perception instead of pixel-by-pixel comparison.

**How well it works.** 99.8% pass percentage reported by customers. Intelligently distinguishes dynamic content (ads, dates, personalized dashboards) from genuine defects. Reduces false positives dramatically compared to pixel-diff tools. The AI learns what "looks right" for your application.

**Key capabilities.** Cross-browser testing, intelligent grouping of changes for batch review, one-click maintenance for expected changes.

### Playwright Visual Testing

**What it does.** Built-in `toHaveScreenshot()` assertion that captures screenshots and performs pixel-level comparison using the pixelmatch library.

**How well it works.** Reliable for deterministic UIs when run in consistent environments. Requires same OS/browser/resolution for baseline matching. Provides `maxDiffPixels` tolerance and `stylePath` for hiding volatile elements.

**Limitations.** Pixel-based comparison is brittle. Browser rendering varies by OS, hardware, and version. No semantic understanding of what changed.

### Midscene.js -- AI-powered UI automation

**What it does.** Uses vision language models (VLMs) to understand UIs purely from screenshots. Enables natural language automation across web, mobile, desktop, and canvas surfaces without DOM parsing.

**How well it works.** Supports Qwen3-VL, Gemini-3-Pro, UI-TARS, and Doubao-1.6-vision models. Pure vision approach means it works even on canvas elements and non-DOM surfaces where traditional testing fails. Reduced token costs versus full DOM + screenshot approaches.

### How a coding agent should use it

A coding agent should support a `/visual-test` command that:
1. Takes a before/after screenshot (or captures one via headless browser)
2. Sends both images to a vision model
3. Gets a semantic diff: "The navigation bar shifted 2px left, the button text changed from 'Submit' to 'Send'"
4. Reports whether changes are intentional or regressions
5. Can generate Playwright snapshot assertions automatically

---

## 3. Design-to-Code Pipelines

### Anima -- 1.5M users

**What it does.** AI-powered design-to-code platform. Imports Figma designs, text prompts, or images and generates functional applications. Exports to React, HTML, and Vue.

**Pipeline.** Import design -> instant generation -> iterate via chat -> deploy with one click. Also supports website cloning via browser extension.

**Key feature.** API access for integrating with coding AI agents, making it composable in automated workflows.

### Locofy.ai

**What it does.** Design-to-code tool focused on converting Figma/Adobe XD designs to production-ready frontend code. Emphasizes responsive layouts and component extraction.

### Figma API + Vision Models (emerging pattern)

**What it does.** The most effective current approach combines Figma's structured JSON export (frames, components, auto-layout properties) with vision model analysis of the rendered design. The structured data provides exact spacing/colors/typography while the vision model handles intent and interaction patterns.

### How a coding agent should use it

A coding agent should implement a `/design` or `/figma` command that:
1. Accepts a Figma URL, image file, or clipboard paste
2. For Figma URLs: fetches the design JSON via Figma API for precise measurements
3. For images: uses vision model to extract layout structure, colors, typography
4. Maps design elements to the project's existing component library
5. Generates code that reuses existing components rather than creating duplicates
6. Highlights elements that don't match existing patterns for human review

---

## 4. OCR for Code Extraction

### Tesseract -- 73.9k stars

**What it does.** Open-source OCR engine combining LSTM neural networks with legacy pattern-recognition. Supports 100+ languages, outputs plain text, hOCR, PDF, TSV, ALTO, and PAGE formats.

**How well it works for code.** General-purpose OCR, not specialized for code. Struggles with monospace font ligatures, syntax highlighting colors on dark backgrounds, and low-resolution terminal screenshots. Accuracy depends heavily on image quality.

### Vision Model OCR (Claude, GPT-4V, Gemini)

**What it does.** Modern vision models can read code from screenshots with near-perfect accuracy for clear images. They understand syntax highlighting, can infer indentation, and recognize programming languages.

**How well it works.** Far superior to traditional OCR for code. Claude and GPT-4V can read code from terminal screenshots, IDE screenshots, documentation screenshots, and even handwritten code with high accuracy. They also understand context -- they can identify the language, spot errors, and explain what the code does.

**Key advantage.** Vision models handle the full range of code presentation: dark/light themes, syntax highlighting, line numbers, diff markers, error underlines, and annotations.

### How a coding agent should use it

A coding agent should:
1. Accept image inputs in the conversation (paste, path, URL)
2. Automatically detect if an image contains code
3. Extract the code using the vision model (not traditional OCR)
4. Offer to create a file with the extracted code
5. Handle terminal screenshots by extracting both commands and output

---

## 5. Diagram Understanding

### Mermaid -- 87.8k stars

**What it does.** Text-to-diagram generation from markdown-like syntax. Supports flowcharts, sequence diagrams, Gantt charts, class diagrams, state diagrams, pie charts, git graphs, user journey diagrams, and C4 architecture diagrams.

**Programmatic parsing.** Mermaid diagrams are text-based and can be parsed to extract architectural information: which components exist, how they connect, what the data flow looks like. The mermaid-cli (`mmdc`) provides a Node.js API for rendering.

### D2 -- text-to-diagram language

**What it does.** Modern diagram scripting language that compiles to SVG/PNG/PDF. Uses plugin-based layout engines (ELK, TALA) for different diagram styles. Has official VSCode, Vim, and Obsidian plugins.

**Advantage over Mermaid.** Runs entirely server-side without browser dependencies, making it suitable for automated generation in CI/CD pipelines and coding agents.

### Excalidraw -- 122k stars

**What it does.** Virtual hand-drawn style whiteboard. Exports to PNG, SVG, and an open `.excalidraw` JSON format. The JSON format contains structured shape/connection data that can be parsed programmatically.

### Vision models + diagrams

**What it does.** Current vision models (Claude Opus 4.7, GPT-4V, Gemini) can interpret architecture diagrams, flowcharts, sequence diagrams, and ERD diagrams from images with moderate accuracy. They can describe the components, identify relationships, and translate diagrams into text descriptions or code structures.

**Limitations.** Complex diagrams with many overlapping connections, small text, or unconventional layouts degrade accuracy. Best when diagrams follow standard conventions.

### How a coding agent should use it

A coding agent should:
1. Parse Mermaid/D2/PlantUML in documentation to understand architecture
2. Accept architecture diagram images and extract structure via vision
3. Generate Mermaid/D2 diagrams from code analysis (reverse engineering)
4. Offer a `/diagram` command that generates architecture diagrams from the current codebase
5. Translate between diagram formats (Mermaid -> D2, image -> Mermaid)

---

## 6. Visual Debugging

### Error screenshot understanding

**What it does.** Vision models can analyze screenshots of error states: browser console errors, terminal stack traces, IDE error panels, and application error screens. They identify the error type, location, and likely cause.

**How well it works.** Very effective for common error patterns. Claude and GPT-4V can read stack traces from terminal screenshots, identify the failing line, correlate with code context, and suggest fixes. Browser DevTools screenshots (Network tab, Console, Elements) are well understood.

### Stack trace visualization

**What it does.** Tools like Better Stack, Sentry, and LogRocket capture and visualize errors with context. When integrated with a coding agent, the agent can see the same visual representation a developer would see.

**Emerging pattern.** Agents that can take a screenshot of the error state (browser, terminal, IDE) and automatically diagnose the issue without the developer needing to manually copy error text.

### How a coding agent should use it

A coding agent should:
1. Accept error screenshots via paste or path
2. Extract error text, stack trace, and context from the image
3. Cross-reference with project source files
4. Suggest specific fixes with code diffs
5. Optionally capture its own screenshots when running browser tests

---

## 7. Whiteboard-to-Code

### make-real (tldraw) -- archived Feb 2026

**What it does.** Users draw UI sketches on the tldraw whiteboard, and the tool converts them into functional HTML/CSS/JS code using vision models.

**How well it works.** Impressive for simple UIs (landing pages, forms, dashboards). The hand-drawn aesthetic of tldraw actually helps the model understand intent vs. precise positioning. Works best when combined with text annotations on the whiteboard.

### Excalidraw + AI (emerging)

**What it does.** Similar concept using Excalidraw's structured JSON export. The combination of structured shape data + visual rendering gives better results than pure image analysis.

### How a coding agent should use it

A coding agent should support a `/sketch` or `/whiteboard` command that:
1. Accepts a hand-drawn sketch image (photo of whiteboard, tablet sketch, Excalidraw export)
2. Uses vision model to identify UI components (buttons, inputs, lists, navigation)
3. Maps to the project's design system/component library
4. Generates a first-pass implementation
5. Enters an iterative refinement loop: "Move the sidebar to the left", "Make the header sticky"

---

## 8. Multi-Modal RAG

### Milvus -- vector database

**What it does.** High-performance vector database supporting dense/sparse embeddings, multi-vector storage, and hybrid search. Supports text, image, and video modalities in a single collection.

**Image search.** Images are converted to vector embeddings (typically 768+ dimensions) and stored alongside metadata. Search uses approximate nearest neighbor (ANN) algorithms (HNSW, IVF).

### Kotaemon -- open-source RAG platform

**What it does.** RAG-based QA on document collections with explicit multi-modal support. Handles documents with figures and tables using Azure Document Intelligence, Adobe PDF Extract, or Docling for parsing.

**Key feature.** Hybrid retrieval combining full-text and vector search with re-ranking.

### Multi-modal RAG pattern

**Architecture.** The state-of-the-art approach for multi-modal RAG:
1. **Document ingestion**: Extract text, tables, and images separately
2. **Embedding**: Use CLIP or similar for image embeddings, text embedders for text
3. **Storage**: Store all modalities in a vector database with metadata
4. **Retrieval**: Query across modalities -- a text query can retrieve relevant images, a diagram query can retrieve related code
5. **Generation**: Present retrieved multi-modal context to a vision model

### Jina Reader

**What it does.** Free API that converts URLs to LLM-friendly content. Automatically captions images using vision language models, formatted as alt tags.

### How a coding agent should use it

A coding agent should:
1. Index project documentation including embedded images and diagrams
2. When answering questions, retrieve relevant images alongside text
3. Include design mockups, architecture diagrams, and screenshots in RAG context
4. Support queries like "show me the diagram from the design doc that describes the auth flow"

---

## 9. Vision Models for UI Analysis

### Claude Vision (Anthropic)

**Capabilities.** Supports JPEG, PNG, GIF, WebP. Up to 600 images per API request. Max 8000x8000px per image. Images are tokenized at roughly `width * height / 750` tokens.

**Claude Opus 4.7 high-res.** First Claude model with high-resolution image support: 2576px on the long edge (up from 1568px), 4784 tokens per image (up from 1568). Automatic, no opt-in needed. Particularly strong for computer use, screenshot understanding, and document analysis.

**Best practices.** Place images before text in prompts. Use base64-encoded images, URL references, or the Files API. Downsample before sending to control costs. Lossy JPEG compression reduces latency but can degrade OCR accuracy.

**Cost.** ~$0.004 per 1000x1000px image on Sonnet, ~$0.007 on Opus 4.7.

### GPT-4V / GPT-5.x (OpenAI)

**Capabilities.** Strong visual understanding across screenshots, diagrams, charts, and documents. The newer GPT-5.x series improves on spatial reasoning and precise text extraction.

### Gemini Vision (Google)

**Capabilities.** Natively multimodal -- built from the ground up to process images alongside text. Supports PNG, JPEG, WEBP, HEIC, HEIF. Images tokenized at 258 tokens for images under 384px, with 768x768 tiles for larger images at 258 tokens each.

**Key advantage.** Up to 3,600 images per request. `media_resolution` parameter controls detail level for cost/quality tradeoff.

### Qwen2.5-VL (Alibaba)

**Capabilities.** Open-source vision-language model in 3B, 7B, and 72B sizes. 256K context window. Strong OCR supporting 32 languages. Robust in low light, blur, and tilt. Good at document layout, UI parsing, and object grounding.

### OmniParser (Microsoft)

**What it does.** Parses UI screenshots into structured elements using YOLO-based icon detection + Florence/BLIP2 captioning. Achieves 39.5% on ScreenSpot Pro benchmark.

**Key contribution.** Significantly enhances GPT-4V's ability to ground actions to specific UI regions.

### VLMEvalKit -- evaluation toolkit

**What it does.** Evaluates 220+ vision-language models across 80+ benchmarks. Covers visual QA, OCR, physics reasoning, spatial understanding, video comprehension, and medical imaging.

**Key finding from benchmarks.** GPT-4V and Claude Opus 4.7 lead on screenshot and UI understanding tasks. Open-source models like Qwen2.5-VL-72B are competitive for OCR and document tasks but lag on complex reasoning about UIs.

### How a coding agent should use it

A coding agent should:
1. Use vision model capabilities already available through the configured provider
2. Detect when the current model supports vision (check `Capabilities.Vision` in router)
3. Automatically route image-containing messages to vision-capable models
4. Apply appropriate image preprocessing: resize to optimal dimensions, compress for cost control
5. Use Claude Opus 4.7's high-res mode for detailed screenshots, Sonnet/Haiku for quick analysis

---

## 10. Web Agent Vision

### browser-use -- 91.7k stars

**What it does.** Python library enabling AI agents to control browsers. Takes screenshots, identifies interactive elements, executes browser commands via LLM decisions.

**Models.** ChatBrowserUse (optimized), OpenAI, Gemini, Claude, Ollama, and open-source bu-30b-a3b.

**How well it works.** Strong for multi-step web tasks: form filling, navigation, data extraction. The vision-based approach handles dynamic content and JavaScript-heavy sites.

### LaVague -- web agent framework

**What it does.** Two-component architecture: World Model (analyzes current page state) + Action Engine (converts instructions to Selenium/Playwright code). Uses GPT-4o by default, fully customizable.

**Capabilities.** Iframe handling, multi-tab navigation, Gradio interface for testing.

### SeeAct -- academic web agent

**What it does.** Uses vision models to interact with web pages through screenshots rather than HTML parsing. Supports Set-of-Mark (SoM) annotations that overlay visual identifiers on page elements for precise grounding.

**Models.** GPT-4V, Gemini, LLaVA (open-source).

### Stagehand -- browser automation framework

**What it does.** Combines AI with code for flexible browser automation. `act()` for actions, `extract()` for structured data extraction with Zod schema validation. Self-healing via action caching.

**Key pattern.** Moves from AI exploration to cached/replayed workflows, reducing token costs over time.

### How a coding agent should use it

A coding agent should support a `/browse` or `/web` command that:
1. Launches a headless browser
2. Navigates to a URL and takes a screenshot
3. Uses the vision model to understand the page
4. Extracts data or performs actions as instructed
5. Captures before/after screenshots for verification
6. Useful for: verifying deployed changes, scraping docs, testing integrations

---

## 11. Mobile App Screenshot Analysis

### Vision model approach (current best)

**What it does.** Modern vision models understand mobile app screenshots well. They can identify UI components (tab bars, navigation, cards, lists), read text, understand layout hierarchy, and detect platform-specific patterns (iOS vs Android).

### OmniParser for mobile

**What it does.** Microsoft's OmniParser works on mobile screenshots too: detects interactive regions, generates functional descriptions of UI elements.

### Midscene.js for mobile

**What it does.** Supports iOS and Android automation using vision-only localization. Works without accessibility tree access.

### UFO (Microsoft) for desktop/mobile

**What it does.** Multi-device orchestration framework. UFO3 ("Galaxy") supports cross-device workflows. Uses visual + accessibility API detection for robust element identification.

### How a coding agent should use it

A coding agent working on mobile projects should:
1. Accept mobile screenshots for UI implementation reference
2. Detect platform (iOS/Android) from screenshot characteristics
3. Generate platform-appropriate code (SwiftUI, Jetpack Compose, React Native)
4. Compare implementation screenshots against design reference
5. Identify platform-specific issues (safe areas, notch handling, gesture conflicts)

---

## 12. PDF/Document Understanding

### Docling (IBM) -- 59k stars

**What it does.** Converts complex documents (PDF, DOCX, PPTX, XLSX, HTML, images, LaTeX) into LLM-ready markdown/JSON. Handles page layout detection, reading order, table structure, code blocks, formulas, image classification, and chart understanding.

**How well it works.** VLM + OCR dual engine for improved accuracy. Supports 109 languages. Charts are converted to tables, code, or detailed descriptions.

### MinerU -- 61.7k stars

**What it does.** Document parsing engine converting PDFs and Office docs into structured markdown/JSON. VLM + OCR dual engine. Handles text, tables (HTML format), images, formulas (LaTeX), and handwritten content.

**How well it works.** Strong on academic papers, technical documentation, and business documents. Supports 109 languages. Available as web app, desktop client, Docker, Python SDK, and REST API.

### Marker -- 34.6k stars

**What it does.** Converts PDFs and documents to markdown/JSON/HTML using a multi-model pipeline: OCR (Surya), layout detection, block cleaning, optional LLM enhancement.

**How well it works.** Benchmark score of 95.67 vs LlamaParse (84.2) and Mathpix (86.4). Processing at 2.84 seconds/page on H100 GPU. Hybrid mode with LLMs improves table accuracy to 0.907.

### Camelot -- 3.7k stars

**What it does.** Python library for table extraction from text-based PDFs. Outputs to CSV, JSON, Excel, HTML, Markdown, SQLite. Reports 99% accuracy on clean documents. Does not work with scanned PDFs.

### Claude vision for PDFs

**What it does.** Claude can directly process PDF pages as images. Claude Opus 4.7's high-resolution mode (2576px) is particularly effective for dense documents with small text, tables, and diagrams.

### Hawk's current state

Hawk already has `readPDFFile()` in `tool/file_read_media.go` with page range parsing, size limits, and magic byte validation. Currently shells out to `pdftotext` (poppler-utils) but returns a fallback message if unavailable.

### How a coding agent should use it

A coding agent should:
1. Accept PDF/document paths and extract structured content automatically
2. For technical specs: extract requirements, API definitions, data models
3. For design docs: extract wireframes, flow descriptions, component lists
4. Integrate with the RAG system so document content is searchable
5. Handle mixed content: text paragraphs, tables, code blocks, diagrams
6. Upgrade from pdftotext to a vision-model approach for scanned/complex PDFs

---

## 13. Chart/Graph Data Extraction

### Vision model extraction (current best)

**What it does.** Vision models can extract data from charts: bar charts, line graphs, pie charts, scatter plots, and tables. They identify axes, labels, data points, trends, and relationships.

**How well it works.** Claude and GPT-4V are strong on standard chart types. They can extract approximate numerical values, identify trends ("revenue grew 30% YoY"), and describe relationships. Precise value extraction requires high-resolution images.

### Docling chart understanding

**What it does.** Docling specifically supports chart understanding: converts bar charts, pie charts, and line plots into tables, code, or detailed descriptions.

### Pix2Struct (Google DeepMind)

**What it does.** Model trained specifically for parsing screenshots into structured output. Understands charts, documents, and UIs. Pre-trained on web page screenshots, fine-tuned for specific tasks.

### How a coding agent should use it

A coding agent should:
1. Accept chart/graph images and extract data automatically
2. Convert chart data to structured formats (CSV, JSON, table)
3. Generate code to recreate the chart from extracted data
4. Answer questions about chart data: "What was the peak value in Q3?"
5. Detect data visualization in project documentation and index it

---

## 14. Terminal Screenshot Understanding

### Vision model approach (current best)

**What it does.** Vision models can read terminal screenshots including:
- Command output with ANSI colors
- Error messages with stack traces
- Build logs with warnings/errors highlighted
- htop/top system monitoring output
- Git log with branch visualization

**How well it works.** Very strong for standard terminal output. The models understand ANSI color semantics (red = error, green = success, yellow = warning). They can parse complex formatted output like tables, progress bars, and tree structures.

**Limitations.** Very dense terminal output (100+ lines of small text) may lose detail at standard resolutions. Opus 4.7's high-res mode helps here.

### How a coding agent should use it

A coding agent should:
1. Accept terminal screenshots when users paste error output
2. Extract command, output, and error information
3. Understand build system output (webpack, go build, cargo, etc.)
4. Parse CI/CD log screenshots from GitHub Actions, Jenkins, etc.
5. Automatically suggest fixes based on detected error patterns

---

## 15. Git Diff Visualization

### Current approaches

**Text diffs.** Standard unified diff format is well understood by all LLMs. Hawk already supports diff coloring in the TUI.

**Visual diff tools.** GitHub's rich diff view, VS Code's inline/side-by-side diff, and tools like Delta (terminal diff viewer) provide visual context that screenshots can capture.

**Vision model understanding.** Vision models can interpret screenshot diffs from GitHub PRs, VS Code, and other tools. They understand added/removed lines, file changes, and structural modifications.

### How a coding agent should use it

A coding agent should:
1. Generate visual diffs for proposed changes (already partially in hawk's diffsandbox)
2. Accept screenshots of diffs from GitHub/IDE for review context
3. Support `/diff` command that shows changes with syntax highlighting
4. Understand PR screenshot context when users share GitHub screenshots

---

## 16. Code Visualization

### Graphviz

**What it does.** Graph description language and rendering engine for directed/undirected graphs. Used extensively for call graphs, dependency diagrams, class hierarchies, and control flow graphs.

### D3.js -- 113k stars

**What it does.** Low-level JavaScript data visualization library using SVG/Canvas/HTML. Can create any custom visualization including call graphs, dependency trees, flame charts, and treemaps.

### Mermaid for code architecture

**What it does.** Mermaid's class diagrams, flowcharts, and sequence diagrams are widely used for documenting code architecture. Can be generated programmatically from code analysis.

### go/ast + visualization (Go-specific)

**What it does.** Go's built-in AST package can be used to extract function calls, type relationships, and package dependencies, then render them as diagrams.

### Hawk's current state

Hawk has `magicdocs` with Go AST parsing for automatic markdown generation, and `repomap` for incremental code indexing. These could generate visualization input data.

### How a coding agent should use it

A coding agent should:
1. Offer `/architecture` command that generates codebase diagrams
2. Generate Mermaid/D2 from code analysis (call graphs, dependency trees)
3. Render diagrams to SVG/PNG for embedding in documentation
4. Accept existing code visualization screenshots and correlate with source
5. Update diagrams when code structure changes

---

## 17. Automated Screenshot Capture

### Playwright screenshot API

**What it does.** Headless browser screenshot capture with full control: full-page, element, viewport, clip regions. Supports PNG, JPEG with quality settings.

### Puppeteer

**What it does.** Chrome DevTools protocol for automated screenshot capture. Supports device emulation, network throttling, and pixel-perfect rendering.

### Emerging pattern: agent-driven screenshot capture

**Architecture.** The most effective approach for coding agents:
1. Agent makes code changes
2. Agent starts/hot-reloads the dev server
3. Agent takes a screenshot via headless browser
4. Agent sends screenshot to vision model for verification
5. Agent iterates if the result doesn't match expectations

### How a coding agent should use it

A coding agent should:
1. Offer a `/screenshot` command that captures the current UI
2. Integrate with the project's dev server (detect from package.json/go.mod)
3. Auto-capture screenshots before and after UI changes
4. Store screenshots for visual regression tracking
5. Support responsive testing: capture at multiple viewport sizes

---

## 18. Image Generation for Documentation

### Vision model diagram generation

**What it does.** While vision models can't generate pixel images, they can generate:
- Mermaid diagrams
- D2 diagrams
- SVG markup
- ASCII art
- PlantUML

These text-based formats are then rendered to images.

### DALL-E / Flux for mockups

**What it does.** Image generation models can create UI mockups, icons, and illustrations for documentation. screenshot-to-code uses DALL-E 3 or Flux Schnell for generating placeholder images within designs.

### How a coding agent should use it

A coding agent should:
1. Generate Mermaid/D2 diagrams for documentation
2. Render diagrams to SVG/PNG using mermaid-cli or D2
3. Generate ASCII diagrams for terminal-based documentation
4. Not use image generation models for code-related documentation (too imprecise)

---

## 19. SVG/Diagram Generation

### Mermaid generation (primary approach)

**What it does.** LLMs are strong at generating Mermaid syntax from descriptions or code analysis. The structured text format is easy to validate and iterate on.

### D2 generation

**What it does.** D2's syntax is also LLM-friendly. Its server-side rendering makes it particularly suitable for automated pipelines.

### Direct SVG generation

**What it does.** LLMs can generate simple SVG for icons, logos, and simple diagrams. Complex SVGs with many elements are unreliable.

### How a coding agent should use it

A coding agent should:
1. Generate Mermaid by default (widest compatibility, GitHub/GitLab render natively)
2. Support D2 for teams that use it
3. Offer `/mermaid` command that generates diagrams from descriptions
4. Validate generated diagram syntax before writing to files
5. Include diagrams in generated documentation

---

## 20. Accessibility Testing via Screenshot

### axe-core (Deque)

**What it does.** Accessibility testing engine that analyzes DOM structure against WCAG guidelines. Identifies violations, provides fix suggestions, and supports custom rules. Used by Playwright, Cypress, and other test frameworks.

### Vision model a11y analysis

**What it does.** Vision models can analyze screenshots for accessibility issues:
- Insufficient color contrast
- Missing alt text (visible image placeholders)
- Touch target sizes too small
- Text too small to read
- No visible focus indicators
- Layout issues at zoom levels

**How well it works.** Complementary to DOM-based tools. Vision analysis catches issues that DOM analysis misses: visual contrast problems, overlapping elements, misleading visual hierarchy. DOM analysis catches issues vision misses: missing ARIA attributes, tab order, screen reader announcements.

### How a coding agent should use it

A coding agent should:
1. Run axe-core on generated UI code
2. Supplement with vision model analysis of screenshots
3. Report both programmatic and visual accessibility issues
4. Suggest fixes with specific code changes
5. Offer `/a11y` command that audits the current UI

---

## 21. Hawk Current State and Gaps

### What hawk already has

1. **Image reading** (`tool/file_read_media.go`): Reads PNG, JPEG, GIF, WebP images and converts to base64. Handles SVG as text. Resizes oversized images. Validates dimensions and file size.

2. **PDF reading** (`tool/file_read_media.go`): Validates PDF magic bytes, parses page ranges, shells out to pdftotext. Falls back gracefully if pdftotext is unavailable.

3. **Vision capability flag** (`model/router.go`): `Capabilities.Vision` field exists in the model router, tracking which models support vision.

4. **Code review bridge** (`sight/bridge.go`): Integrates with the sight code-review library for AI-powered diff analysis.

5. **Repo mapping** (`repomap/`): Incremental code indexing that could feed visualization.

6. **Magic docs** (`magicdocs/`): Go AST parsing for automatic documentation.

7. **Diff sandbox** (`diffsandbox/`): Virtual file overlay for proposed edits.

### Critical gaps

1. **No multi-modal message support.** The engine passes messages as string content only. There is no `ContentPart`, `ImageBlock`, or structured content block type that would allow sending images alongside text to the LLM.

2. **No vision routing.** While `Capabilities.Vision` exists, the router does not use it. When a user includes an image, the agent does not automatically select a vision-capable model.

3. **No clipboard/paste image support.** Users cannot paste screenshots into the terminal input.

4. **No screenshot capture.** No integration with headless browsers for capturing UI state.

5. **No diagram generation.** No Mermaid/D2 integration for generating or rendering diagrams.

6. **No multi-modal RAG.** The memory/yaad system indexes text only, not images.

7. **PDF extraction is minimal.** Falls back to "pdftotext not available" in most cases. No vision-based PDF understanding.

8. **Image analysis is read-only.** Images are converted to base64 text but not sent as actual image content to the vision API.

---

## 22. Recommended Implementation Plan

### Priority 1: Foundation (required for any multi-modal capability)

**P1a. Multi-modal message protocol.**
Add a content block type system to the engine message format. Messages should support an array of content blocks: `text`, `image` (with base64 data + media type), and `tool_result` (which can contain image blocks). This is the single blocker for all other multi-modal features.

**P1b. Vision-aware model routing.**
When a message contains image content blocks, the router should prefer vision-capable models. If the current model lacks vision, the agent should either auto-switch or warn the user.

**P1c. Image input pipeline.**
Connect `file_read_media.go`'s image reading to the message protocol. When the Read tool encounters an image, it should return an image content block (not just base64 text). This enables "read this screenshot and explain what you see."

### Priority 2: High-value solo developer workflows

**P2a. Screenshot-to-code command (`/mockup` or `/screenshot-to-code`).**
Accept an image path, detect the project's framework, generate code. This is the single most requested multi-modal feature for solo developers who often start from a design screenshot.

**P2b. Error screenshot diagnosis.**
Accept a screenshot of an error (terminal, browser, IDE) and automatically extract + diagnose the issue. Solo developers frequently screenshot errors to share in Slack/Discord -- the same workflow should work with their agent.

**P2c. PDF/document understanding upgrade.**
Replace pdftotext fallback with vision-model PDF reading. Send PDF pages as images to the vision model. This handles scanned documents, complex layouts, and diagrams that pdftotext misses.

### Priority 3: Developer experience improvements

**P3a. Diagram generation (`/diagram`, `/architecture`).**
Generate Mermaid diagrams from code analysis. Leverage existing repomap and magicdocs infrastructure. Render via mermaid-cli if available, otherwise output raw Mermaid for GitHub/GitLab rendering.

**P3b. Visual diff review.**
Enhance the diffsandbox to capture before/after screenshots when changes affect UI files. Send both screenshots to the vision model for verification.

**P3c. Design reference comparison.**
Accept a design image alongside code changes. After generating UI code, capture a screenshot and compare it to the reference design, reporting discrepancies.

### Priority 4: Advanced capabilities

**P4a. Multi-modal RAG.**
Extend yaad to store image embeddings alongside text. Enable retrieval of design mockups, architecture diagrams, and documentation screenshots.

**P4b. Browser integration (`/browse`).**
Integrate a headless browser for capturing UI state, running visual tests, and verifying deployed changes.

**P4c. Accessibility auditing (`/a11y`).**
Combine axe-core DOM analysis with vision model screenshot analysis for comprehensive accessibility testing.

**P4d. Mobile screenshot analysis.**
Platform detection (iOS/Android) from screenshots, with framework-appropriate code generation.

### Architecture recommendations

1. **Use eyrie for multi-modal API calls.** The image content block protocol should be implemented in eyrie so all providers handle it consistently. Claude, GPT-4V, and Gemini all support image inputs but with different API formats.

2. **Image preprocessing pipeline.** Build a shared pipeline for: dimension checking, resize to optimal size for the target model, format conversion (HEIC->JPEG), base64 encoding, and token cost estimation.

3. **Cost awareness.** Image tokens are expensive. A 1920x1080 screenshot costs ~1568 tokens ($0.005 on Sonnet, $0.014 on Opus). The agent should inform users of image costs and downsample aggressively for exploratory queries.

4. **Progressive detail.** Start with low-resolution analysis, then re-analyze at high resolution only if needed. Similar to how Claude's computer use demo recommends XGA resolution for interaction.

5. **Tool composition.** Multi-modal tools should compose: "read this PDF" -> extract images -> analyze diagrams -> generate code. The engine's tool orchestration already supports this pattern.

### What NOT to implement

1. **Image generation models** (DALL-E, Midjourney) for code documentation. Text-based diagram formats (Mermaid, D2) are superior: version-controllable, diffable, and precise.

2. **Custom OCR models.** Vision models are better at code OCR than Tesseract. Don't add a separate OCR dependency.

3. **Full browser automation agent.** This is a different product (browser-use, LaVague). Hawk should support screenshot capture and verification, not general web automation.

4. **Video understanding.** While some models support it, the use cases for coding agents are too narrow to justify the complexity and cost.
