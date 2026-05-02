//go:build linux

package sandbox

// SeccompProfile provides syscall filtering via seccomp-bpf.
// Blocks dangerous syscalls that a coding agent should never need:
// mount, ptrace, reboot, kexec_load, init_module, bpf, etc.
//
// The BPF program uses a simple allowlist approach:
// 1. Load the syscall number from the seccomp_data struct.
// 2. If it matches a blocked syscall, return SECCOMP_RET_ERRNO (EPERM).
// 3. Otherwise return SECCOMP_RET_ALLOW.

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// seccomp / BPF constants
// ---------------------------------------------------------------------------

const (
	// seccomp operations for prctl / seccomp(2)
	prSetSeccomp     = 22
	seccompModeFilter = 2

	// seccomp(2) syscall number on amd64
	sysSeccomp = 317

	// BPF instruction classes
	bpfLD  = 0x00
	bpfJMP = 0x05
	bpfRET = 0x06

	// BPF ld/st sizes
	bpfW = 0x00

	// BPF src operands
	bpfABS = 0x20
	bpfK   = 0x00

	// BPF jump comparisons
	bpfJEQ = 0x10

	// seccomp return values
	seccompRetAllow = 0x7fff0000
	seccompRetErrno = 0x00050000 // SECCOMP_RET_ERRNO | errno
)

// syscalls that a coding agent should never need.
var blockedSyscalls = []uint32{
	// Privilege escalation / namespace manipulation
	161, // chroot
	165, // mount
	166, // umount2
	167, // swapon
	168, // swapoff
	175, // init_module
	176, // delete_module
	313, // finit_module

	// Dangerous debugging / tracing
	101, // ptrace
	310, // process_vm_readv
	311, // process_vm_writev

	// System destruction
	169, // reboot
	246, // kexec_load
	320, // kexec_file_load

	// BPF (prevent overwriting our own filter)
	321, // bpf

	// Keyring manipulation
	248, // add_key
	249, // request_key
	250, // keyctl

	// Userfaultfd (can be abused for races)
	323, // userfaultfd

	// Kernel module signing
	312, // kcmp
}

// ---------------------------------------------------------------------------
// BPF instruction encoding
// ---------------------------------------------------------------------------

// bpfInsn matches struct sock_filter (linux/filter.h).
type bpfInsn struct {
	code uint16
	jt   uint8
	jf   uint8
	k    uint32
}

// sockFprog matches struct sock_fprog.
type sockFprog struct {
	len    uint16
	_      [6]byte // padding on 64-bit
	filter unsafe.Pointer
}

// encodeBPF serialises a slice of BPF instructions to bytes.
func encodeBPF(insns []bpfInsn) []byte {
	buf := make([]byte, len(insns)*8)
	for i, ins := range insns {
		off := i * 8
		binary.LittleEndian.PutUint16(buf[off:], ins.code)
		buf[off+2] = ins.jt
		buf[off+3] = ins.jf
		binary.LittleEndian.PutUint32(buf[off+4:], ins.k)
	}
	return buf
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// DefaultSeccompProfile returns a raw BPF program (as bytes) that blocks
// dangerous syscalls.  Each blocked syscall returns EPERM to the caller.
func DefaultSeccompProfile() []byte {
	// Program structure:
	//   BPF_LD | BPF_W | BPF_ABS  offsetof(seccomp_data, nr)   -- load syscall nr
	//   for each blocked syscall:
	//     BPF_JMP | BPF_JEQ | BPF_K  nr  jt=0 jf=1  -- if match, fall through to kill
	//     BPF_RET | BPF_K  SECCOMP_RET_ERRNO|EPERM   -- deny
	//   BPF_RET | BPF_K  SECCOMP_RET_ALLOW           -- allow everything else

	n := len(blockedSyscalls)
	insns := make([]bpfInsn, 0, 1+n*2+1)

	// Load syscall number.  offsetof(struct seccomp_data, nr) == 0.
	insns = append(insns, bpfInsn{
		code: bpfLD | bpfW | bpfABS,
		k:    0,
	})

	// For each blocked syscall, emit a conditional deny.
	for i, nr := range blockedSyscalls {
		// Jump distances: if equal, execute the next instruction (deny).
		// If not equal, skip the deny and continue checking.
		remaining := n - i // instructions left including this pair
		_ = remaining
		insns = append(insns,
			bpfInsn{
				code: bpfJMP | bpfJEQ | bpfK,
				jt:   0, // true: fall through to deny
				jf:   1, // false: skip deny
				k:    nr,
			},
			bpfInsn{
				code: bpfRET | bpfK,
				k:    seccompRetErrno | 1, // EPERM = 1
			},
		)
	}

	// Default: allow.
	insns = append(insns, bpfInsn{
		code: bpfRET | bpfK,
		k:    seccompRetAllow,
	})

	return encodeBPF(insns)
}

// ApplySeccomp applies the default seccomp-bpf filter to the current process.
// The filter is irreversible: once installed it cannot be removed.
// Requires PR_SET_NO_NEW_PRIVS to be set first (Landlock's Apply does this).
func ApplySeccomp() error {
	prog := DefaultSeccompProfile()
	nInsns := len(prog) / 8

	// Ensure NO_NEW_PRIVS is set (idempotent if already set by Landlock).
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_PRCTL,
		uintptr(0x26), // PR_SET_NO_NEW_PRIVS
		1, 0, 0, 0, 0,
	); errno != 0 {
		return fmt.Errorf("prctl(NO_NEW_PRIVS): %w", errno)
	}

	// Build sock_fprog.
	fprog := sockFprog{
		len:    uint16(nInsns),
		filter: unsafe.Pointer(&prog[0]),
	}

	// seccomp(SECCOMP_SET_MODE_FILTER, 0, &fprog)
	if _, _, errno := syscall.Syscall(
		uintptr(sysSeccomp),
		uintptr(seccompModeFilter),
		0,
		uintptr(unsafe.Pointer(&fprog)),
	); errno != 0 {
		// Fallback: try prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &fprog)
		if _, _, errno2 := syscall.Syscall(
			syscall.SYS_PRCTL,
			uintptr(prSetSeccomp),
			uintptr(seccompModeFilter),
			uintptr(unsafe.Pointer(&fprog)),
		); errno2 != 0 {
			return fmt.Errorf("seccomp: %w (prctl fallback: %w)", errno, errno2)
		}
	}

	return nil
}
