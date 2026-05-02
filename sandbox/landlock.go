//go:build linux

package sandbox

// Landlock provides unprivileged filesystem and network access control on Linux 5.13+.
// It restricts the agent to read/write only the project directory and /tmp,
// without requiring root, Docker, or any external tools.
//
// Research basis: Landlock is the most underappreciated isolation technology.
// It works without root, without Docker, without any external tool, and adds
// near-zero overhead. This should be hawk's default Linux isolation.
//
// Security guarantees:
// - Agent can only read/write within allowed paths
// - Rules are hierarchically merged (can only add restrictions, never remove)
// - Available since Linux 5.13 (filesystem), 6.7 (network)
// - ABI is stable and versioned

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Landlock ABI constants
// ---------------------------------------------------------------------------

// Syscall numbers for landlock on amd64/arm64.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446
)

// Linux-specific open flags (defined here because the Go syscall package
// does not export them during cross-compilation from non-Linux hosts).
const (
	oPath   = 0x200000 // O_PATH
	oCloexec = 0x80000 // O_CLOEXEC
)

// Rule types.
const (
	landlockRulePathBeneath = 1
	landlockRuleNet         = 2 // ABI v4+
)

// Access rights for filesystem rules (Landlock ABI v1+).
const (
	accessFSExecute    uint64 = 1 << 0
	accessFSWriteFile  uint64 = 1 << 1
	accessFSReadFile   uint64 = 1 << 2
	accessFSReadDir    uint64 = 1 << 3
	accessFSRemoveDir  uint64 = 1 << 4
	accessFSRemoveFile uint64 = 1 << 5
	accessFSMakeChar   uint64 = 1 << 6
	accessFSMakeDir    uint64 = 1 << 7
	accessFSMakeReg    uint64 = 1 << 8
	accessFSMakeSock   uint64 = 1 << 9
	accessFSMakeFifo   uint64 = 1 << 10
	accessFSMakeBlock  uint64 = 1 << 11
	accessFSMakeSym    uint64 = 1 << 12

	// ABI v2+
	accessFSRefer uint64 = 1 << 13

	// ABI v3+
	accessFSTruncate uint64 = 1 << 14
)

// Derived convenience masks.
const (
	// accessFSReadOnly covers the minimal set needed for read-only browsing.
	accessFSReadOnly = accessFSReadFile | accessFSReadDir | accessFSExecute

	// accessFSReadWrite covers the full set a coding agent typically needs.
	accessFSReadWrite = accessFSReadOnly |
		accessFSWriteFile | accessFSRemoveDir | accessFSRemoveFile |
		accessFSMakeDir | accessFSMakeReg | accessFSMakeSym |
		accessFSTruncate

	// accessFSAll is the mask of every v1-v3 access right.  It is used when
	// creating a ruleset so the kernel knows which rights we want to govern.
	accessFSAll = accessFSExecute | accessFSWriteFile | accessFSReadFile |
		accessFSReadDir | accessFSRemoveDir | accessFSRemoveFile |
		accessFSMakeChar | accessFSMakeDir | accessFSMakeReg |
		accessFSMakeSock | accessFSMakeFifo | accessFSMakeBlock |
		accessFSMakeSym | accessFSRefer | accessFSTruncate
)

// ---------------------------------------------------------------------------
// Kernel ABI structures (must match include/uapi/linux/landlock.h)
// ---------------------------------------------------------------------------

// landlockRulesetAttr is the attribute struct for landlock_create_ruleset(2).
type landlockRulesetAttr struct {
	handledAccessFS  uint64
	handledAccessNet uint64 // ABI v4+; zero for older ABIs
}

// landlockPathBeneathAttr is the attribute struct for LANDLOCK_RULE_PATH_BENEATH.
type landlockPathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
	_             [4]byte // padding
}

// ---------------------------------------------------------------------------
// LandlockSandbox
// ---------------------------------------------------------------------------

// LandlockSandbox restricts filesystem access for the current process.
type LandlockSandbox struct {
	projectDir string
	readOnly   []string
	readWrite  []string
}

// NewLandlockSandbox creates a sandbox that allows read/write to the project
// directory and /tmp, and read-only access to essential system paths.
func NewLandlockSandbox(projectDir string) *LandlockSandbox {
	return &LandlockSandbox{
		projectDir: projectDir,
		readOnly: []string{
			"/usr",
			"/lib",
			"/lib64",
			"/etc",
			"/bin",
			"/sbin",
			"/proc",
			"/dev",
			"/sys",
		},
		readWrite: []string{
			projectDir,
			"/tmp",
		},
	}
}

