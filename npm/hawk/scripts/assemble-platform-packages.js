#!/usr/bin/env node
// Assemble the six per-platform npm packages prior to `npm publish`.
//
// For each supported (platform, arch) target this:
//   1. Brotli-compresses the built Hawk binary into
//      `../hawk-<platform>-<arch>/bin/<bin>.br`
//   2. Stamps the sub-package's version to match the meta package
//   3. Copies the third-party notices file into the sub-package
//
// Each per-platform package is its own npm publish target. The meta package
// (`@graycodeai/hawk`) lists all six as `optionalDependencies` pinned to
// the same version; npm installs only the one matching the host's
// `os` + `cpu` filters.
//
// Why brotli? npm's tarball ceiling is ~200 MB and the raw Go binary is
// 100–150 MB per platform. Brotli at max quality cuts that to 30–40 MB,
// leaves plenty of headroom for binary growth, and is decoded by Node's
// built-in zlib.brotliDecompressSync (no native deps required).
//
// Source paths come from environment variables (set in CI) and fall back to
// the default GoReleaser build output dirs for local testing.
//
// Source pattern: grok `npm/grok/scripts/assemble-platform-packages.js`.
// Hawk's binary is the plain Go binary named `hawk` (or `hawk.exe`), produced
// by GoReleaser (`.goreleaser.yml`, main `./cmd/hawk`, project_name `hawk`).
const fs = require('fs');
const path = require('path');
const { promisify } = require('util');
const zlib = require('zlib');

const brotliCompress = promisify(zlib.brotliCompress);

// GoReleaser v2 default build output root. Override with DIST_ROOT when CI
// produces artifacts elsewhere (e.g. a GITHUB_WORKSPACE-relative `dist`).
const distRoot = process.env.DIST_ROOT || path.resolve(__dirname, '..', '..', '..', 'dist');
const npmRoot = path.resolve(__dirname, '..', '..');

// Third-party notices: best-effort. Point at the canonical notices file if it
// exists; if the repo has none, copy is skipped so packaging proceeds.
const NOTICES_SOURCE = process.env.HAWK_THIRD_PARTY_NOTICES
    || path.resolve(npmRoot, '..', '..', 'THIRD_PARTY_NOTICES.md');
const NOTICES_NAME = 'THIRD_PARTY_NOTICES.md';

const META_PKG_JSON = path.resolve(__dirname, '..', 'package.json');
const meta = JSON.parse(fs.readFileSync(META_PKG_JSON, 'utf8'));
const VERSION = meta.version;

function ensureDir(p) { fs.mkdirSync(path.dirname(p), { recursive: true }); }

async function packPlatform({ platform, arch, envVar, defaultSource, binName }) {
    const pkgDir = path.join(npmRoot, `hawk-${platform}-${arch}`);
    const pkgJsonPath = path.join(pkgDir, 'package.json');

    if (!fs.existsSync(pkgJsonPath)) {
        console.error(`[assemble] Missing per-platform package at ${pkgDir}`);
        return false;
    }

    const source = process.env[envVar] || defaultSource;
    if (!fs.existsSync(source)) {
        console.error(`[assemble] Missing binary for ${platform}-${arch}: ${source}`);
        console.error(`            Set ${envVar} or build to the default location.`);
        return false;
    }

    // Stamp the sub-package's version to match the meta package.
    const subPkg = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8'));
    subPkg.version = VERSION;
    fs.writeFileSync(pkgJsonPath, JSON.stringify(subPkg, null, 4) + '\n');

    if (fs.existsSync(NOTICES_SOURCE)) {
        fs.copyFileSync(NOTICES_SOURCE, path.join(pkgDir, NOTICES_NAME));
    } else {
        console.warn(`[assemble] ${platform}-${arch}: third-party notices not found at ${NOTICES_SOURCE} (skipping)`);
    }

    // Brotli-compress into the sub-package's bin/.
    const outBr = path.join(pkgDir, 'bin', `${binName}.br`);
    ensureDir(outBr);
    const raw = fs.readFileSync(source);
    const compressed = await brotliCompress(raw, {
        params: { [zlib.constants.BROTLI_PARAM_QUALITY]: zlib.constants.BROTLI_MAX_QUALITY },
    });
    fs.writeFileSync(outBr, compressed);
    console.log(
        `[assemble] hawk-${platform}-${arch}@${VERSION}: ` +
        `${(raw.length / 1048576).toFixed(1)} MB -> ${(compressed.length / 1048576).toFixed(1)} MB ` +
        `(${path.relative(npmRoot, outBr)})`
    );
    return true;
}

