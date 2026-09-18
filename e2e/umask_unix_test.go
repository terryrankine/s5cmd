//go:build !windows

package e2e

import "syscall"

// setUmask pins the umask of the test process, and so of every s5cmd it
// spawns, to 022. Downloaded files honour the umask, and the tests expect
// them to be 0644.
func setUmask() {
	syscall.Umask(0o022)
}
