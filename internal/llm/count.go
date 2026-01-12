package llm

import (
	"github.com/pkoukk/tiktoken-go"
)

func CountToken(text string) int {
	if text == "" {
		return 0
	}

	// Use cl100k_base encoding (GPT-4, GPT-3.5-turbo)
	encoder, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0
	}

	// Encode the text and return the token count
	tokens := encoder.Encode(text, nil, nil)
	return len(tokens)
}
