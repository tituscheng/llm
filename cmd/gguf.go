package cmd

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nlpodyssey/safetensors"
	"github.com/spf13/cobra"
)

const (
	ggufMagic     uint32 = 0x46554747
	ggufVersion   uint32 = 3
	ggufAlignment        = 32
)

type GGMLType uint16

const (
	GGMLTypeF32  GGMLType = 0
	GGMLTypeF16  GGMLType = 1
	GGMLTypeQ4_0 GGMLType = 2
	GGMLTypeQ4_1 GGMLType = 3
	GGMLTypeQ5_0 GGMLType = 6
	GGMLTypeQ5_1 GGMLType = 7
	GGMLTypeQ8_0 GGMLType = 8
	GGMLTypeQ8_1 GGMLType = 9
	GGMLTypeQ2_K GGMLType = 10
	GGMLTypeQ3_K GGMLType = 11
	GGMLTypeQ4_K GGMLType = 12
	GGMLTypeQ5_K GGMLType = 13
	GGMLTypeQ6_K GGMLType = 14
	GGMLTypeI8   GGMLType = 20
	GGMLTypeI16  GGMLType = 21
	GGMLTypeI32  GGMLType = 22
	GGMLTypeI64  GGMLType = 23
	GGMLTypeF64  GGMLType = 24
	GGMLTypeBF16 GGMLType = 25
)

func safetensorsDTypeToGGML(dt safetensors.DType) GGMLType {
	switch dt {
	case safetensors.F32:
		return GGMLTypeF32
	case safetensors.F16:
		return GGMLTypeF16
	case safetensors.BF16:
		return GGMLTypeBF16
	case safetensors.F64:
		return GGMLTypeF64
	case safetensors.I8:
		return GGMLTypeI8
	case safetensors.I16:
		return GGMLTypeI16
	case safetensors.I32:
		return GGMLTypeI32
	case safetensors.I64:
		return GGMLTypeI64
	case safetensors.U8:
		return GGMLTypeI8
	case safetensors.U16:
		return GGMLTypeI16
	case safetensors.U32:
		return GGMLTypeI32
	case safetensors.U64:
		return GGMLTypeI64
	default:
		return GGMLTypeF32
	}
}

type tensorInfo struct {
	name      string
	ndim      uint32
	shape     []uint64
	dtype     GGMLType
	offset    uint64
	nElements uint64
	data      []byte
}

type ggufWriter struct {
	w        *os.File
	metadata map[string]interface{}
	tensors  []tensorInfo
}

func newGGUFWriter(w *os.File) *ggufWriter {
	return &ggufWriter{
		w:        w,
		metadata: make(map[string]interface{}),
		tensors:  make([]tensorInfo, 0),
	}
}

func (gw *ggufWriter) SetMetadata(key string, value interface{}) {
	gw.metadata[key] = value
}

func (gw *ggufWriter) AddTensor(name string, shape []uint64, dtype GGMLType, data []byte) {
	nElements := uint64(1)
	for _, s := range shape {
		nElements *= s
	}

	var offset uint64
	if len(gw.tensors) > 0 {
		lastTensor := gw.tensors[len(gw.tensors)-1]
		elementSize := gw.getElementSize(lastTensor.dtype)
		alignedSize := gw.alignOffset(lastTensor.nElements * elementSize)
		offset = lastTensor.offset + alignedSize
	}

	gw.tensors = append(gw.tensors, tensorInfo{
		name:      name,
		ndim:      uint32(len(shape)),
		shape:     shape,
		dtype:     dtype,
		offset:    offset,
		nElements: nElements,
		data:      data,
	})
}

func (gw *ggufWriter) getElementSize(dtype GGMLType) uint64 {
	switch dtype {
	case GGMLTypeF32, GGMLTypeI32, GGMLTypeI8:
		return 4
	case GGMLTypeF16, GGMLTypeI16, GGMLTypeBF16:
		return 2
	case GGMLTypeF64, GGMLTypeI64:
		return 8
	default:
		return 4
	}
}

func (gw *ggufWriter) alignOffset(offset uint64) uint64 {
	align := uint64(ggufAlignment)
	return (offset + align - 1) & ^(align - 1)
}

