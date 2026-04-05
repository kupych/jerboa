//go:build !windows

package main

import "fmt"

func waitIfWindows() {
	// no-op on non-Windows
	_ = fmt.Sprintf
}
