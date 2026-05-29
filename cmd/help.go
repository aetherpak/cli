package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// renderPremiumHelp renders a premium, highly formatted help page using Lipgloss.
func renderPremiumHelp(cmd *cobra.Command, w io.Writer) {
	var (
		sectionTitleStyle lipgloss.Style
		commandNameStyle  lipgloss.Style
		flagNameStyle     lipgloss.Style
		typeStyle         lipgloss.Style
		descStyle         lipgloss.Style
		footerStyle       lipgloss.Style
	)

	if logger.IsPlain() {
		// Empty styles for clean unformatted plain text output in CI logs
		sectionTitleStyle = lipgloss.NewStyle()
		commandNameStyle = lipgloss.NewStyle()
		flagNameStyle = lipgloss.NewStyle()
		typeStyle = lipgloss.NewStyle()
		descStyle = lipgloss.NewStyle()
		footerStyle = lipgloss.NewStyle()
	} else {
		sectionTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99")) // Bright Purple/Indigo

		commandNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("36")) // Teal/Cyan

		flagNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")) // Amber/Yellow

		typeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")) // Dimmed Gray

		descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")) // Off-white

		footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))
	}

	// 1. Description Header
	if cmd.Long != "" {
		fmt.Fprintln(w, descStyle.Render(cmd.Long))
		fmt.Fprintln(w)
	} else if cmd.Short != "" {
		fmt.Fprintln(w, descStyle.Render(cmd.Short))
		fmt.Fprintln(w)
	}

	// 2. Usage Instructions
	fmt.Fprintln(w, sectionTitleStyle.Render("USAGE"))
	fmt.Fprintf(w, "  %s\n\n", cmd.UseLine())

	// 3. Examples (if any)
	if cmd.Example != "" {
		fmt.Fprintln(w, sectionTitleStyle.Render("EXAMPLES"))
		lines := strings.Split(strings.TrimSpace(cmd.Example), "\n")
		for _, line := range lines {
			fmt.Fprintf(w, "  %s\n", descStyle.Render(line))
		}
		fmt.Fprintln(w)
	}

	// 4. Subcommands list
	var visibleCmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if !c.Hidden && c.Name() != "help" {
			visibleCmds = append(visibleCmds, c)
		}
	}

	if len(visibleCmds) > 0 {
		fmt.Fprintln(w, sectionTitleStyle.Render("AVAILABLE COMMANDS"))

		maxLen := 0
		for _, c := range visibleCmds {
			if len(c.Name()) > maxLen {
				maxLen = len(c.Name())
			}
		}

		for _, c := range visibleCmds {
			padding := strings.Repeat(" ", maxLen-len(c.Name())+4)
			fmt.Fprintf(w, "  %s%s%s\n",
				commandNameStyle.Render(c.Name()),
				padding,
				descStyle.Render(c.Short),
			)
		}
		fmt.Fprintln(w)
	}

	// 5. Flag formatter helper
	formatFlags := func(title string, flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}

		var flags []*pflag.Flag
		flagSet.VisitAll(func(f *pflag.Flag) {
			if !f.Hidden {
				flags = append(flags, f)
			}
		})

		if len(flags) == 0 {
			return
		}

		fmt.Fprintln(w, sectionTitleStyle.Render(title))

		maxLen := 0
		specs := make(map[string]string)
		for _, f := range flags {
			var sb strings.Builder
			if f.Shorthand != "" {
				sb.WriteString(fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name))
			} else {
				sb.WriteString(fmt.Sprintf("    --%s", f.Name))
			}
			if f.Value.Type() != "bool" {
				sb.WriteString(fmt.Sprintf(" %s", f.Value.Type()))
			}
			spec := sb.String()
			specs[f.Name] = spec
			if len(spec) > maxLen {
				maxLen = len(spec)
			}
		}

		for _, f := range flags {
			spec := specs[f.Name]
			padding := strings.Repeat(" ", maxLen-len(spec)+4)

			var styledSpec string
			if f.Shorthand != "" {
				styledSpec = fmt.Sprintf("%s, %s",
					flagNameStyle.Render("-"+f.Shorthand),
					flagNameStyle.Render("--"+f.Name),
				)
			} else {
				styledSpec = fmt.Sprintf("    %s", flagNameStyle.Render("--"+f.Name))
			}
			if f.Value.Type() != "bool" {
				styledSpec += " " + typeStyle.Render(f.Value.Type())
			}

			defVal := ""
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "true" && f.DefValue != "\"\"" {
				defVal = fmt.Sprintf(" (default %q)", f.DefValue)
			} else if f.DefValue == "true" {
				defVal = " (default true)"
			}

			fmt.Fprintf(w, "  %s%s%s%s\n",
				styledSpec,
				padding,
				descStyle.Render(f.Usage),
				typeStyle.Render(defVal),
			)
		}
		fmt.Fprintln(w)
	}

	formatFlags("FLAGS", cmd.LocalFlags())
	formatFlags("GLOBAL FLAGS", cmd.InheritedFlags())

	// 6. Subcommands helper prompt
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w, footerStyle.Render(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", cmd.Root().Name())))
	}
}