func (gw *ggufWriter) Write() error {
	if err := binary.Write(gw.w, binary.LittleEndian, ggufMagic); err != nil {
		return fmt.Errorf("failed to write magic: %w", err)
	}

	if err := binary.Write(gw.w, binary.LittleEndian, ggufVersion); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	tensorCount := uint32(len(gw.tensors))
	if err := binary.Write(gw.w, binary.LittleEndian, tensorCount); err != nil {
		return fmt.Errorf("failed to write tensor count: %w", err)
	}

	_, metadataData, err := gw.serializeMetadata()
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	metadataSizeVal := uint32(len(metadataData))
	if err := binary.Write(gw.w, binary.LittleEndian, metadataSizeVal); err != nil {
		return fmt.Errorf("failed to write metadata size: %w", err)
	}

	headerSize := 4 + 4 + 4 + 4
	padding := gw.alignOffset(uint64(headerSize)) - uint64(headerSize)
	paddingData := make([]byte, padding)
	if _, err := gw.w.Write(paddingData); err != nil {
		return fmt.Errorf("failed to write padding: %w", err)
	}

	if _, err := gw.w.Write(metadataData); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	for _, tensor := range gw.tensors {
		nameLen := uint64(len(tensor.name))
		if err := binary.Write(gw.w, binary.LittleEndian, nameLen); err != nil {
			return fmt.Errorf("failed to write tensor name length: %w", err)
		}
		if _, err := gw.w.Write([]byte(tensor.name)); err != nil {
			return fmt.Errorf("failed to write tensor name: %w", err)
		}

		if err := binary.Write(gw.w, binary.LittleEndian, tensor.ndim); err != nil {
			return fmt.Errorf("failed to write tensor ndim: %w", err)
		}

		for _, s := range tensor.shape {
			if err := binary.Write(gw.w, binary.LittleEndian, s); err != nil {
				return fmt.Errorf("failed to write tensor shape: %w", err)
			}
		}

		if err := binary.Write(gw.w, binary.LittleEndian, tensor.dtype); err != nil {
			return fmt.Errorf("failed to write tensor dtype: %w", err)
		}

		if err := binary.Write(gw.w, binary.LittleEndian, tensor.offset); err != nil {
			return fmt.Errorf("failed to write tensor offset: %w", err)
		}
	}

	for _, tensor := range gw.tensors {
		alignedSize := gw.alignOffset(tensor.nElements * gw.getElementSize(tensor.dtype))
		currentDataSize := uint64(len(tensor.data))
		if currentDataSize < alignedSize {
			padding := make([]byte, alignedSize-currentDataSize)
			if _, err := gw.w.Write(padding); err != nil {
				return fmt.Errorf("failed to write tensor data padding: %w", err)
			}
		}
		if _, err := gw.w.Write(tensor.data); err != nil {
			return fmt.Errorf("failed to write tensor data: %w", err)
		}
	}

	return nil
}

func (gw *ggufWriter) serializeMetadata() (uint64, []byte, error) {
	var sb strings.Builder
	sb.WriteString("{")

	first := true
	for key, value := range gw.metadata {
		if !first {
			sb.WriteString(",")
		}
		first = false

		sb.WriteString(fmt.Sprintf(`"%s":`, key))

		switch v := value.(type) {
		case string:
			sb.WriteString(fmt.Sprintf(`"%s"`, v))
		case int:
			sb.WriteString(fmt.Sprintf("%d", v))
		case int64:
			sb.WriteString(fmt.Sprintf("%d", v))
		case uint64:
			sb.WriteString(fmt.Sprintf("%d", v))
		case float32:
			sb.WriteString(fmt.Sprintf("%f", v))
		case float64:
			sb.WriteString(fmt.Sprintf("%f", v))
		case bool:
			sb.WriteString(fmt.Sprintf("%t", v))
		default:
			sb.WriteString(fmt.Sprintf(`"%v"`, v))
		}
	}

	sb.WriteString("}")
	return uint64(sb.Len()), []byte(sb.String()), nil
}

type mergedTensors map[string]struct {
	shape []uint64
	dtype safetensors.DType
	data  []byte
}

func findShardedFiles(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return nil, err
	}

	var shards []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".safetensors") {
			shards = append(shards, entry.Name())
		}
	}

	if len(shards) <= 1 {
		return nil, nil
	}

	sort.Slice(shards, func(i, j int) bool {
		return extractShardNum(shards[i]) < extractShardNum(shards[j])
	})

	return shards, nil
}

