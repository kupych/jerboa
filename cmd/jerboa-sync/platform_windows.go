//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
)

// waitIfWindows pauses so the terminal doesn't vanish when double-clicked.
func waitIfWindows() {
	fmt.Fprint(os.Stderr, "\npress Enter to close...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
