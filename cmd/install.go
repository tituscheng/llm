package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"llm/internal/install"
	"llm/internal/tools"
)

func installToCWD(bin tools.Binary) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	res, err := install.ToDir(cwd, bin)
	if err != nil {
		return err
	}

	for _, file := range res.Files {
		switch {
		case file.Copied && file.Version != "":
			fmt.Printf("%s %s copied to %s\n", file.Name, file.Version, file.Path)
		case file.Copied:
			fmt.Printf("%s copied to %s\n", file.Name, file.Path)
		default:
			fmt.Printf("%s already exists at %s\n", file.Name, file.Path)
		}
	}
	return nil
}

func binaryNames(bins []tools.Binary) []string {
	names := make([]string, 0, len(bins))
	for _, bin := range bins {
		names = append(names, bin.Name)
	}
	return names
}

func selectBinary(cmd *cobra.Command, cat tools.Category, args []string) (tools.Binary, error) {
	if len(cat.Binaries) == 0 {
		return tools.Binary{}, fmt.Errorf("no %s binaries are embedded", cat.ID)
	}

	if len(args) == 1 {
		bin, ok := cat.Find(args[0])
		if !ok {
			return tools.Binary{}, fmt.Errorf("unknown %s binary %q", cat.ID, args[0])
		}
		return bin, nil
	}

	if len(cat.Binaries) == 1 {
		return cat.Binaries[0], nil
	}

	return promptBinary(cmd, cat)
}

func promptBinary(cmd *cobra.Command, cat tools.Category) (tools.Binary, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Select a %s binary to copy into the current directory:\n\n", cat.ID)
	for i, bin := range cat.Binaries {
		fmt.Fprintf(out, "  %d) %-10s  %s\n", i+1, bin.Name, bin.Version)
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, "Enter number or name: ")

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return tools.Binary{}, fmt.Errorf("read selection: %w", err)
		}
		if !isInteractive(cmd) {
			return tools.Binary{}, fmt.Errorf("select a binary: llm %s download <name>", cat.ID)
		}
		return tools.Binary{}, fmt.Errorf("no binary selected")
	}

	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return tools.Binary{}, fmt.Errorf("no binary selected")
	}

	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(cat.Binaries) {
			return tools.Binary{}, fmt.Errorf("invalid selection %d", n)
		}
		return cat.Binaries[n-1], nil
	}

	bin, ok := cat.Find(choice)
	if !ok {
		return tools.Binary{}, fmt.Errorf("unknown %s binary %q", cat.ID, choice)
	}
	return bin, nil
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
