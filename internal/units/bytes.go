// Package units provides human-readable formatting shared by the sweep TUI
// and the stats command, so both surfaces render sizes identically.
package units

import "fmt"

// Bytes renders n as a human-readable size, e.g. "128.0 MiB" or "512B".
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for k := n / unit; k >= unit; k /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
