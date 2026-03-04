package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nlpodyssey/safetensors"
	"github.com/spf13/cobra"
)

var safetensorsCmd = &cobra.Command{
	Use:   "safetensors",
	Short: "Safetensors file operations",
	Long:  "Tools for working with safetensors files",
}

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge multiple safetensors files into one",
	Long:  "Merge all .safetensors files in the current directory into a single file",
	RunE: func(cmd *cobra.Command, args []string) error {
		inputDir, err := cmd.Flags().GetString("input")
		if err != nil {
			return err
		}
		outputFile, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		if inputDir == "" {
			inputDir = "."
		}

		entries, err := os.ReadDir(inputDir)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}

		var shardFiles []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".safetensors") {
				shardFiles = append(shardFiles, entry.Name())
			}
		}

		if len(shardFiles) == 0 {
			return fmt.Errorf("no safetensors files found in %s", inputDir)
		}

		if len(shardFiles) == 1 {
			fmt.Println("Only one safetensors file found, no merge needed.")
			if outputFile != "" {
				inputPath := filepath.Join(inputDir, shardFiles[0])
				data, err := os.ReadFile(inputPath)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}
				err = os.WriteFile(outputFile, data, 0644)
				if err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
				fmt.Printf("Copied to %s\n", outputFile)
			}
			return nil
		}

		sort.Slice(shardFiles, func(i, j int) bool {
			return extractShardNumber(shardFiles[i]) < extractShardNumber(shardFiles[j])
		})

		fmt.Printf("Found %d safetensors files, merging...\n", len(shardFiles))

		merged := make(map[string]struct {
			shape []uint64
			dtype safetensors.DType
			data  []byte
		})

		for _, shardFile := range shardFiles {
			shardPath := filepath.Join(inputDir, shardFile)
			fmt.Printf("Processing %s...\n", shardFile)

			data, err := os.ReadFile(shardPath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", shardFile, err)
			}

			st, err := safetensors.Deserialize(data)
			if err != nil {
				return fmt.Errorf("failed to deserialize %s: %w", shardFile, err)
			}

			tensors := st.Tensors()
			for _, t := range tensors {
				if _, exists := merged[t.Name]; !exists {
					merged[t.Name] = struct {
						shape []uint64
						dtype safetensors.DType
						data  []byte
					}{
						shape: t.TensorView.Shape(),
						dtype: t.TensorView.DType(),
						data:  t.TensorView.Data(),
					}
				}
			}
		}

		if outputFile == "" {
			outputFile = "merged.safetensors"
		}

		tensorMap := make(map[string]safetensors.TensorView)
		for name, t := range merged {
			tv, err := safetensors.NewTensorView(t.dtype, t.shape, t.data)
			if err != nil {
				return fmt.Errorf("failed to create tensor view for %s: %w", name, err)
			}
			tensorMap[name] = tv
		}

		serialized, err := safetensors.Serialize(tensorMap, nil)
		if err != nil {
			return fmt.Errorf("failed to serialize: %w", err)
		}

		err = os.WriteFile(outputFile, serialized, 0644)
		if err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}

		fmt.Printf("Merged %d tensors into %s\n", len(merged), outputFile)
		return nil
	},
}

func extractShardNumber(filename string) int {
	parts := strings.Split(filename, "-")
	for _, part := range parts {
		if strings.HasSuffix(part, ".safetensors") {
			numStr := strings.TrimSuffix(part, ".safetensors")
			if num, err := strconv.Atoi(numStr); err == nil {
				return num
			}
		}
		parts2 := strings.Split(part, ".")
		for _, p := range parts2 {
			if num, err := strconv.Atoi(p); err == nil {
				return num
			}
		}
	}
	return 0
}

func init() {
	rootCmd.AddCommand(safetensorsCmd)
	safetensorsCmd.AddCommand(mergeCmd)
	mergeCmd.Flags().StringP("input", "i", ".", "Input directory containing safetensors files")
	mergeCmd.Flags().StringP("output", "o", "", "Output file (default: merged.safetensors)")
}