async function main() {
    // Default source paths follow GoReleaser v2 layout:
    //   dist/<project>_<os>_<arch>[vN]/binary
    // with binary `hawk` (Unix) / `hawk.exe` (Windows). The build ID is `hawk`,
    // so the dir is `dist/hawk_<os>_<arch>_v1`. The `[vN]` arch version suffix
    // is GoReleaser's GOAMD64 default (v1 for amd64); if your config changes
    // this, set the HAWK_* env vars instead of relying on the defaults.
    const goosGoarch = {
        darwin: { arm64: 'arm64', x64: 'amd64' },
        linux: { arm64: 'arm64', x64: 'amd64' },
        win32: { arm64: 'arm64', x64: 'amd64' },
    };

    function gorel(platform, arch) {
        const ga = goosGoarch[platform]?.[arch];
        if (!ga) throw new Error(`no goarch mapping for ${platform}-${arch}`);
        // No windows/arm64 build per .goreleaser.yml `ignore`, but we still ship
        // a sub-package target (see comment below) for completeness; the env var
        // override is the real source there.
        const os = platform === 'win32' ? 'windows' : platform;
        const bin = platform === 'win32' ? 'hawk.exe' : 'hawk';
        const dirName = `hawk_${os}_${ga}${ga === 'arm64' ? '' : '_v1'}`;
        return path.join(distRoot, dirName, bin);
    }

    // Note: hawk's `.goreleaser.yml` currently ignores windows/arm64 in builds,
    // so hawk-win32-arm64 has no default GoReleaser artifact yet. The target is
    // retained so that when the build matrix adds it (or CI supplies the
    // binary via HAWK_WIN32_ARM64), packaging works without script changes.
    const targets = [
        {
            platform: 'darwin', arch: 'arm64', binName: 'hawk',
            envVar: 'HAWK_DARWIN_ARM64',
            defaultSource: gorel('darwin', 'arm64'),
        },
        {
            platform: 'darwin', arch: 'x64', binName: 'hawk',
            envVar: 'HAWK_DARWIN_X64',
            defaultSource: gorel('darwin', 'x64'),
        },
        {
            platform: 'linux', arch: 'x64', binName: 'hawk',
            envVar: 'HAWK_LINUX_X64',
            defaultSource: gorel('linux', 'x64'),
        },
        {
            platform: 'linux', arch: 'arm64', binName: 'hawk',
            envVar: 'HAWK_LINUX_ARM64',
            defaultSource: gorel('linux', 'arm64'),
        },
        {
            platform: 'win32', arch: 'x64', binName: 'hawk.exe',
            envVar: 'HAWK_WIN32_X64',
            defaultSource: gorel('win32', 'x64'),
        },
        {
            platform: 'win32', arch: 'arm64', binName: 'hawk.exe',
            envVar: 'HAWK_WIN32_ARM64',
            defaultSource: gorel('win32', 'arm64'),
        },
    ];

    // Compress in parallel — brotliCompress runs on the libuv thread pool so
    // calls genuinely overlap (set UV_THREADPOOL_SIZE>=6 in CI for full
    // parallelism; Node's default pool size is 4).
    const results = await Promise.all(targets.map(packPlatform));
    const failed = results.filter(r => !r).length;
    if (failed > 0) {
        console.error(`[assemble] ${failed} target(s) failed.`);
        process.exit(1);
    }

    console.log(`[assemble] All 6 per-platform packages assembled at version ${VERSION}.`);
}

main().catch((err) => { console.error(err); process.exit(1); });
