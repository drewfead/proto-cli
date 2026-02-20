package cliauth

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

// Commands builds the "auth" parent command with subcommands based on the
// capabilities of the configured provider.
func Commands(cfg *Config) *cli.Command {
	subcommands := []*cli.Command{
		loginCommand(cfg),
	}

	if _, ok := cfg.Provider.(LogoutProvider); ok {
		subcommands = append(subcommands, logoutCommand(cfg))
	}

	if _, ok := cfg.Provider.(StatusProvider); ok {
		subcommands = append(subcommands, statusCommand(cfg))
	}

	return &cli.Command{
		Name:     "auth",
		Usage:    "manage authentication",
		Commands: subcommands,
	}
}

// loginCommand builds the "auth login" subcommand.
func loginCommand(cfg *Config) *cli.Command {
	flags := append([]cli.Flag{}, cfg.Provider.Flags()...)

	// Add --interactive flag if provider supports it
	if _, ok := cfg.Provider.(InteractiveLoginProvider); ok {
		flags = append(flags, &cli.BoolFlag{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "use interactive login prompt",
		})
	}

	return &cli.Command{
		Name:  "login",
		Usage: "authenticate with the service",
		Flags: flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			interactive, ok := cfg.Provider.(InteractiveLoginProvider)
			if !ok {
				// Non-interactive provider: always use Login
				return cfg.Provider.Login(ctx, cmd, cfg.Store)
			}

			// Interactive provider: dispatch based on flags
			if cmd.IsSet("interactive") {
				return interactive.LoginInteractive(ctx, os.Stdin, cmd.Writer, cfg.Store)
			}

			if anyProviderFlagSet(cmd, cfg.Provider.Flags()) {
				return cfg.Provider.Login(ctx, cmd, cfg.Store)
			}

			// No flags set: auto-trigger interactive
			return interactive.LoginInteractive(ctx, os.Stdin, cmd.Writer, cfg.Store)
		},
	}
}

// logoutCommand builds the "auth logout" subcommand.
func logoutCommand(cfg *Config) *cli.Command {
	return &cli.Command{
		Name:  "logout",
		Usage: "remove stored credentials",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := cfg.Provider.(LogoutProvider).Logout(ctx, cfg.Store); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.Writer, "Logged out successfully.")
			return nil
		},
	}
}

// statusCommand builds the "auth status" subcommand.
func statusCommand(cfg *Config) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "show authentication status",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			msg, err := cfg.Provider.(StatusProvider).Status(ctx, cfg.Store)
			if err != nil {
				return err
			}
			if msg == "" {
				msg = "Not authenticated."
			}
			_, _ = fmt.Fprintln(cmd.Writer, msg)
			return nil
		},
	}
}

// anyProviderFlagSet returns true if any of the given provider flags are set on cmd.
func anyProviderFlagSet(cmd *cli.Command, flags []cli.Flag) bool {
	for _, f := range flags {
		for _, name := range f.Names() {
			if cmd.IsSet(name) {
				return true
			}
		}
	}
	return false
}
