package toolresult

import (
	"fmt"
	"time"
)

const (
	IconRead     = "\U0001F4C4" // 📄
	IconGlob     = "\U0001F50D" // 🔍
	IconGrep     = "\U0001F50E" // 🔎
	IconWeb      = "\U0001F310" // 🌐
	IconError    = "\u274C"     // ❌
	IconSuccess  = "\u2713"     // ✓
	IconFile     = "\U0001F4C1" // 📁
	IconDuration = "\u23F1"     // ⏱
)

func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func FormatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
}
