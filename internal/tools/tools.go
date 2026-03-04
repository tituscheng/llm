package tools

import _ "embed"

const LlamaServerVersion = "e877ad8 (2026-02-22)"

//go:embed llama-e877ad8/llama-server
var LlamaServerBinary []byte