// LandlockAvailable returns true if Landlock is supported on this kernel.
// It attempts to create a zero-length ruleset; ENOSYS or EOPNOTSUPP
// indicates no support.
func LandlockAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	attr := landlockRulesetAttr{
		handledAccessFS: accessFSAll,
	}
	fd, _, errno := syscall.Syscall(
		sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0, // flags
	)
	if errno != 0 {
		return false
	}
	// Kernel returned a valid fd -- close it and report success.
	syscall.Close(int(fd))
	return true
}

// Apply enforces the Landlock rules on the current process.
// After this call, the process cannot access any path not explicitly allowed.
// This is irreversible for the lifetime of the process.
func (l *LandlockSandbox) Apply() error {
	if !LandlockAvailable() {
		return fmt.Errorf("landlock: not supported by this kernel")
	}

	// 1. Determine the best ABI version by probing with the full access mask.
	//    If the kernel does not support newer bits it returns EINVAL; fall back
	//    to a smaller mask.
	handledFS := bestHandledFS()

	// 2. Create the ruleset.
	attr := landlockRulesetAttr{
		handledAccessFS: handledFS,
	}
	rulesetFD, _, errno := syscall.Syscall(
		sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	fd := int(rulesetFD)
	defer syscall.Close(fd)

	// 3. Add read-write rules.
	for _, path := range l.readWrite {
		access := accessFSReadWrite & handledFS
		if err := addPathRule(fd, path, access); err != nil {
			// Skip paths that don't exist on this system.
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("landlock: read-write rule for %q: %w", path, err)
		}
	}

	// 4. Add read-only rules.
	for _, path := range l.readOnly {
		access := accessFSReadOnly & handledFS
		if err := addPathRule(fd, path, access); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("landlock: read-only rule for %q: %w", path, err)
		}
	}

	// 5. Drop the ability to add further Landlock rules (defence in depth).
	//    prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	if _, _, errno = syscall.Syscall6(
		syscall.SYS_PRCTL,
		uintptr(0x26), // PR_SET_NO_NEW_PRIVS
		1, 0, 0, 0, 0,
	); errno != 0 {
		return fmt.Errorf("prctl(NO_NEW_PRIVS): %w", errno)
	}

	// 6. Enforce.
	if _, _, errno = syscall.Syscall(
		sysLandlockRestrictSelf,
		uintptr(fd),
		0, // flags
		0,
	); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}

	return nil
}

// AddReadOnlyPath appends a read-only path to the sandbox configuration.
// Must be called before Apply.
func (l *LandlockSandbox) AddReadOnlyPath(path string) {
	l.readOnly = append(l.readOnly, path)
}

// AddReadWritePath appends a read-write path to the sandbox configuration.
// Must be called before Apply.
func (l *LandlockSandbox) AddReadWritePath(path string) {
	l.readWrite = append(l.readWrite, path)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// addPathRule opens dir as O_PATH and issues landlock_add_rule.
func addPathRule(rulesetFD int, path string, access uint64) error {
	pathFD, err := syscall.Open(path, oPath|oCloexec, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(pathFD)

	rule := landlockPathBeneathAttr{
		allowedAccess: access,
		parentFD:      int32(pathFD),
	}
	_, _, errno := syscall.Syscall6(
		sysLandlockAddRule,
		uintptr(rulesetFD),
		uintptr(landlockRulePathBeneath),
		uintptr(unsafe.Pointer(&rule)),
		0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// bestHandledFS probes the kernel for the widest supported access mask.
// It starts with all v3 bits and drops newer ones on EINVAL.
func bestHandledFS() uint64 {
	masks := []uint64{
		accessFSAll,                                     // v3 (truncate + refer)
		accessFSAll &^ accessFSTruncate,                 // v2 (refer, no truncate)
		accessFSAll &^ accessFSTruncate &^ accessFSRefer, // v1 (base set)
	}
	for _, m := range masks {
		attr := landlockRulesetAttr{handledAccessFS: m}
		fd, _, errno := syscall.Syscall(
			sysLandlockCreateRuleset,
			uintptr(unsafe.Pointer(&attr)),
			unsafe.Sizeof(attr),
			0,
		)
		if errno == 0 {
			syscall.Close(int(fd))
			return m
		}
	}
	// Fallback: only the original v1 bits without refer.
	return accessFSAll &^ accessFSTruncate &^ accessFSRefer
}
