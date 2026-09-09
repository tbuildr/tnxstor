// Package util provides small helpers shared across tnxstor.
package util

import (
	"fmt"
	"strconv"
	"strings"
)

// Bytes returns the largest suitable unit representation of n, e.g. "4.72 GiB".
func Bytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ParseBytes converts common Nix/du size strings into a byte count. It
// understands "4.72 GiB", "512K", "1.5MiB", "123", and plain bytes. Ok is
// false when the input cannot be interpreted.
func ParseBytes(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	upper := strings.ToUpper(s)
	// Extract numeric prefix.
	i := 0
	for i < len(upper) && (upper[i] >= '0' && upper[i] <= '9' || upper[i] == '.' || upper[i] == '-' || upper[i] == '+') {
		i++
	}
	if i == 0 {
		return 0, false
	}
	num, err := strconv.ParseFloat(upper[:i], 64)
	if err != nil {
		return 0, false
	}
	suffix := strings.TrimSpace(upper[i:])
	suffix = strings.TrimSuffix(suffix, "IB") // GiB, MiB -> G, M
	suffix = strings.TrimSuffix(suffix, "B")  // KB -> K
	var mult float64 = 1
	switch suffix {
	case "":
		mult = 1
	case "K":
		mult = 1024
	case "M":
		mult = 1024 * 1024
	case "G":
		mult = 1024 * 1024 * 1024
	case "T":
		mult = 1024 * 1024 * 1024 * 1024
	case "P":
		mult = 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0, false
	}
	if num < 0 {
		return 0, false
	}
	return uint64(num * mult), true
}
