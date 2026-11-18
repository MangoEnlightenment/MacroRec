//go:build !windows

package main

import (
	"fmt"
	"os"
)

// macrorec uses Windows APIs; this stub only exists so the platform-independent
// code and its tests build on other systems.
func main() {
	fmt.Fprintln(os.Stderr, "macrorec only runs on Windows. Cross-compile with GOOS=windows.")
	os.Exit(1)
}
