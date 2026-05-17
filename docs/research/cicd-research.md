# CI/CD Automation & DevOps -- State of the Art Research for Hawk

Research compiled 2026-05-03. Sources: GitHub repos, official documentation, and project wikis for 40+ tools and systems.

Focus: What can be **fully automated** so a solo developer never thinks about CI/CD plumbing. Each section covers the tool/technique, its solo-dev value, and what hawk's ecosystem should integrate.

---

## Table of Contents

1. [GitHub Actions Automation](#1-github-actions-automation)
2. [AI-Powered CI](#2-ai-powered-ci)
3. [Deployment Automation](#3-deployment-automation)
4. [Infrastructure as Code](#4-infrastructure-as-code)
5. [Release Automation](#5-release-automation)
6. [Dependency Management](#6-dependency-management)
7. [Automated Testing in CI](#7-automated-testing-in-ci)
8. [Performance Regression Detection](#8-performance-regression-detection)
9. [Security Scanning in CI](#9-security-scanning-in-ci)
10. [Docker Optimization](#10-docker-optimization)
11. [Monorepo Tools](#11-monorepo-tools)
12. [Preview Environments](#12-preview-environments)
13. [Canary and Progressive Deployments](#13-canary-and-progressive-deployments)
14. [Automated Rollback Strategies](#14-automated-rollback-strategies)
15. [Database Migration Safety](#15-database-migration-safety)
16. [Feature Flags Integration](#16-feature-flags-integration)
17. [Automated Changelog Generation](#17-automated-changelog-generation)
18. [Binary Distribution](#18-binary-distribution)
19. [Self-Hosted Runners Optimization](#19-self-hosted-runners-optimization)
20. [Cost Optimization for CI/CD](#20-cost-optimization-for-cicd)
21. [Hawk Integration Priorities](#21-hawk-integration-priorities)

---

## 1. GitHub Actions Automation

### act (local runner)
- **What**: CLI tool (Go, 70.1k stars) that runs GitHub Actions workflows locally using Docker containers. Reads `.github/workflows/`, determines dependencies, pulls/builds container images, executes actions with matching env vars and filesystem layout. v0.2.88 as of May 2026.
- **How it works**: Replicates GitHub's hosted runner environment in local Docker containers. Supports VS Code extension ("GitHub Local Actions") for IDE integration.
- **Solo dev value**: Test workflow changes instantly without push-wait-debug cycles. Catches YAML errors, missing secrets, and broken steps before they consume CI minutes. Eliminates the "commit, push, wait 3 minutes, see failure, repeat" loop.
- **Limitations**: Not 100% fidelity with GitHub-hosted runners (some actions rely on GitHub-specific networking, OIDC tokens, or larger runner features). Service containers and some matrix strategies can diverge.

### actionlint
- **What**: Static analysis tool (3.8k stars) for GitHub Actions workflow files. v1.7.12. Multi-layered analysis: syntax validation against official spec, type checking for `${{ }}` expressions, action metadata verification, embedded script analysis via shellcheck/pyflakes, security scanning for script injection.
- **Errors caught**: Invalid YAML keys, typos, undefined matrix variables, incorrect action input names, type mismatches, untrusted variable usage in scripts, hard-coded credentials, invalid runner labels.
- **Solo dev value**: Catches workflow bugs statically before any push. Can run as a pre-commit hook. Validates reusable workflow inputs/outputs/secrets. The script injection detection is particularly valuable -- solo devs often don't think about Actions security.

### Workflow optimization techniques
- **Dependency caching**: `actions/cache` with `hashFiles('**/go.sum')` keys. GitHub provides 10GB per repo, 7-day TTL on inactive caches. Use `setup-go`/`setup-node` built-in caching for simplicity.
- **Matrix strategy**: Parallel builds across OS/arch combinations (hawk already does this for linux/darwin/windows x amd64/arm64).
- **Job dependencies**: Run lint in parallel with test, build only after test passes (hawk's current CI does this correctly).
- **Concurrency groups**: Cancel in-progress runs when new commits push -- saves CI minutes on rapid iteration.
- **Path filters**: Only run relevant jobs when specific paths change (skip Go tests when only docs change).
- **Reusable workflows**: Extract common patterns into `.github/workflows/reusable-*.yml` for DRY CI configuration.

### Hawk integration opportunity
A `hawk ci init` or `/ci` skill that:
1. Scans the project (language, dependencies, test framework, deployment target)
2. Generates an optimized `.github/workflows/ci.yml` with caching, matrix builds, path filters, concurrency groups
3. Generates an `actionlint` pre-commit hook
4. Optionally creates an `act` configuration for local testing
5. Validates existing workflows and suggests improvements (missing caching, redundant steps, security gaps)

---

## 2. AI-Powered CI

### Trunk.io
- **What**: CI reliability platform solving flaky tests, slow builds, and broken main branches. Used by Brex, Faire, Gusto, Zillow, Google.
- **Flaky test detection**: Automatically detects, quarantines, and eliminates flaky tests across any language, test runner, or CI provider. Provides AI-powered failure analysis identifying duplicate failures and summarizing root causes. Test status history tracking.
- **Merge queue**: Anti-flake protection keeps failed PRs in queue while downstream PRs test. Intelligent batching handles up to 100 PRs per run with automatic bisection on failure. Parallel queues for non-overlapping changes.
- **Solo dev value**: Flaky test detection is the killer feature. Solo devs lack the team bandwidth to manually triage intermittent failures. The AI failure summary means you get "this test is flaky because of timezone-dependent date comparison" instead of digging through logs.

### Codecov
- **What**: Code coverage platform that uploads coverage reports from CI and tracks coverage trends. GitHub Action supports multi-format uploads, matrix testing, OIDC auth.
- **Key features**: Coverage diff on PRs (which lines of the diff are untested), historical tracking, badge generation, configurable coverage thresholds.
- **Solo dev value**: Coverage visibility without configuration overhead. The PR comment showing "this change reduces coverage by 2.3%" is a useful guardrail.

### Codacy
- **What**: Automated code review platform. Detects static analysis issues across 40+ languages. Categories: code style, security, error proneness, performance, unused code. AI-powered issue explanation and automated fix suggestions. Free for open source.
- **Solo dev value**: Acts as a second pair of eyes. When you don't have a team for code review, automated quality checks catch things you'd miss. The AI fix suggestions save research time.

### Hawk integration opportunity
A `hawk ci analyze` or `/ci-health` command that:
1. Parses recent CI runs (via `gh run list` and `gh run view`)
2. Identifies flaky tests by looking at intermittent failures across runs
3. Surfaces the slowest CI steps and suggests caching/parallelism improvements
4. Generates a coverage trend summary from uploaded artifacts
5. Could integrate Trunk-style flaky test quarantine directly into test configuration files

---

## 3. Deployment Automation

### Kamal (formerly MRSK)
- **What**: Basecamp's deployment tool (14.2k stars, Ruby). v2.11.0. Deploys containerized web apps to bare metal or cloud VMs via SSH. Uses kamal-proxy for zero-downtime container switching. Framework agnostic (built for Rails, works with anything Docker-compatible).
- **How it works**: SSHKit-based command execution across multiple servers. Pulls Docker images, runs health checks, switches proxy to new containers, cleans up old ones. Configuration via `deploy.yml`.
- **Solo dev value**: Deploy to a $5/month Hetzner VPS with the same zero-downtime guarantees that big companies get. No Kubernetes learning curve. `kamal deploy` is the entire deployment workflow.

### Coolify
- **What**: Open-source, self-hosted PaaS alternative to Heroku/Netlify/Vercel (54.5k stars). v4.0.0 (April 2026). PHP/Blade. Manages servers, applications, and databases via SSH. 280+ one-click services. Supports VPS, bare metal, Raspberry Pi.
- **Solo dev value**: Self-hosted Heroku for $5/month on a VPS. No vendor lock-in. Configurations stored on your servers. One-click database provisioning. Ideal for solo devs who want PaaS convenience without PaaS pricing.

### CapRover
- **What**: Easy PaaS (13k+ stars) using Docker Swarm + nginx + Let's Encrypt. CLI and web GUI. One-click apps (MariaDB, MySQL, MongoDB, PostgreSQL). No Docker/nginx expertise required. No vendor lock-in.
- **Comparison to Heroku**: "Heroku charges 250 USD/month for their 2GB instance, the same server is $5 on Hetzner."
- **Solo dev value**: Lowest barrier to self-hosted deployment. Web dashboard for monitoring. Automatic SSL. But Docker Swarm is being deprecated in favor of Kubernetes, which is a long-term concern.

### Railway
- **What**: Cloud deployment platform (2M+ developers). Auto-config detection, instant preview environments per PR, 100Gbps internal networking, geographic distribution, centralized monitoring.
- **Solo dev value**: "Took about 2 minutes to set up my server with a postgres using postgis." Hard spending limits prevent bill shock. Preview environments per PR without configuration. The managed experience means zero ops overhead.

### Hawk integration opportunity
A `/deploy` skill that:
1. Detects the project type and suggests deployment target (Kamal for VPS, Railway/Coolify for managed, Vercel for static/Next.js)
2. Generates deployment configuration (Kamal deploy.yml, Railway project config, Dockerfile if missing)
3. Sets up GitHub Actions deployment workflow with environment protection rules
4. Configures health checks and zero-downtime switching
5. For Kamal users: generates the SSH setup, Docker registry config, and proxy configuration

---

## 4. Infrastructure as Code

### Terraform
- **What**: The standard IaC tool (43k+ stars, Go). v1.15.1. Declarative HCL configuration. Execution plans (preview changes before apply), resource dependency graph (parallel provisioning), change automation.
- **State management**: Remote state in S3, GCS, Terraform Cloud, etc. State locking prevents concurrent modifications.
- **Provider ecosystem**: Thousands of providers for every major cloud and SaaS service.
- **License note**: BSL 1.1 since August 2023 (not fully open-source). OpenTofu (CNCF fork) provides the Apache 2.0 alternative.
- **Solo dev value**: Infrastructure becomes reviewable, reproducible code. But HCL has a learning curve, and state management adds operational overhead for simple deployments.

### Pulumi
- **What**: IaC using real programming languages (Go, TypeScript, Python, Java, C#, YAML). Supports 120+ cloud providers. v3.x. Diff-based deployments, secrets management via Pulumi ESC, automation API for embedding IaC in applications.
- **Key difference from Terraform**: Write infrastructure in Go/TypeScript/Python instead of learning HCL. Use loops, functions, classes, package management you already know.
- **Solo dev value**: If you're a Go developer (like hawk's users), you write infrastructure in Go. No context-switching to a DSL. The automation API means hawk could programmatically generate and apply infrastructure.

### SST (Serverless Stack)
- **What**: Framework for full-stack apps on your own infrastructure (25.9k stars). v4.12.11. TypeScript/Go. Live development environment, component linking system, console for monitoring. Supports Next.js, Remix, Astro.
- **Solo dev value**: Deploys full-stack apps (frontend + API + database) as a single unit. The live development mode means changes are reflected in your deployed environment in real-time without redeploy cycles.

### Hawk integration opportunity
A `/infra` skill that:
1. Generates Pulumi programs in Go (matching hawk's language) for common patterns (VPS + database, serverless API, static site + CDN)
2. For Terraform users: validates `.tf` files, suggests best practices, detects drift
3. Manages state configuration (S3 backend setup, locking)
4. Generates the minimal infrastructure needed -- solo devs don't need multi-AZ Kubernetes clusters
5. Could maintain an "infrastructure manifest" in the repo that hawk understands and can modify

---

## 5. Release Automation

### semantic-release
- **What**: Fully automated version management and release. Determines next version from commit messages (Angular convention: `fix:` = patch, `feat:` = minor, `BREAKING CHANGE:` = major). Generates release notes, publishes packages, creates git tags. 9-step process: verify conditions, get last release, analyze commits, verify release, generate notes, prepare, publish, add channel, succeed/fail hooks.
- **Plugin ecosystem**: Extensive -- npm publish, GitHub releases, changelogs, Slack notifications, Docker image tagging.
- **Solo dev value**: Tag a release by merging to main. Everything else is automated. No manual version bumping, no forgetting to update CHANGELOG, no mismatched git tags.

### release-please (Google)
- **What**: Google's release automation. Creates "Release PRs" that accumulate conventional commits, auto-update CHANGELOG.md and version files. Merging the Release PR triggers the actual release. Supports monorepos with per-package release tracking.
- **Key difference from semantic-release**: Release-please uses a PR-based workflow (you review the release before it happens). semantic-release is fully autonomous.
- **Solo dev value**: The PR-based approach gives you a chance to review what's about to release. Good for projects where you want a human checkpoint.

### changesets
- **What**: Version management for monorepos (8k+ stars). Developers declare "changesets" (release intents) specifying packages and bump types. Flattens multiple changes into single releases per package. Auto-updates changelogs, handles internal dependencies, orchestrates multi-package publishing.
- **Used by**: Chakra UI, Astro, Biome, SvelteKit.
- **Solo dev value**: If you maintain multiple packages (e.g., hawk core + hawk-skills + hawk-plugins), changesets coordinates their releases. Overkill for single-package projects.

### Hawk integration opportunity
Hawk already uses goreleaser (`.goreleaser.yml` with cross-compilation, Homebrew tap, changelog filtering). The natural extension:
1. A `/release` skill that validates conventional commit messages, previews the next version number, generates release notes draft
2. Integration with release-please for the PR-based workflow (create a "Release PR" that hawk helps write)
3. Automated CHANGELOG.md updates using the existing changelog skill, triggered by the release workflow
4. Validation that the goreleaser config matches the current project structure (new binaries, new architectures)

---

## 6. Dependency Management

### Renovate
- **What**: Cross-platform dependency automation (21.4k stars, 5000+ releases). 90+ package managers supported (npm, Python, Java, .NET, Go, Docker, Terraform, Kubernetes manifests). Multi-platform: GitHub, GitLab, Bitbucket, Azure DevOps.
- **Key features**: Automatic PR generation with changelogs and release notes, merge confidence scores (age, adoption rate, pass rates), grouping related updates, scheduling windows, auto-merge for trusted updates.
- **vs Dependabot**: Renovate supports far more ecosystems and platforms. More configurable (regex managers for custom file formats). Can group updates. Supports monorepos natively. Renovate's merge confidence feature (showing how many other repos successfully upgraded) is unique.
- **Solo dev value**: Set-and-forget dependency updates. Configure auto-merge for patch updates of trusted packages. Group minor updates into weekly PRs. The merge confidence data means you can auto-merge updates that 95%+ of other repos have safely adopted.

### Dependabot
- **What**: GitHub's built-in dependency update tool. Creates PRs with latest versions. Smart version resolution. Rich PR context (changelogs, release notes, commit history). Security updates (minimum non-vulnerable version). 25+ package managers.
- **Solo dev value**: Zero-configuration if you use GitHub. Just add `.github/dependabot.yml`. Tight integration with GitHub's security advisories means you get security PRs automatically.

### Socket.dev
- **What**: Supply chain security platform. Analyzes package behavior rather than just known CVEs -- detects install scripts, network access, filesystem access, obfuscated code, typosquatting. Proactively identifies suspicious package behavior before CVEs are published.
- **Solo dev value**: Dependabot/Renovate tell you about known vulnerabilities. Socket tells you "this package runs a post-install script that sends data to an unknown server" -- catching supply chain attacks before they're publicly disclosed. Critical for solo devs who can't manually audit every dependency.

### Hawk integration opportunity
1. Generate optimal `renovate.json` or `.github/dependabot.yml` based on project analysis (auto-detect package managers, set appropriate grouping and scheduling)
2. A `/deps audit` command that runs `govulncheck` (Go), cross-references with Socket-style behavioral analysis, and produces a prioritized remediation plan
3. Auto-merge configuration for safe updates (patch bumps of well-adopted packages)
4. Dependency update impact analysis: "upgrading X from v1 to v2 will require changing these 3 call sites"

---

## 7. Automated Testing in CI

### Test impact analysis (TIA)
- **What**: Technique that maps tests to the code they exercise, then only runs tests affected by the current change. Implementations: Launchable (ML-based test selection), Microsoft's TIA in Azure DevOps, Nx's affected commands, Turborepo's task hashing.
- **How it works**: Static analysis of import graphs + runtime coverage data. When a file changes, determine which test files transitively depend on it. Run only those tests.
- **Solo dev value**: As test suites grow, running all tests on every push becomes slow. TIA keeps CI fast without sacrificing correctness. For a Go project like hawk, this means running `go test ./analytics/...` when only `analytics/` changed, not `go test ./...`.

### Test selection strategies
- **File-based**: Map source files to test files via naming convention or import analysis.
- **Coverage-based**: Use code coverage data from previous runs to identify which tests cover which source lines. Requires coverage collection infrastructure.
- **ML-based (Launchable)**: Uses historical test data to predict which tests are most likely to fail for a given change. Prioritizes those tests first.
- **Graph-based (Nx/Turborepo)**: Build a dependency graph of packages/modules. Run tests in packages affected by the change.

### Test reporting in CI
- **test-reporter**: GitHub Action that parses XML/JSON test results from 15+ frameworks and creates GitHub Check Runs with code annotations at failure points. Supports .NET, Go, Java, JavaScript, Python, Ruby, Swift.
- **Solo dev value**: Failed tests annotated directly on the PR diff, showing exactly which test failed and where. No log digging.

### Hawk integration opportunity
1. A `/test-impact` command that analyzes the current git diff and determines which Go packages are affected, then generates the minimal `go test` command
2. Pre-push hook that runs only affected tests locally before pushing
3. CI workflow generation that uses path filters to run only relevant test suites
4. Integration with test-reporter for PR annotations
5. Historical flaky test tracking: store test results per commit, surface intermittent failures

---

## 8. Performance Regression Detection

### Techniques
- **Benchmark tracking**: Run `go test -bench` in CI, store results, compare against baseline. Tools: `benchstat` (Go), `hyperfine` (CLI), `criterion` (Rust).
- **Relative comparison**: Compare benchmarks from the PR branch against main. Flag regressions above a threshold (e.g., >5% slower).
- **Statistical significance**: Use `benchstat` to determine if differences are statistically significant (not just noise). Requires multiple benchmark iterations.
- **Binary size tracking**: Track compiled binary size per commit. Regressions indicate unnecessary dependencies or dead code.
- **Startup time tracking**: For CLI tools like hawk, track time-to-first-output. Regressions in startup time directly impact UX.

### GitHub Actions patterns
```yaml
- name: Run benchmarks
  run: go test -bench=. -benchmem -count=5 ./... | tee bench.txt
- name: Compare benchmarks
  uses: bencherdev/bencher@main  # or custom benchstat comparison
```

### Solo dev value
Performance regressions creep in unnoticed without automated detection. A solo dev doesn't have the bandwidth to manually benchmark every change. Automated regression detection catches "this commit made startup 40% slower because it added an import that initializes eagerly."

### Hawk integration opportunity
1. A `/bench` command that runs Go benchmarks, stores results in `.hawk/benchmarks/`, and compares against the last stored baseline
2. CI workflow step that posts benchmark comparison as a PR comment
3. Binary size tracking in CI (hawk already tracks this in TODO item 205)
4. Startup time regression detection: `time hawk --version` in CI with threshold alerting
5. Memory allocation tracking via `-benchmem` with regression thresholds

---

## 9. Security Scanning in CI

### Trivy (Aqua Security)
- **What**: Comprehensive security scanner (24k+ stars, Go). v0.70.0. Scans container images, filesystems, git repos, VM images, Kubernetes clusters. Detects OS vulnerabilities, software dependency CVEs, IaC misconfigurations, secrets, and license issues. Generates SBOMs.
- **Integration**: Direct GitHub Actions integration. Supports multiple output formats (table, JSON, SARIF for GitHub security tab).
- **Solo dev value**: One scanner for everything -- containers, dependencies, IaC, secrets. The SARIF output integrates with GitHub's security tab for a unified view.

### Grype (Anchore)
- **What**: Vulnerability scanner for container images and filesystems (9k+ stars, Go). Broad package support (Alpine, Debian, Ubuntu, RHEL + Python, Java, JavaScript, Go, Ruby, .NET, PHP, Rust). Risk prioritization using EPSS scores and KEV data. OpenVEX support for filtering false positives. Works with Syft-generated SBOMs.
- **Solo dev value**: EPSS scores answer "how likely is this vulnerability to be exploited in the wild?" -- critical for solo devs who can't fix every CVE and need to prioritize.

### Snyk
- **What**: Cloud-native security scanning. CLI scans open-source dependencies, application code, container images, and IaC (Terraform, Kubernetes). `snyk test` for dependencies, `snyk code test` for source code, `snyk container test` for images. Continuous monitoring via `snyk monitor`. Multi-language: JavaScript, Python, Java, Go, Ruby, PHP, C#.
- **Solo dev value**: The `snyk monitor` feature creates a persistent dependency snapshot that alerts you when new vulnerabilities are discovered, even between deploys. The code scanning catches vulnerabilities in your own code, not just dependencies.

### govulncheck (Go-specific)
- **What**: Go's official vulnerability scanner. Checks your code's actual call graph against the Go vulnerability database. Only reports vulnerabilities in functions your code actually calls (not just "this package has a CVE somewhere").
- **Solo dev value**: Far fewer false positives than generic scanners. If the vulnerable function isn't in your call path, it doesn't report it. Hawk's CI already runs `govulncheck ./...`.

### Hawk integration opportunity
1. The existing `security-scan` skill should be extended to orchestrate Trivy/Grype/govulncheck and synthesize results
2. A `/security` command that runs all applicable scanners and produces a prioritized remediation plan sorted by EPSS score
3. CI workflow generation that adds Trivy container scanning to the Docker build step
4. Pre-commit hook that runs `govulncheck` on changed packages
5. SARIF output integration for GitHub security tab visibility
6. Secret scanning as a pre-commit hook (detect API keys before they're committed)

---

## 10. Docker Optimization

### Multi-stage builds
- **What**: Multiple `FROM` statements in a single Dockerfile. Each stage can use a different base. Selectively copy artifacts between stages, discarding build tools from the final image.
- **Key technique**: Build stage has the SDK/compiler, final stage has only the runtime + binary. For Go: build in `golang:1.26-alpine`, copy binary to `alpine:latest` or `scratch`.
- **BuildKit optimization**: Only builds stages that the target depends on (skips unused stages). Better cache efficiency than legacy builder.
- **Named stages**: Use `AS build` for maintainability: `COPY --from=build` survives Dockerfile reordering.

### Layer caching strategies
- **Dependency-first ordering**: Copy dependency files (go.mod, package.json) and install before copying source code. Dependencies change less frequently, so this layer stays cached.
- **Build cache mounts**: `RUN --mount=type=cache,target=/go/pkg/mod go build` keeps the Go module cache between builds.
- **BuildKit inline cache**: `--build-arg BUILDKIT_INLINE_CACHE=1` embeds cache metadata in the image for registry-based caching.

### Slim (formerly DockerSlim)
- **What**: CNCF Sandbox project that analyzes and optimizes container images without Dockerfile changes. Dynamic analysis of runtime requirements. Removes unnecessary components.
- **Size reductions**: Go apps: up to 448x smaller (700MB to 1.56MB). Python: 33x. Ruby: 32x. Node.js: 30x.
- **Additional features**: `xray` (reveal contents, reverse-engineer Dockerfile), `lint` (Dockerfile best practices), auto-generates Seccomp and AppArmor security profiles.
- **Solo dev value**: Run `slim build myapp:latest` and get a dramatically smaller, more secure image with zero Dockerfile changes.

### ko (Go-specific)
- **What**: Container image builder for Go apps (8k+ stars). Runs `go build` locally, packages binary into a container. No Docker required. Multi-platform builds, automatic SBOMs, Kubernetes YAML templating.
- **Solo dev value**: For Go projects like hawk, `ko build ./` is simpler and faster than writing a Dockerfile. Produces minimal images (just the Go binary + base). Perfect for CI where you want to build and push images without Docker-in-Docker.

### Hawk integration opportunity
The existing `docker-deploy` skill is a good foundation. Extensions:
1. Dockerfile analysis and optimization suggestions (detect missing layer caching, unnecessary packages in final stage, non-root user)
2. `ko` integration for Go projects (generate ko config, explain when ko is better than Dockerfile)
3. Image size tracking in CI (alert when image grows beyond threshold)
4. Slim integration: suggest `slim build` for production images
5. Multi-arch build configuration (buildx with QEMU for arm64 + amd64)
6. Security: detect secrets in Docker build context, validate .dockerignore

---

## 11. Monorepo Tools

### Turborepo (Vercel)
- **What**: High-performance build system for JS/TS monorepos (30.3k stars). Written in Rust. v2.9.7. Intelligent task caching, task orchestration based on dependency graph, remote caching via Vercel.
- **Key features**: Content-aware hashing (only rebuilds what changed), parallel task execution, pipeline definition in `turbo.json`, incremental builds.
- **Solo dev value**: If you maintain a monorepo with multiple packages (e.g., frontend + backend + shared lib), Turborepo dramatically speeds up builds. Remote caching means your CI and local dev share the same cache.

### Nx (Nrwl)
- **What**: Monorepo platform (25k+ stars). "The Monorepo Platform that amplifies both developers and AI agents." Intelligent caching, distributed task execution, project graph visualization, conformance rules, code ownership definitions.
- **AI features (2026)**: Self-healing AI agents that fix broken PRs and flaky tests. Repo-aware dependency management with AI assistance.
- **Performance claims**: Payfit: "360x faster deployments from 5 days to 20 minutes." Hetzner Cloud: "60x faster testing from 20 minutes to seconds."
- **Solo dev value**: The affected command (`nx affected --target=test`) only runs tests in packages affected by the current change. The computation cache means you never rebuild the same thing twice.

### Bazel (Google)
- **What**: Build and test system for large-scale projects (24k+ stars). "Fast, Correct -- Choose two." Advanced local and distributed caching, dependency analysis, parallel execution. Supports Java, C++, Go, Android, iOS. Extensible rule system.
- **Solo dev value**: Overkill for most solo dev projects. Bazel's power shows at massive scale (Google-scale monorepos). The learning curve and configuration overhead don't justify the benefits for small-to-medium projects.
- **When to use**: Multi-language monorepo with 100+ packages, need hermetic builds, have existing Bazel ecosystem.

### Hawk integration opportunity
For hawk's own build system and for projects hawk helps:
1. Detect monorepo structure and suggest appropriate tool (Turborepo for JS/TS, Nx for full-stack, skip Bazel for solo devs)
2. Generate `turbo.json` or `nx.json` configuration based on package structure analysis
3. CI workflow generation with affected-only test runs
4. For Go specifically: leverage Go's built-in workspace support (`go.work`) which hawk already uses

---

## 12. Preview Environments

### Vercel Previews
- **What**: Automatic preview deployments on every PR push. Three default environments: Local, Preview, Production. Each deployment gets a unique URL (branch-specific and commit-specific). Custom environments on Pro/Enterprise (staging, QA). Environment-specific variables and secrets.
- **How it works**: Push to non-production branch or create PR -> automatic preview deployment -> URL posted as PR comment. `vercel deploy` for CLI, `vercel --prod` for production.
- **Solo dev value**: See your changes live before merging. Share preview URLs with beta testers or clients. No infrastructure to manage.

### Railway
- **What**: "Every pull request gets its own preview. No surprises after merge." Auto-config detection, 100Gbps internal networking, centralized monitoring.
- **Solo dev value**: Full-stack preview environments (app + database) per PR. Hard spending limits prevent surprises. 2-minute setup for server + database.

### Netlify
- **What**: Similar to Vercel for static/JAMstack sites. Deploy Previews on every PR. Branch deploys for staging environments. Split testing (A/B) at the CDN level.
- **Solo dev value**: Free tier is generous for static sites. PR preview deploys are automatic with zero configuration.

### Self-hosted alternatives
- **Coolify**: Self-hosted previews on your own infrastructure. Configure preview environments per branch.
- **Kubernetes-based**: Namespace-per-PR using tools like `vcluster` or `Argo CD ApplicationSets`. Overkill for solo devs.

### Hawk integration opportunity
1. Detect project type and recommend preview environment solution (Vercel for Next.js/static, Railway for full-stack, Coolify for self-hosted)
2. Generate the deployment configuration for automatic PR previews
3. Add preview URL to PR descriptions automatically via GitHub Actions
4. For self-hosted: generate a lightweight Docker Compose-based preview system that spins up per branch

---

## 13. Canary and Progressive Deployments

### Concepts
- **Canary deployment**: Route a small percentage of traffic (1-5%) to the new version. Monitor error rates and latency. If healthy, gradually increase to 100%. If unhealthy, route all traffic back to the old version.
- **Blue/green deployment**: Run two identical environments. Switch traffic from blue (current) to green (new) atomically. Roll back by switching back.
- **Progressive delivery**: Combine feature flags with deployment strategies. Deploy code to all servers but only enable features for a subset of users.

### Solo dev tools
- **Kamal**: Built-in zero-downtime deploys via kamal-proxy. Blue/green by default (new container starts, health check passes, proxy switches).
- **Cloudflare Workers**: Gradual rollout percentages built into the platform. No infrastructure management.
- **Feature flags (see section 16)**: Decouple deployment from release. Deploy code everywhere, enable features progressively.
- **Railway/Vercel**: Canary deployments via preview environments + DNS-level traffic splitting.

### Solo dev value
Solo devs rarely need sophisticated canary deployments because they typically don't have the traffic to make statistical analysis meaningful. The pragmatic approach: deploy with feature flags, enable for yourself first, then enable for everyone. Kamal's built-in zero-downtime switching covers 95% of solo dev needs.

### Hawk integration opportunity
1. For projects using Kamal: validate deployment configuration, generate health check endpoints
2. Feature flag integration (see section 16) for progressive rollouts
3. Simple rollout script: deploy -> verify health endpoint -> if healthy, complete; if unhealthy, roll back
4. Monitoring integration: check error rates after deployment via simple HTTP health checks

---

## 14. Automated Rollback Strategies

### Techniques
- **Health check-based**: After deployment, hit a health endpoint. If it returns non-200 within N seconds, trigger rollback.
- **Error rate-based**: Monitor error rate (via logs or metrics) after deployment. If error rate exceeds baseline by >X%, trigger rollback.
- **Deployment versioning**: Keep the previous version's artifacts (Docker image, binary) available for instant rollback.
- **Git-based rollback**: `git revert` the deployment commit, trigger a new deployment of the previous state.
- **Database considerations**: Rollback is only safe if database migrations are backward-compatible (see section 15).

### Tool support
- **Kamal**: `kamal rollback <version>` reverts to a previous container image. Built-in.
- **Kubernetes**: Deployment rollback is native (`kubectl rollout undo`).
- **GitHub Actions**: Re-run a previous successful deployment workflow.
- **Coolify**: One-click rollback in the web UI.

### Solo dev value
Without automated rollback, a solo dev's deploy-then-sleep pattern is risky. The minimum viable rollback: deploy behind a health check, auto-revert if the health check fails within 60 seconds. This catches the "deployed broken code at midnight" scenario.

### Hawk integration opportunity
1. Generate health check endpoints for web applications (Go: `/healthz` handler, Node: Express middleware)
2. Deployment workflow with built-in rollback: deploy -> health check -> if fail, `kamal rollback` or redeploy previous image
3. Pre-deployment checklist: are migrations backward-compatible? Are environment variables set? Is the health endpoint responding?
4. Post-deployment monitoring: check error logs for N minutes after deploy, alert if anomalies detected

---

## 15. Database Migration Safety

### gh-ost (GitHub)
- **What**: Online schema migration for MySQL. Triggerless approach -- uses binary log stream instead of triggers. Creates ghost table, copies data incrementally, applies ongoing changes from binlog.
- **Key features**: True pause (stops writes when throttled), dynamic reconfiguration during migration, testability (run on replica first), postponable cut-over, external hooks.
- **vs pt-online-schema-change**: gh-ost eliminates triggers (source of many limitations and risks). Better control over the migration process, including genuine suspension.
- **Solo dev value**: If you have a MySQL database with live traffic, schema changes without downtime. Run on replica first to build confidence.

### pgroll (xata.io)
- **What**: PostgreSQL zero-downtime migrations (Go). Expand/contract workflow: creates virtual schemas using views, performs additive changes, synchronizes writes between old and new columns via triggers. Instant rollback. PostgreSQL 14.0+, any service (RDS, Aurora, etc.).
- **How it works**: Expand phase creates new columns/tables alongside old ones with bidirectional triggers. Contract phase removes old schema when no clients use it. Views provide version isolation.
- **Solo dev value**: Zero-downtime Postgres schema changes with instant rollback. Single Go binary, no external dependencies. If a migration goes wrong, roll back immediately without data loss.

### Atlas (Ariga)
- **What**: Language-agnostic database schema management (6k+ stars, Go). Two workflows: declarative (Terraform-like: define desired state, Atlas plans the migration) and versioned (define desired schema, Atlas generates migration files).
- **Key features**: 50+ safety analyzers (destructive changes, data-dependent modifications, table locks, backward-incompatible alterations), schema testing, security-as-code (roles, permissions, RLS), 16 ORM integrations, 14+ databases (PostgreSQL, MySQL, SQLite, ClickHouse, Snowflake, etc.).
- **CI/CD integration**: GitHub Actions, GitLab CI, Kubernetes, Terraform.
- **Solo dev value**: The declarative workflow is transformative -- describe what your schema should look like, Atlas figures out how to get there. The 50+ safety analyzers catch "this ALTER TABLE will lock the table for 30 minutes" before you run it in production.

### Hawk integration opportunity
This is a high-value integration area:
1. A `/migrate` skill that generates Atlas configuration for the project's database
2. Migration safety analysis: review pending migrations for destructive changes, table locks, data-dependent modifications
3. Generate backward-compatible migration patterns (expand/contract for column renames, add-then-backfill-then-drop for column type changes)
4. CI workflow step that runs Atlas `migrate lint` on migration files in PRs
5. Pre-deployment checklist: are all pending migrations backward-compatible with the previous code version? (Critical for rollback safety)

---

## 16. Feature Flags Integration

### Flipt (GitOps-native)
- **What**: Open-source feature management (4k+ stars, Go). Git-native: flags stored in Git repositories alongside code. Single binary, no external database. Real-time updates via SSE. GPG commit signing, multiple auth methods.
- **Key difference**: Flags deploy alongside code via existing CI/CD. No separate feature flag infrastructure. Flags are version-controlled with full Git history.
- **Solo dev value**: The GitOps approach means no external service to manage. Flags are just files in your repo. `git blame` on a flag config tells you who changed it and when.

### Unleash
- **What**: Open-source feature flag platform (12k+ stars). Activation strategies for targeted releases. 12+ official SDKs. Canary releases, A/B testing, kill switches, multi-environment support. Self-hostable via Docker.
- **Solo dev value**: More feature-rich than Flipt but requires running a server. The kill switch feature ("disable this feature instantly in production") is valuable for solo devs who deploy and then discover issues.

### OpenFeature/flagd
- **What**: Vendor-neutral standard for feature flag evaluation (CNCF). flagd is the reference implementation. Supports multiple data sources, real-time updates, OpenTelemetry integration.
- **Solo dev value**: If you start with flagd and later want to switch to LaunchDarkly or Flipt, the OpenFeature SDK abstraction means zero code changes. Good insurance against vendor lock-in.

### LaunchDarkly
- **What**: Enterprise feature management (commercial). Trillions of flags/day. Phased releases, attribute-based targeting, maintenance windows. SDKs for Node.js, browser, React Native, edge platforms.
- **Solo dev value**: Expensive for solo devs. The free tier is limited. Better suited for teams. Solo devs should start with Flipt or Unleash.

### Hawk integration opportunity
1. A `/flags` skill that sets up Flipt (GitOps approach, matching hawk's Git-centric workflow) with initial configuration
2. Feature flag code generation: generate the SDK initialization and flag check boilerplate for the project's language
3. CI integration: validate flag configurations in PRs, detect unreferenced flags (technical debt)
4. Progressive rollout recipes: "deploy this feature to 10% of users" with the appropriate Flipt/Unleash configuration
5. Flag cleanup: detect feature flags that have been 100% enabled for >30 days and suggest removal

---

## 17. Automated Changelog Generation

### Approaches
- **Conventional commits**: Parse `feat:`, `fix:`, `BREAKING CHANGE:` prefixes from git log. Tools: semantic-release, release-please, goreleaser's built-in changelog.
- **PR-based**: Generate changelog from merged PR titles and labels. Tools: GitHub's auto-generated release notes, `git-cliff`.
- **Manual with automation**: Developers write changeset files (changesets), automation compiles them into a CHANGELOG.

### goreleaser (hawk's current approach)
Hawk's `.goreleaser.yml` already has changelog generation with commit filtering (excludes `docs:`, `test:`, `chore:`). This is functional but basic -- it passes through raw commit messages rather than rewriting them for users.

### git-cliff
- **What**: Highly configurable changelog generator (Go/Rust). Custom templates, regex-based commit parsing, conventional commits support, monorepo support, remote integration (fetch GitHub PR titles/labels). Can generate changelogs for any commit range.
- **Solo dev value**: More flexibility than goreleaser's built-in changelog. Template system means you can match any changelog format.

### Hawk's existing changelog skill
The `hawk-skills/changelog/SKILL.md` already defines a workflow: identify commit range, categorize by conventional commit prefix, rewrite technical messages into user-friendly language, format as markdown. This is the right approach -- the AI-powered rewriting transforms "fix: handle nil ptr in session.Resume when WAL corrupt" into "Fixed crash when resuming sessions with corrupted data."

### Hawk integration opportunity
1. Enhance the changelog skill to use the AI rewriting approach as a CI step (generate changelog draft as part of release workflow)
2. Validate conventional commit format in a pre-push hook (reject "fix stuff" commits)
3. Generate both a user-facing CHANGELOG.md and a detailed developer-facing release note
4. Support different audiences: "what changed for users" vs "what changed for contributors"

---

## 18. Binary Distribution

### GoReleaser
- **What**: Release automation for Go (15.8k stars). v2.15.4. Cross-platform compilation, multiple packaging formats, CI integration. Also supports Rust, Zig, TypeScript, Python. 593 releases.
- **Hawk's current usage**: Cross-compilation (linux/darwin/windows x amd64/arm64), tar.gz/zip archives, checksums, Homebrew tap, changelog generation. This is a solid configuration.
- **Solo dev value**: One command (`goreleaser release`) produces binaries for all platforms, publishes to GitHub Releases, updates Homebrew, generates checksums. The entire release pipeline in a single tool.

### ko (Go container images)
- **What**: Builds Go container images without Docker. `go build` locally, package into minimal container. Multi-platform, automatic SBOMs, Kubernetes YAML templating.
- **Solo dev value**: For Go projects that need container images, ko is simpler and faster than Dockerfile-based builds. No Docker-in-Docker in CI. Produces distroless-style minimal images.

### crane (container registry tool)
- **What**: Google's CLI for container registry operations. List tags, tag images, inspect remote images, manage metadata. SLSA provenance verification.
- **Solo dev value**: Useful for CI scripts that need to check if an image exists, copy images between registries, or inspect deployed images without pulling them.

### Hawk integration opportunity
Hawk already has a strong goreleaser setup. Extensions:
1. Validate goreleaser config when project structure changes (new main packages, new architectures)
2. Add ko integration for container image distribution alongside binary releases
3. Generate platform-specific installation instructions (Homebrew, apt, scoop, curl|bash)
4. SBOM generation for binary releases (goreleaser supports this via plugin)
5. Cosign/sigstore integration for binary signing

---

## 19. Self-Hosted Runners Optimization

### GitHub's options
- **Standard runners**: Free for public repos, limited minutes for private repos. Ubuntu, macOS, Windows.
- **Larger runners**: More RAM/CPU/disk. Static IPs. GPU options. Autoscaling. Only billed when running (no idle charges). Only for Team/Enterprise.
- **Self-hosted runners**: Your own hardware. No per-minute charges. Full control over environment. Security: must trust the code running on them.

### Optimization strategies
- **Persistent caching**: Self-hosted runners can maintain a warm Go module cache, Docker layer cache, and build artifact cache across runs. No 10GB cache limit.
- **Pre-installed tools**: Install Go, Docker, linters once instead of downloading every run.
- **Hardware matching**: Use fast NVMe storage for I/O-heavy builds (Go compilation). ARM runners for ARM builds (faster than QEMU emulation).
- **Ephemeral runners**: Use `--ephemeral` flag for security (fresh environment per job) while still benefiting from pre-installed tools via custom images.
- **Runner groups**: Separate runners for different workloads (fast for lint, beefy for builds).

### Solo dev considerations
Self-hosted runners make sense when: (1) CI minutes cost more than the hardware, (2) you need specific hardware (GPU, ARM), or (3) builds need large caches. For most solo devs, GitHub's standard runners with proper caching are sufficient. The operational overhead of maintaining self-hosted runners typically isn't worth it.

### Hawk integration opportunity
1. CI cost analysis: estimate monthly cost of current CI usage vs self-hosted alternative
2. If self-hosted is recommended: generate the runner setup script, systemd service, and Docker image with pre-installed tools
3. Cache optimization analysis: identify which CI steps would benefit most from persistent caching
4. Runner scaling advice: when to use `--ephemeral` vs persistent, how to handle concurrent jobs

---

## 20. Cost Optimization for CI/CD

### Caching strategies
- **Dependency caching**: Cache `~/go/pkg/mod` (Go), `node_modules` (Node), `~/.m2` (Maven). Saves 30-60 seconds per job.
- **Build artifact caching**: Cache compiled intermediates. For Go: cache `~/.cache/go-build`.
- **Docker layer caching**: Use `docker/build-push-action` with `cache-from/cache-to` for registry-based caching.
- **Test result caching**: Cache test results for unchanged packages (Turborepo/Nx approach).

### Parallelism optimization
- **Matrix strategy with `fail-fast: false`**: Continue other jobs when one fails (useful for cross-platform builds where a Linux failure shouldn't cancel the macOS build).
- **Job dependency graph**: Run independent jobs in parallel, dependent jobs sequentially. Don't serialize unnecessarily.
- **Test sharding**: Split test suite across N parallel runners. Each runs 1/N of tests.

### Reducing unnecessary runs
- **Path filters**: `on.push.paths` to skip CI when only docs change.
- **Concurrency groups**: `concurrency: { group: ${{ github.ref }}, cancel-in-progress: true }` cancels old runs when new commits push.
- **Skip CI**: `[skip ci]` in commit messages for documentation-only changes.
- **Conditional jobs**: `if: contains(github.event.head_commit.message, '[deploy]')` for on-demand deployment.

### Spot instances / preemptible runners
- **AWS spot instances**: 60-90% cheaper than on-demand. Risk of interruption. Good for parallelizable, idempotent CI jobs.
- **GCP preemptible VMs**: Similar savings. 24-hour maximum lifetime.
- **Self-hosted on spot**: Run self-hosted GitHub runners on spot instances with auto-scaling group.
- **Solo dev relevance**: Only relevant if you're running self-hosted. For GitHub-hosted, the cost optimization is in caching and avoiding unnecessary runs.

### Hawk integration opportunity
This is a high-value area for hawk's analytics/cost-optimization features:
1. A `/ci-cost` command that analyzes recent workflow runs (via `gh api`) and identifies:
   - Longest-running steps (candidates for caching)
   - Jobs that could run in parallel but don't
   - Runs triggered unnecessarily (could have been skipped with path filters)
   - Duplicate work across jobs (e.g., `go mod download` in both test and build)
2. Generate optimized workflow with all applicable cost-saving techniques
3. Monthly cost estimate based on usage patterns
4. Comparison: current cost vs optimized workflow vs self-hosted

---

## 21. Hawk Integration Priorities

Based on the research, here are the highest-impact integrations for hawk's solo-developer audience, ordered by value-to-effort ratio:

### Tier 1: Immediate high value (low effort, high automation)

| Integration | Why | Effort |
|---|---|---|
| **CI workflow generator** (`/ci init`) | Scan project, generate optimized GitHub Actions with caching, matrix builds, path filters, concurrency groups. Most solo devs have suboptimal CI. | Medium |
| **actionlint pre-commit hook** | Catch workflow errors before push. Zero ongoing cost. | Low |
| **Dependency audit** (`/deps audit`) | Run govulncheck + check dependency health. Already partially in CI. | Low |
| **Test impact analysis** (`/test-impact`) | `go test` only affected packages based on git diff. Saves CI time immediately. | Medium |
| **CI cost analysis** (`/ci-cost`) | Analyze workflow runs, identify waste. Leverages existing analytics infrastructure. | Medium |

### Tier 2: High value (medium effort)

| Integration | Why | Effort |
|---|---|---|
| **Release workflow** (`/release`) | Validate conventional commits, preview version, generate notes, trigger goreleaser. Builds on existing changelog skill. | Medium |
| **Docker optimization** (extend `docker-deploy` skill) | Analyze Dockerfile, suggest improvements, add ko for Go projects. | Medium |
| **Migration safety** (`/migrate`) | Review database migrations for safety issues. High-value for any project with a database. | Medium |
| **Security scanning orchestration** | Extend security-scan skill to run Trivy + govulncheck + secret detection, produce prioritized report. | Medium |
| **Deployment configuration** (`/deploy init`) | Generate Kamal/Railway deployment config based on project analysis. | Medium |

### Tier 3: Strategic value (higher effort)

| Integration | Why | Effort |
|---|---|---|
| **Preview environment setup** | Configure PR-based preview deployments (Vercel/Railway/self-hosted). | High |
| **Feature flag scaffolding** (`/flags`) | Set up Flipt with GitOps, generate SDK code. | High |
| **Performance regression detection** | Benchmark tracking in CI with PR comments. | High |
| **Infrastructure as Code** (`/infra`) | Generate Pulumi programs in Go for common patterns. | High |
| **Automated rollback** | Health-check-based deployment with auto-revert. | High |

### Tier 4: Nice to have (specialized)

| Integration | Why | Effort |
|---|---|---|
| **Monorepo tools setup** | Only relevant for monorepo projects. Detect and configure Turborepo/Nx. | Medium |
| **Self-hosted runner setup** | Only relevant when CI costs exceed hardware costs. | Medium |
| **Canary deployments** | Rarely needed for solo dev traffic levels. | High |
| **Binary signing** | Cosign/sigstore for supply chain security. Important for widely-distributed tools. | Medium |

### The "zero-thought CI/CD" vision

The ultimate goal for hawk is a single command (or automatic behavior) that gives a solo developer production-grade CI/CD without thinking about it:

1. **On project init** (`hawk init`): Detect language, frameworks, dependencies. Generate CI workflow, Dockerfile, deployment config, security scanning, dependency automation. Everything works out of the box.

2. **On every push**: Linting, testing (affected packages only), security scanning, coverage reporting -- all cached and parallelized. PR gets annotations for failed tests, security findings, coverage changes.

3. **On merge to main**: Automated version bump (from conventional commits), changelog generation (AI-rewritten for users), binary release (goreleaser), container image (ko), deployment (Kamal/Railway), health check verification.

4. **Ongoing maintenance**: Automated dependency updates (Renovate, auto-merge safe patches), vulnerability alerts with prioritized remediation, performance regression detection, CI cost monitoring.

5. **When things break**: Automated rollback on health check failure, flaky test quarantine, clear error attribution ("this deployment failed because migration X locks table Y for >30 seconds").

The key insight: **hawk should own the entire lifecycle, not just code generation**. A coding agent that can also manage CI/CD, deployments, and infrastructure turns a solo developer into a one-person engineering organization with the operational maturity of a well-staffed team.

### Existing hawk infrastructure to build on

- **hawk-skills/**: The skill system is the natural extension point. CI/CD skills can be community-contributed.
- **hawk-skills/docker-deploy/**: Already covers multi-stage builds and layer caching. Extend with ko, Slim, and optimization analysis.
- **hawk-skills/security-scan/**: Already covers secrets, injection, auth, and dependencies. Extend with Trivy/Grype orchestration.
- **hawk-skills/changelog/**: Already generates changelogs from conventional commits. Integrate into release automation.
- **.goreleaser.yml**: Already configured for cross-platform Go releases with Homebrew. Solid foundation for binary distribution.
- **.github/workflows/ci.yml**: Already has test + lint + build with matrix strategy. Missing: caching, path filters, concurrency groups, security scanning upload, test reporting.
- **.github/workflows/release.yml**: Already uses goreleaser. Missing: automated version detection, changelog validation, deployment trigger.
- **analytics/**: Cost optimization infrastructure exists. Extend to CI/CD cost analysis.
- **Dockerfile**: Already uses multi-stage build with non-root user. Could add build cache mounts, healthcheck instruction, and slim optimization.
