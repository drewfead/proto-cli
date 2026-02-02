package clilog

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// SlogConfigurationContext provides context information for logger configuration.
// This is a copy of protocli.SlogConfigurationContext to avoid circular imports.
type SlogConfigurationContext interface {
	// IsDaemon returns true if running in daemon mode, false for single commands
	IsDaemon() bool

	// Level returns the configured log level from the --verbosity flag
	Level() slog.Level
}

// MachineFriendlySlogHandler returns a JSON handler for machine-readable logging.
// This is useful for daemon mode or when logs need to be parsed by other tools.
//
// The handler writes to the provided writer and respects the log level from opts.
func MachineFriendlySlogHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return slog.NewJSONHandler(w, opts)
}

// AlwaysHumanFriendly returns a logging configuration callback that always uses the
// human-friendly logging format, regardless of daemon mode. This is useful for CLIs
// that want a consistent user-friendly logging experience.
//
// The returned callback respects the --verbosity flag for log level filtering.
//
// Example:
//
//	rootCmd, _ := protocli.RootCommand("mycli",
//	    protocli.Service(serviceCLI),
//	    protocli.ConfigureLogging(clilog.AlwaysHumanFriendly()),
//	)
func AlwaysHumanFriendly() func(context.Context, SlogConfigurationContext) *slog.Logger {
	return func(_ context.Context, config SlogConfigurationContext) *slog.Logger {
		handler := HumanFriendlySlogHandler(os.Stderr, &slog.HandlerOptions{
			Level: config.Level(), // Respects --verbosity flag
		})
		return slog.New(handler)
	}
}

// AlwaysMachineFriendly returns a logging configuration callback that always uses the
// machine-friendly (JSON) logging format, regardless of daemon mode. This is useful for
// CLIs that always want structured, parseable logs.
//
// The returned callback respects the --verbosity flag for log level filtering.
//
// Example:
//
//	rootCmd, _ := protocli.RootCommand("mycli",
//	    protocli.Service(serviceCLI),
//	    protocli.ConfigureLogging(clilog.AlwaysMachineFriendly()),
//	)
func AlwaysMachineFriendly() func(context.Context, SlogConfigurationContext) *slog.Logger {
	return func(_ context.Context, config SlogConfigurationContext) *slog.Logger {
		handler := MachineFriendlySlogHandler(os.Stdout, &slog.HandlerOptions{
			Level: config.Level(), // Respects --verbosity flag
		})
		return slog.New(handler)
	}
}

// Default returns a logging configuration callback that uses human-friendly logging
// for single commands and machine-friendly (JSON) logging for daemon mode.
// This is the recommended default for most CLI applications.
//
// The returned callback respects the --verbosity flag for log level filtering.
//
// Example:
//
//	rootCmd, _ := protocli.RootCommand("mycli",
//	    protocli.Service(serviceCLI),
//	    protocli.ConfigureLogging(clilog.Default()),
//	)
func Default() func(context.Context, SlogConfigurationContext) *slog.Logger {
	return func(_ context.Context, config SlogConfigurationContext) *slog.Logger {
		var handler slog.Handler
		if config.IsDaemon() {
			// Daemon mode: JSON to stdout
			handler = MachineFriendlySlogHandler(os.Stdout, &slog.HandlerOptions{
				Level: config.Level(),
			})
		} else {
			// Single command mode: Human-friendly to stderr
			handler = HumanFriendlySlogHandler(os.Stderr, &slog.HandlerOptions{
				Level: config.Level(),
			})
		}
		return slog.New(handler)
	}
}
