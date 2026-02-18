package docs

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
)

type markdownConfig struct {
	title string
}

// MarkdownOption configures markdown generation.
type MarkdownOption func(*markdownConfig)

// WithTitle overrides the default title (command name) in the generated markdown.
func WithTitle(title string) MarkdownOption {
	return func(c *markdownConfig) {
		c.title = title
	}
}

// Markdown generates reference documentation in markdown format from a fully-constructed
// *cli.Command tree. This works post-wiring, so it captures everything: format flags,
// hook-added flags, all registered services.
func Markdown(cmd *cli.Command, opts ...MarkdownOption) string {
	cfg := &markdownConfig{
		title: cmd.Name,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var b strings.Builder

	// Title
	fmt.Fprintf(&b, "# %s\n\n", cfg.title)

	// Root usage and description
	if cmd.Usage != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Usage)
	}
	if cmd.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Description)
	}

	// Global flags
	if flags := visibleFlags(cmd.Flags); len(flags) > 0 {
		fmt.Fprintf(&b, "## Global Flags\n\n")
		writeFlagTable(&b, flags)
		b.WriteString("\n")
	}

	// Commands
	if cmds := visibleCommands(cmd.Commands); len(cmds) > 0 {
		fmt.Fprintf(&b, "## Commands\n\n")
		for _, sub := range cmds {
			writeCommand(&b, sub, 3)
		}
	}

	return b.String()
}

func writeCommand(b *strings.Builder, cmd *cli.Command, depth int) {
	if depth > 6 {
		depth = 6
	}
	prefix := strings.Repeat("#", depth)

	fmt.Fprintf(b, "%s %s\n\n", prefix, cmd.Name)

	if cmd.UsageText != "" {
		fmt.Fprintf(b, "```\n%s\n```\n\n", cmd.UsageText)
	}

	if cmd.Usage != "" {
		fmt.Fprintf(b, "%s\n\n", cmd.Usage)
	}

	if cmd.Description != "" {
		fmt.Fprintf(b, "%s\n\n", cmd.Description)
	}

	if flags := visibleFlags(cmd.Flags); len(flags) > 0 {
		b.WriteString("**Flags:**\n\n")
		writeFlagTable(b, flags)
		b.WriteString("\n")
	}

	for _, sub := range visibleCommands(cmd.Commands) {
		writeCommand(b, sub, depth+1)
	}
}

func writeFlagTable(b *strings.Builder, flags []cli.Flag) {
	b.WriteString("| Flag | Aliases | Usage | Default | Required |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")

	for _, f := range flags {
		names := f.Names()
		if len(names) == 0 {
			continue
		}

		primary := "--" + names[0]

		var aliases []string
		for _, a := range names[1:] {
			if len(a) == 1 {
				aliases = append(aliases, "-"+a)
			} else {
				aliases = append(aliases, "--"+a)
			}
		}
		aliasStr := strings.Join(aliases, ", ")

		var usage, defaultText string
		var required bool

		if dgf, ok := f.(cli.DocGenerationFlag); ok {
			usage = escPipe(dgf.GetUsage())
			defaultText = dgf.GetDefaultText()
		}

		if rf, ok := f.(cli.RequiredFlag); ok {
			required = rf.IsRequired()
		}

		reqStr := ""
		if required {
			reqStr = "Yes"
		}

		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s |\n",
			primary, aliasStr, usage, defaultText, reqStr)
	}
}

func visibleFlags(flags []cli.Flag) []cli.Flag {
	var result []cli.Flag
	for _, f := range flags {
		if vf, ok := f.(cli.VisibleFlag); ok && !vf.IsVisible() {
			continue
		}
		result = append(result, f)
	}
	return result
}

func visibleCommands(cmds []*cli.Command) []*cli.Command {
	var result []*cli.Command
	for _, c := range cmds {
		if c.Hidden {
			continue
		}
		result = append(result, c)
	}
	return result
}

// escPipe escapes pipe characters for markdown table safety.
func escPipe(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
