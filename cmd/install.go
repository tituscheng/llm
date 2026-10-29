package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"llm/install"
)

func compileTool(cmd *cobra.Command, tool install.Tool) error {
	dir, err := install.ToolsDir()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Compiling %s into %s\n", tool.Name, dir)
	return install.Install(cmd.Context(), tool.Name, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func toolNames(tools []install.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func selectTool(cmd *cobra.Command, cat install.Category, args []string) (install.Tool, error) {
	if len(cat.Tools) == 0 {
		return install.Tool{}, fmt.Errorf("no %s tools are available", cat.ID)
	}

	if len(args) == 1 {
		tool, ok := cat.Find(args[0])
		if !ok {
			return install.Tool{}, fmt.Errorf("unknown %s tool %q", cat.ID, args[0])
		}
		return tool, nil
	}

	if len(cat.Tools) == 1 {
		return cat.Tools[0], nil
	}

	return promptTool(cmd, cat)
}

func promptTool(cmd *cobra.Command, cat install.Category) (install.Tool, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Select a %s tool to compile into ~/.llm/tools:\n\n", cat.ID)
	for i, tool := range cat.Tools {
		if tool.Note != "" {
			fmt.Fprintf(out, "  %d) %-12s  %s\n", i+1, tool.Name, tool.Note)
			continue
		}
		fmt.Fprintf(out, "  %d) %s\n", i+1, tool.Name)
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, "Enter number or name: ")

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return install.Tool{}, fmt.Errorf("read selection: %w", err)
		}
		if !isInteractive(cmd) {
			return install.Tool{}, fmt.Errorf("select a tool: llm %s download <name>", cat.ID)
		}
		return install.Tool{}, fmt.Errorf("no tool selected")
	}

	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return install.Tool{}, fmt.Errorf("no tool selected")
	}

	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(cat.Tools) {
			return install.Tool{}, fmt.Errorf("invalid selection %d", n)
		}
		return cat.Tools[n-1], nil
	}

	tool, ok := cat.Find(choice)
	if !ok {
		return install.Tool{}, fmt.Errorf("unknown %s tool %q", cat.ID, choice)
	}
	return tool, nil
}

func isInteractive(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
