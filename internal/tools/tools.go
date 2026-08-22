package tools

import _ "embed"

const LlamaServerVersion = "v0.2.0 (bb4caa7, 2026-08-21)"

//go:embed llama-v0.2.0/llama-server
var LlamaServerBinary []byte
