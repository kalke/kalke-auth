package logging

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Setup configures the default slog logger for structured audit output.
func Setup(service, level, format string) *slog.Logger {
	lv := parseLevel(level)
	opts := &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "ts"
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.UTC().Format(time.RFC3339Nano))
				}
			}
			if a.Key == slog.MessageKey {
				a.Key = "event"
			}
			return a
		},
	}
	var h slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "text") {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	base := slog.New(h).With("service", service)
	slog.SetDefault(base)
	return base
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
