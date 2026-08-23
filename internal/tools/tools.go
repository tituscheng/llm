package tools

import "embed"

const LlamaServerVersion = "v0.2.0 (bb4caa7, 2026-08-21)"

//go:embed llama-v0.2.0/llama-server
var LlamaServerBinary []byte

const SDCLIVersion = "master-827-97d2990 (97d2990, 2026-08-19)"

//go:embed sd-master-827-97d2990/sd-cli
var SDCLIBinary []byte

const PiperVersion = "v1.7.0 (b275826, 2026-08-22)"

//go:embed piper-v1.7.0/piper
var PiperBinary []byte

//go:embed piper-v1.7.0/libonnxruntime.1.22.0
var PiperONNXRuntime []byte

//go:embed piper-v1.7.0/espeak-ng-data
var PiperEspeakData embed.FS

const BarkVersion = "master-5d5be84 (5d5be84, 2024-11-16)"

//go:embed bark-5d5be84/bark.cpp
var BarkBinary []byte

const TTSCPPVersion = "master-c04c77a (c04c77a, 2025-10-05)"

//go:embed tts-c04c77a/tts.cpp
var TTSCPPBinary []byte

const LlamaTTSVersion = "b10588 (70adb1b, 2026-08-23)"

//go:embed llama-tts-b10588/llama-tts
var LlamaTTSBinary []byte

const AudioCPPVersion = "master-62735ea (62735ea, 2026-08-22)"

//go:embed audio.cpp-62735ea/audio.cpp
var AudioCPPBinary []byte
