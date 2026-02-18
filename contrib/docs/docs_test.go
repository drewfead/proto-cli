package docs

import (
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestMarkdown_SingleCommandWithFlags(t *testing.T) {
	cmd := &cli.Command{
		Name:  "myapp",
		Usage: "A test application",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "Path to config file",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose output",
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "# myapp") {
		t.Error("expected title '# myapp'")
	}
	if !strings.Contains(out, "A test application") {
		t.Error("expected usage text")
	}
	if !strings.Contains(out, "## Global Flags") {
		t.Error("expected global flags section")
	}
	if !strings.Contains(out, "`--config`") {
		t.Error("expected --config flag")
	}
	if !strings.Contains(out, "`--verbose`") {
		t.Error("expected --verbose flag")
	}
	if !strings.Contains(out, "Path to config file") {
		t.Error("expected config flag usage")
	}
}

func TestMarkdown_NestedCommands(t *testing.T) {
	cmd := &cli.Command{
		Name:  "myapp",
		Usage: "A test application",
		Commands: []*cli.Command{
			{
				Name:  "service",
				Usage: "Service commands",
				Commands: []*cli.Command{
					{
						Name:  "start",
						Usage: "Start the service",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:  "port",
								Usage: "Port to listen on",
							},
						},
					},
					{
						Name:  "stop",
						Usage: "Stop the service",
					},
				},
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "## Commands") {
		t.Error("expected commands section")
	}
	if !strings.Contains(out, "### service") {
		t.Error("expected ### service heading")
	}
	if !strings.Contains(out, "#### start") {
		t.Error("expected #### start heading")
	}
	if !strings.Contains(out, "#### stop") {
		t.Error("expected #### stop heading")
	}
	if !strings.Contains(out, "`--port`") {
		t.Error("expected --port flag in start command")
	}
}

func TestMarkdown_WithTitle(t *testing.T) {
	cmd := &cli.Command{
		Name:  "myapp",
		Usage: "A test application",
	}

	out := Markdown(cmd, WithTitle("My Custom Title"))

	if !strings.Contains(out, "# My Custom Title") {
		t.Error("expected custom title")
	}
	if strings.Contains(out, "# myapp") {
		t.Error("should not contain default title")
	}
}

func TestMarkdown_RequiredFlagsAndAliases(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "User name",
				Required: true,
			},
			&cli.IntFlag{
				Name:    "count",
				Aliases: []string{"c"},
				Usage:   "Number of items",
			},
		},
	}

	out := Markdown(cmd)

	// Check required flag shows "Yes"
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "`--name`") {
			if !strings.Contains(line, "Yes") {
				t.Error("expected required=Yes for --name flag")
			}
			if !strings.Contains(line, "-n") {
				t.Error("expected alias -n for --name flag")
			}
		}
		if strings.Contains(line, "`--count`") {
			if strings.Contains(line, "Yes") {
				t.Error("expected no required marker for --count flag")
			}
			if !strings.Contains(line, "-c") {
				t.Error("expected alias -c for --count flag")
			}
		}
	}
}

func TestMarkdown_HiddenFlagsExcluded(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "visible",
				Usage: "A visible flag",
			},
			&cli.StringFlag{
				Name:   "secret",
				Usage:  "A hidden flag",
				Hidden: true,
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "`--visible`") {
		t.Error("expected visible flag")
	}
	if strings.Contains(out, "secret") {
		t.Error("hidden flag should be excluded")
	}
}

func TestMarkdown_HiddenCommandsExcluded(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Commands: []*cli.Command{
			{
				Name:  "public",
				Usage: "A public command",
			},
			{
				Name:   "internal",
				Usage:  "An internal command",
				Hidden: true,
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "### public") {
		t.Error("expected public command")
	}
	if strings.Contains(out, "internal") {
		t.Error("hidden command should be excluded")
	}
}

func TestMarkdown_PipeEscaping(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "level",
				Usage: "Log level [debug|info|warn|error]",
			},
		},
	}

	out := Markdown(cmd)

	// Pipes in usage should be escaped
	if !strings.Contains(out, `debug\|info\|warn\|error`) {
		t.Errorf("expected escaped pipes in usage, got:\n%s", out)
	}
	// Should not have unescaped pipes breaking the table (other than table separators)
}

func TestMarkdown_UsageTextCodeBlock(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Commands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Get a resource",
				UsageText: "get --id <resource-id> [--format json]",
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "```\nget --id <resource-id> [--format json]\n```") {
		t.Error("expected UsageText rendered in code block")
	}
}

func TestMarkdown_DefaultText(t *testing.T) {
	cmd := &cli.Command{
		Name: "myapp",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "host",
				Usage:       "Server host",
				DefaultText: "<hostname>",
			},
		},
	}

	out := Markdown(cmd)

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "`--host`") {
			if !strings.Contains(line, "<hostname>") {
				t.Error("expected default text '<hostname>' for --host flag")
			}
		}
	}
}

func TestMarkdown_EmptyCommand(t *testing.T) {
	cmd := &cli.Command{
		Name: "empty",
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "# empty") {
		t.Error("expected title for empty command")
	}
	// Should not have flags or commands sections
	if strings.Contains(out, "## Global Flags") {
		t.Error("should not have global flags section")
	}
	if strings.Contains(out, "## Commands") {
		t.Error("should not have commands section")
	}
}

func TestMarkdown_Description(t *testing.T) {
	cmd := &cli.Command{
		Name:        "myapp",
		Usage:       "Short usage",
		Description: "This is a longer description of the application.",
		Commands: []*cli.Command{
			{
				Name:        "sub",
				Usage:       "Sub usage",
				Description: "Sub description text.",
			},
		},
	}

	out := Markdown(cmd)

	if !strings.Contains(out, "This is a longer description of the application.") {
		t.Error("expected root description")
	}
	if !strings.Contains(out, "Sub description text.") {
		t.Error("expected subcommand description")
	}
}
