package tools

import _ "embed"

const LlamaServerVersion = "v0.2.0 (bb4caa7, 2026-08-21)"

//go:embed llama-v0.2.0/llama-server
var LlamaServerBinary []byte

const SDCLIVersion = "master-827-97d2990 (97d2990, 2026-08-19)"

//go:embed sd-master-827-97d2990/sd-cli
var SDCLIBinary []byte
