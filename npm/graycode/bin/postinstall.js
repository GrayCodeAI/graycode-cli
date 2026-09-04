#!/usr/bin/env node
// Runs once after npm install/update. Reads the Graycode binary from the matching
// per-platform optional dependency (@graycodeai/graycode-<platform>-<arch>) and
// installs it to ~/.graycode/bin/ using versioned filenames:
//
//   Unix:    graycode-<version>  +  graycode  (symlink)
//   Windows: graycode-<version>.exe  +  graycode.exe  (copy)
//
// Versioned files ensure running processes are never disrupted on macOS (and
// any codesigned platform): replacing a binary that a running process has
// mmap'd invalidates the kernel's code-signature cache, and the kernel then
// SIGKILLs that process. Installing into a per-version file and swapping the
// symlink means a running process keeps its open fd on the old inode and keeps
// running the previous version untouched — no SIGKILL, no disruption.
//
// This is the npm-path equivalent of the shell `install.sh` (W1C), which uses
// the same versioned-at-~/.graycode/bin/+symlink convention. Install either path;
// they converge on the same layout.
//
// Suppress entirely with GRAYCODE_SKIP_POSTINSTALL=1 (useful for CI / controlled
// installs where you manage the binary yourself).
//
// Source pattern: grok `npm/grok/bin/postinstall.js`.
const path = require('path');
const fs = require('fs');
const os = require('os');
const zlib = require('zlib');

// Canonical install location mirrors install.sh (GRAYCODE_HOME default ~/.graycode).
const GRAYCODE_HOME = process.env.GRAYCODE_HOME || path.join(os.homedir(), '.graycode');
const CANONICAL_DIR = path.join(GRAYCODE_HOME, 'bin');

const key = `${process.platform}-${process.arch}`;
const SUPPORTED = new Set([
    'darwin-arm64',
    'darwin-x64',
    'linux-x64',
    'linux-arm64',
    'win32-x64',
    'win32-arm64',
]);
if (!SUPPORTED.has(key)) {
    console.error(`@graycodeai/graycode: unsupported platform ${key}`);
    process.exit(0);
}

// Resolve the per-platform sibling package's directory. The matching
// optionalDependency is installed by npm based on `os`/`cpu` filters; the
// other five are silently skipped. If the matching one is missing, npm was
// likely invoked with --no-optional or the platform is unsupported.
function resolvePlatformPackageDir() {
    const platformPkg = `@graycodeai/graycode-${key}`;
    try {
        return path.dirname(require.resolve(`${platformPkg}/package.json`));
    } catch {
        return null;
    }
}

let version;
try { version = require('../package.json').version; } catch {}
if (!version) {
    console.error('@graycodeai/graycode: unable to determine version');
    process.exit(0);
}

const IS_WINDOWS = process.platform === 'win32';
const EXE = IS_WINDOWS ? '.exe' : '';

fs.mkdirSync(CANONICAL_DIR, { recursive: true });

