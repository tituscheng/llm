package install

// Category groups inference tools by modality.
type Category struct {
	ID          string
	Name        string
	Description string
	Tools       []Tool
}

// Tool is an inference executable compiled from the latest upstream source.
type Tool struct {
	Name string
	Note string
}

// Categories returns the tools grouped for selection.
func Categories() []Category {
	return []Category{
		{
			ID:          "text",
			Name:        "Text",
			Description: "llama.cpp text inference",
			Tools: []Tool{
				{Name: "llama-server"},
			},
		},
		{
			ID:          "image",
			Name:        "Image",
			Description: "stable-diffusion.cpp image generation",
			Tools: []Tool{
				{Name: "sd-cli"},
			},
		},
		{
			ID:          "audio",
			Name:        "Audio",
			Description: "speech and audio inference",
			Tools: []Tool{
				{Name: "piper", Note: "also installs ONNX Runtime and espeak-ng-data"},
				{Name: "bark.cpp"},
				{Name: "tts.cpp"},
				{Name: "llama-tts"},
				{Name: "audio.cpp"},
			},
		},
	}
}

// Tools returns every tool in category order.
func Tools() []Tool {
	cats := Categories()
	tools := make([]Tool, 0, 8)
	for _, cat := range cats {
		tools = append(tools, cat.Tools...)
	}
	return tools
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

// Find returns the tool with the given name in the category.
func (c Category) Find(name string) (Tool, bool) {
	for _, tool := range c.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}
