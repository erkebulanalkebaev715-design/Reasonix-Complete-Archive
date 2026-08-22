package main

import (
	"fmt"
	"strings"

	"reasonix/internal/config"
)

func main() {
	long := strings.Repeat("always-verify-the-desktop-session-lease-before-rebuild-", 8) + "end"
	stem := strings.Trim(strings.ToLower(strings.TrimSpace(long)), "-")
	b := config.BoundFilenameComponent(stem, 255-len(".md"))
	fmt.Printf("stem len=%d bound=%d file len=%d\n", len(stem), len(b), len(b)+len(".md"))
	o := strings.Repeat("a", 240)
	b2 := config.BoundFilenameComponent(o, 128)
	fmt.Printf("stats: o len=%d bound len=%d\n", len(o), len(b2))
}