func extractShardNum(filename string) int {
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

func mergeShardedTensors(dirPath string, shardFiles []string) (mergedTensors, error) {
	merged := make(mergedTensors)

	for _, shardFile := range shardFiles {
		shardPath := filepath.Join(dirPath, shardFile)
		data, err := os.ReadFile(shardPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read shard %s: %w", shardFile, err)
		}

		st, err := safetensors.Deserialize(data)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize shard %s: %w", shardFile, err)
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

	return merged, nil
}

func convertSafetensorsToGGUF(inputPath, outputPath string) error {
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("failed to stat input: %w", err)
	}

	var tensors mergedTensors
	var modelName string

	if info.IsDir() {
		shardFiles, err := findShardedFiles(inputPath)
		if err != nil {
			return fmt.Errorf("failed to find sharded files: %w", err)
		}

		if len(shardFiles) > 1 {
			fmt.Printf("Detected %d sharded files, merging...\n", len(shardFiles))
			tensors, err = mergeShardedTensors(inputPath, shardFiles)
			if err != nil {
				return fmt.Errorf("failed to merge shards: %w", err)
			}
			modelName = filepath.Base(inputPath)
		} else {
			entries, err := os.ReadDir(inputPath)
			if err != nil {
				return fmt.Errorf("failed to read directory: %w", err)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".safetensors") {
					shardPath := filepath.Join(inputPath, entry.Name())
					data, err := os.ReadFile(shardPath)
					if err != nil {
						return fmt.Errorf("failed to read file: %w", err)
					}
					st, err := safetensors.Deserialize(data)
					if err != nil {
						return fmt.Errorf("failed to deserialize: %w", err)
					}
					tensors = make(mergedTensors)
					for _, t := range st.Tensors() {
						tensors[t.Name] = struct {
							shape []uint64
							dtype safetensors.DType
							data  []byte
						}{
							shape: t.TensorView.Shape(),
							dtype: t.TensorView.DType(),
							data:  t.TensorView.Data(),
						}
					}
					modelName = strings.TrimSuffix(entry.Name(), ".safetensors")
					break
				}
			}
		}
	} else {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}

		st, err := safetensors.Deserialize(data)
		if err != nil {
			return fmt.Errorf("failed to deserialize safetensors: %w", err)
		}

		tensors = make(mergedTensors)
		for _, t := range st.Tensors() {
			tensors[t.Name] = struct {
				shape []uint64
				dtype safetensors.DType
				data  []byte
			}{
				shape: t.TensorView.Shape(),
				dtype: t.TensorView.DType(),
				data:  t.TensorView.Data(),
			}
		}
		modelName = filepath.Base(inputPath)
	}

	if tensors == nil || len(tensors) == 0 {
		return fmt.Errorf("no tensors found")
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	writer := newGGUFWriter(outputFile)

	writer.SetMetadata("general.architecture", "llama")
	writer.SetMetadata("general.name", modelName)
	writer.SetMetadata("general.file_type", int(GGMLTypeF16))

	for name, t := range tensors {
		dtype := safetensorsDTypeToGGML(t.dtype)
		writer.AddTensor(name, t.shape, dtype, t.data)
	}

	if err := writer.Write(); err != nil {
		return fmt.Errorf("failed to write GGUF: %w", err)
	}

	return nil
}

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert safetensors to GGUF format",
	Long:  "Convert a .safetensors file to GGUF format",
	RunE: func(cmd *cobra.Command, args []string) error {
		inputFile, err := cmd.Flags().GetString("input")
		if err != nil {
			return err
		}
		outputFile, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		if inputFile == "" {
			return fmt.Errorf("input file is required")
		}
		if outputFile == "" {
			ext := filepath.Ext(inputFile)
			outputFile = strings.TrimSuffix(inputFile, ext) + ".gguf"
		}

		fmt.Printf("Converting %s to %s...\n", inputFile, outputFile)

		if err := convertSafetensorsToGGUF(inputFile, outputFile); err != nil {
			return err
		}

		fmt.Println("Conversion complete!")
		return nil
	},
}

var ggufCmd = &cobra.Command{
	Use:   "gguf",
	Short: "GGUF file operations",
	Long:  "Tools for working with GGUF files including conversion from safetensors",
}

var convertDirCmd = &cobra.Command{
	Use:   "convert-dir",
	Short: "Convert a directory of safetensors files to GGUF",
	Long:  "Convert all .safetensors files in a directory to GGUF format",
	RunE: func(cmd *cobra.Command, args []string) error {
		inputDir, err := cmd.Flags().GetString("input")
		if err != nil {
			return err
		}
		outputDir, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		if inputDir == "" {
			return fmt.Errorf("input directory is required")
		}
		if outputDir == "" {
			outputDir = inputDir
		}

		entries, err := os.ReadDir(inputDir)
		if err != nil {
			return fmt.Errorf("failed to read input directory: %w", err)
		}

		var converted int
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".safetensors") {
				inputPath := filepath.Join(inputDir, entry.Name())
				outputPath := filepath.Join(outputDir, strings.TrimSuffix(entry.Name(), ".safetensors")+".gguf")

				fmt.Printf("Converting %s to %s...\n", inputPath, outputPath)

				if err := convertSafetensorsToGGUF(inputPath, outputPath); err != nil {
					fmt.Printf("Error converting %s: %v\n", entry.Name(), err)
					continue
				}
				converted++
			}
		}

		fmt.Printf("Conversion complete! Converted %d files.\n", converted)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ggufCmd)
	convertCmd.Flags().StringP("input", "i", "", "Input safetensors file")
	convertCmd.Flags().StringP("output", "o", "", "Output GGUF file (default: <input>.gguf)")

	convertDirCmd.Flags().StringP("input", "i", "", "Input directory containing safetensors files")
	convertDirCmd.Flags().StringP("output", "o", "", "Output directory for GGUF files (default: same as input)")

	ggufCmd.AddCommand(convertCmd)
	ggufCmd.AddCommand(convertDirCmd)
}
