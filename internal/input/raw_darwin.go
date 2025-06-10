//go:build darwin

package input

import (
	"os"
	"syscall"
	"unsafe"
)

// startRaw puts the terminal in raw mode and returns the previous state on macOS.
func startRaw() (*syscall.Termios, error) {
	fd := int(os.Stdin.Fd())
	var old syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&old)), 0, 0, 0); err != 0 {
		return nil, err
	}
	newState := old
	newState.Lflag &^= syscall.ICANON | syscall.ECHO
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&newState)), 0, 0, 0); err != 0 {
		return nil, err
	}
	return &old, nil
}

// stopRaw restores the terminal state returned from startRaw.
func stopRaw(state *syscall.Termios) {
	if state == nil {
		return
	}
	fd := int(os.Stdin.Fd())
	syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(state)), 0, 0, 0)
}
