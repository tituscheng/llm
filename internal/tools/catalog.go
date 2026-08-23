package tools

import (
	"fmt"
	"io/fs"
)

// ExtraFile is a sidecar copied next to an installed binary.
type ExtraFile struct {
	Name string
	Data []byte
}

// ExtraDir is a directory tree copied next to an installed binary.
type ExtraDir struct {
	Name string
	FS   fs.FS
	Root string
}

// Category groups embedded inference binaries by modality.
type Category struct {
	ID          string
	Name        string
	Description string
	Binaries    []Binary
}

// Binary is an embedded executable that can be copied to the working directory.
type Binary struct {
	Name       string
	Version    string
	Data       []byte
	Note       string
	ExtraFiles []ExtraFile
	ExtraDirs  []ExtraDir
}

// Categories returns the embedded binaries grouped for selection.
func Categories() []Category {
	return []Category{
		{
			ID:          "text",
			Name:        "Text",
			Description: "llama.cpp text inference",
			Binaries:    TextBinaries(),
		},
		{
			ID:          "image",
			Name:        "Image",
			Description: "stable-diffusion.cpp image generation",
			Binaries:    ImageBinaries(),
		},
		{
			ID:          "audio",
			Name:        "Audio",
			Description: "speech and audio inference",
			Binaries:    AudioBinaries(),
		},
	}
}

// TextBinaries returns the embedded Apple Silicon text tools.
func TextBinaries() []Binary {
	return []Binary{
		{Name: "llama-server", Version: LlamaServerVersion, Data: LlamaServerBinary},
	}
}

// ImageBinaries returns the embedded Apple Silicon image tools.
func ImageBinaries() []Binary {
	return []Binary{
		{Name: "sd-cli", Version: SDCLIVersion, Data: SDCLIBinary},
	}
}

// AudioBinaries returns the embedded Apple Silicon audio tools.
func AudioBinaries() []Binary {
	return []Binary{
		{
			Name:    "piper",
			Version: PiperVersion,
			Data:    PiperBinary,
			Note:    "also copies ONNX Runtime and espeak-ng-data",
			ExtraFiles: []ExtraFile{
				{Name: "libonnxruntime.1.22.0.dylib", Data: PiperONNXRuntime},
			},
			ExtraDirs: []ExtraDir{
				{
					Name: "espeak-ng-data",
					FS:   PiperEspeakData,
					Root: "piper-v1.7.0/espeak-ng-data",
				},
			},
		},
		{Name: "bark.cpp", Version: BarkVersion, Data: BarkBinary},
		{Name: "tts.cpp", Version: TTSCPPVersion, Data: TTSCPPBinary},
		{Name: "llama-tts", Version: LlamaTTSVersion, Data: LlamaTTSBinary},
		{Name: "audio.cpp", Version: AudioCPPVersion, Data: AudioCPPBinary},
	}
}

// CategoryByID returns the category with the given ID.
func CategoryByID(id string) (Category, bool) {
	for _, cat := range Categories() {
		if cat.ID == id {
			return cat, true
		}
	}
	return Category{}, false
}

// Find returns the binary with the given name in the category.
func (c Category) Find(name string) (Binary, bool) {
	for _, bin := range c.Binaries {
		if bin.Name == name {
			return bin, true
		}
	}
	return Binary{}, false
}

// Size returns the total bytes that would be written for the binary and sidecars.
func (b Binary) Size() int64 {
	n := int64(len(b.Data))
	for _, f := range b.ExtraFiles {
		n += int64(len(f.Data))
	}
	for _, d := range b.ExtraDirs {
		n += dirSize(d.FS, d.Root)
	}
	return n
}

// FormatSize renders n as a human-readable byte count.
func FormatSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	}
}

func dirSize(fsys fs.FS, root string) int64 {
	var n int64
	_ = fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		n += info.Size()
		return nil
	})
	return n
}
