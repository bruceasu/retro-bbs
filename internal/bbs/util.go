package bbs

import (
	"time"
)

func fmtRelTime(unix int64) string {
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h ago"
	case d < 7*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d ago"
	default:
		return fmtTime(unix)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "..."
}

func fmtTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04")
}