// Install a vendored binary: versioned filename + symlink (Unix) or copy (Windows).
// Binaries are shipped brotli-compressed in the per-platform npm tarball to keep
// each sub-package well under npm's ~200 MB tarball limit. This function
// decompresses them before installing into the canonical layout.
function installBinary(binName, sourceDir, vendorSubpath) {
    const brPath = path.join(sourceDir, 'bin', vendorSubpath + '.br');
    const rawPath = path.join(sourceDir, 'bin', vendorSubpath);
    let vendoredBinPath;
    if (fs.existsSync(brPath)) {
        const compressed = fs.readFileSync(brPath);
        const decompressed = zlib.brotliDecompressSync(compressed);
        vendoredBinPath = rawPath;
        fs.writeFileSync(vendoredBinPath, decompressed);
        if (!IS_WINDOWS) fs.chmodSync(vendoredBinPath, 0o755);
        try { fs.unlinkSync(brPath); } catch {}
    } else if (fs.existsSync(rawPath)) {
        vendoredBinPath = rawPath;
    } else {
        console.error(`@graycodeai/graycode: missing binary at ${brPath}`);
        return false;
    }

    const versionedName = `${binName}-${version}${EXE}`;
    const versionedPath = path.join(CANONICAL_DIR, versionedName);
    const canonicalName = `${binName}${EXE}`;
    const canonicalPath = path.join(CANONICAL_DIR, canonicalName);

    // Only copy if this exact version isn't already installed.
    if (!fs.existsSync(versionedPath)) {
        const tmpPath = versionedPath + `.tmp.${process.pid}`;
        try {
            fs.copyFileSync(vendoredBinPath, tmpPath);
            if (!IS_WINDOWS) fs.chmodSync(tmpPath, 0o755);
            fs.renameSync(tmpPath, versionedPath);
        } finally {
            try { fs.unlinkSync(tmpPath); } catch {}
        }
    }

    if (IS_WINDOWS) {
        // Symlinks need elevation on Windows; copy instead. If the exe is
        // locked by a running process, rename it aside then retry.
        const oldPath = canonicalPath + '.old';
        try { fs.unlinkSync(oldPath); } catch {} // stale backup from prior update
        try {
            try { fs.unlinkSync(canonicalPath); } catch {}
            fs.copyFileSync(versionedPath, canonicalPath);
        } catch (e) {
            try {
                fs.renameSync(canonicalPath, oldPath);
                try {
                    fs.copyFileSync(versionedPath, canonicalPath);
                } catch (copyErr) {
                    // Rollback: restore the old binary so the install isn't broken.
                    try { fs.renameSync(oldPath, canonicalPath); } catch {}
                    throw copyErr;
                }
            } catch (e2) {
                console.error(`@graycodeai/graycode: failed to update ${canonicalPath}: ${e2.message}`);
                console.error('Close all running graycode processes and try again.');
                return false;
            }
        }
    } else {
        // Atomic symlink swap.
        const tmpLink = canonicalPath + `.link.${process.pid}`;
        try { fs.unlinkSync(tmpLink); } catch {}
        fs.symlinkSync(versionedName, tmpLink);
        fs.renameSync(tmpLink, canonicalPath);
    }

    console.log(`${binName} ${version} installed to ${canonicalPath} -> ${versionedName}`);
    return true;
}

// Best-effort cleanup of old versioned binaries for a given binary name.
// Keeps the current version and the previous one (in case a process is still
// running the old binary and hasn't fully loaded all pages yet).
function cleanupOldVersions(binName) {
    try {
        const prefix = `${binName}-`;
        const currentVersioned = `${binName}-${version}${EXE}`;
        const entries = fs.readdirSync(CANONICAL_DIR);
        const versionedBinaries = entries
            .filter(e => {
                if (!e.startsWith(prefix)) return false;
                if (e.includes('.tmp.') || e.includes('.link.')) return false;
                if (e === currentVersioned) return false;
                const suffix = e.slice(prefix.length);
                return /^\d/.test(suffix);
            })
            .sort((a, b) => {
                const pa = a.slice(prefix.length).split('.').map(Number);
                const pb = b.slice(prefix.length).split('.').map(Number);
                for (let i = 0; i < 3; i++) {
                    if ((pa[i] || 0) !== (pb[i] || 0)) return (pb[i] || 0) - (pa[i] || 0);
                }
                return 0;
            });
        for (const old of versionedBinaries.slice(1)) {
            try { fs.unlinkSync(path.join(CANONICAL_DIR, old)); } catch {}
        }
    } catch {}
}

// Allow CI / controlled installs to skip the postinstall bootstrap entirely.
if (process.env.GRAYCODE_SKIP_POSTINSTALL === '1') {
    console.log('@graycodeai/graycode: GRAYCODE_SKIP_POSTINSTALL=1 set; skipping binary install.');
    process.exit(0);
}

const platformDir = resolvePlatformPackageDir();
if (!platformDir) {
    console.error(`@graycodeai/graycode: platform package @graycodeai/graycode-${key} not installed.`);
    console.error('  This usually means npm was invoked with --no-optional, or the install failed.');
    console.error('  Try: npm install -g @graycodeai/graycode');
    process.exit(0);
}

installBinary('graycode', platformDir, `graycode${EXE}`);
cleanupOldVersions('graycode');

console.log('');
if (process.env.GRAYCODE_SKIP_HINT !== '1') {
    console.log(`Add ${CANONICAL_DIR} to your PATH if it is not already, e.g.`);
    console.log(`  export PATH="$PATH:${CANONICAL_DIR}"`);
    console.log('');
    console.log('Restart any running graycode sessions to pick up the new binary — the old');
    console.log('process keeps running the previous version until it is restarted.');
}
