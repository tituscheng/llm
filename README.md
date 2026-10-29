# llm

Command-line tool for compiling local inference binaries on Apple Silicon and for converting model weights between safetensors and GGUF.

```bash
llm text download
llm image download
llm audio download piper
llm gui
```

Each download command clones the latest upstream source, builds a release binary, and installs it into `~/.llm/tools`. Press Ctrl+C to stop a build. The temporary source tree is removed when the build finishes or is cancelled.

## Getting Started

Compiling tools requires macOS on Apple Silicon (`darwin/arm64`), Go 1.25.10 or newer, `git`, `cmake`, and `bash`. `ninja` is optional and is used when it is on `PATH`.

The `gguf` and `safetensors` commands do not compile native code. They run anywhere this module builds.

Install the `llm` command into `GOBIN` (usually `~/go/bin`):

```bash
task install
```

Or, without [Task](https://taskfile.dev):

```bash
go install ./cmd/llm
```

Confirm the binary is on your `PATH`:

```bash
llm --help
```

### Compile llama-server

```bash
llm text download
```

When the build finishes, the binary is at:

```text
~/.llm/tools/llama-server
```

`llm audio download` with no argument prints a numbered list when stdin is a terminal. Pass the tool name to skip the prompt:

```bash
llm audio download piper
```

## Features

### Inference tools

Download commands compile from the latest upstream tag when the repository publishes one, and from the default branch otherwise. `bark.cpp` and `tts.cpp` always build the default branch. `llama-tts` tracks the latest llama.cpp `b*` build tag.

| Command | Tool | Upstream | Installed files |
| --- | --- | --- | --- |
| `llm text download` | `llama-server` | [ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp) | `llama-server` |
| `llm image download` | `sd-cli` | [leejet/stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) | `sd-cli` |
| `llm audio download piper` | `piper` | [OHF-Voice/piper1-gpl](https://github.com/OHF-Voice/piper1-gpl) | `piper`, ONNX Runtime, `espeak-ng-data/` |
| `llm audio download bark.cpp` | `bark.cpp` | [PABannier/bark.cpp](https://github.com/PABannier/bark.cpp) | `bark.cpp` |
| `llm audio download tts.cpp` | `tts.cpp` | [mmwillet/TTS.cpp](https://github.com/mmwillet/TTS.cpp) | `tts.cpp` |
| `llm audio download llama-tts` | `llama-tts` | [ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp) | `llama-tts` |
| `llm audio download audio.cpp` | `audio.cpp` | [0xShug0/audio.cpp](https://github.com/0xShug0/audio.cpp) | `audio.cpp` |

Text and image builds enable Metal. A rebuild replaces the files already in `~/.llm/tools`.

`llm gui` opens a window for the same three categories and runs the selected compile, with the build log in the window.

### Safetensors

Merge every `.safetensors` file in a directory into one file. Shard names are ordered by the number embedded in the filename. Duplicate tensor names keep the first shard's data.

```bash
llm safetensors merge
llm safetensors merge --input ./shards --output model.safetensors
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--input`, `-i` | `.` | Directory of `.safetensors` files |
| `--output`, `-o` | `merged.safetensors` | Output file |

### GGUF

Convert safetensors tensors to GGUF. Metadata is written with architecture `llama`, and each dtype is mapped to the matching GGML type.

```bash
llm gguf convert --input model.safetensors
llm gguf convert --input model.safetensors --output model.gguf
llm gguf convert-dir --input ./shards --output ./gguf
```

| Command | Flag | Default | Meaning |
| --- | --- | --- | --- |
| `convert` | `--input`, `-i` | required | One `.safetensors` file |
| `convert` | `--output`, `-o` | `<input>.gguf` | Output GGUF file |
| `convert-dir` | `--input`, `-i` | required | Directory of `.safetensors` files |
| `convert-dir` | `--output`, `-o` | same as `--input` | Directory for the GGUF files |

`llm chat` and `llm pull` are registered commands with no behavior yet.

## Use in a Go project

The importable package is `llm/install`. It compiles the same tools as the download commands and writes them to `~/.llm/tools`. A call to `Install` needs the same machine and tools as the CLI: Apple Silicon, `git`, `cmake`, `bash`, and network access to clone upstream source.

The module path is `llm`, so a consumer points `go.mod` at this checkout with a `replace` directive. In the other module:

```bash
go mod init example.com/myapp
```

```go
module example.com/myapp

go 1.25.10

require llm v0.0.0

replace llm => ../llm
```

Use the relative or absolute path of this repository. Then:

```bash
go mod tidy
```

A program in that module compiles one tool and stops when the process is interrupted:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"llm/install"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	installed, err := install.IsInstalled("llama-server")
	if err != nil {
		log.Fatal(err)
	}
	if installed {
		return
	}

	err = install.Install(ctx, "llama-server", os.Stdout, os.Stderr)
	if err != nil {
		log.Fatal(err)
	}
}
```

`IsInstalled` reports whether the tool's executable is already in `~/.llm/tools`. It does not create that directory. For `piper`, the check also requires an `espeak-ng-data` directory and a `libonnxruntime` dylib next to the executable. `Install` still compiles when the tool is already present, so call `IsInstalled` first when a rebuild should be skipped.

`Install` creates `~/.llm/tools` when that directory is missing. Pass `nil` for `stdout` or `stderr` to discard that stream. The `name` argument is one of `llama-server`, `sd-cli`, `piper`, `bark.cpp`, `tts.cpp`, `llama-tts`, or `audio.cpp`.

`Install` returns an error when `ctx` is nil, when `name` is unknown, when the process is not `darwin/arm64`, or when the compile script fails. Cancelling `ctx` stops the build and removes the temporary source tree.

### Hugging Face models

Import `llm/huggingface` to search the Hub and download a model repository. `Search` returns up to 20 matches. A query can be free text or a repository id such as `LiquidAI/LFM3-1.7B`.

```go
models, err := huggingface.Search(ctx, "LiquidAI/LFM3-1.7B")
if err != nil {
	log.Fatal(err)
}

var model huggingface.Model
for _, candidate := range models {
	if candidate.ID == "LiquidAI/LFM3-1.7B" {
		model = candidate
		break
	}
}
if model.ID == "" {
	log.Fatal("model not found")
}

downloaded, err := model.IsDownloaded(ctx, "")
if err != nil {
	log.Fatal(err)
}
if downloaded {
	return
}

dir, err := model.Download(ctx, "")
if err != nil {
	log.Fatal(err)
}
log.Println(dir)
```

`IsDownloaded` reports whether every file is already in `~/.llm/models/LiquidAI/LFM3-1.7B` at the size published by the Hub. It does not create that directory. A missing file or a file of the wrong size makes it return false.

`Download` fetches every file at revision `main`, including the weight files, into that directory. Pass a directory to choose another location. A file that is already present with the expected size is left in place, so a cancelled download can be continued by calling `Download` again. Cancelling `ctx` stops the transfer.

`Model.Revision` selects a branch, tag, or commit. `HF_TOKEN`, or `HUGGING_FACE_HUB_TOKEN` when that is unset, is sent for gated repositories.

### Run a server and send a prompt

Import `llm/serve` to run `llama-server` and send a chat prompt. The model argument is a GGUF file, such as one file from `LiquidAI/LFM3-1.7B-GGUF`. `llama-server` must already be installed, or pass its path in `Options.Bin`.

```go
srv, err := serve.Start(ctx, serve.Options{
	Model: "/path/to/model.gguf",
})
if err != nil {
	log.Fatal(err)
}
defer srv.Stop()

alive, err := srv.IsAlive(ctx)
if err != nil {
	log.Fatal(err)
}
if !alive {
	log.Fatal("server is not ready")
}

reply, err := srv.Chat(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
})
if err != nil {
	log.Fatal(err)
}
log.Println(reply)

for token, err := range srv.ChatStream(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
}) {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(token)
}
```

`Start` listens on `127.0.0.1` and returns after `/health` says the model is ready. Cancelling `ctx` stops the process. `Stop` kills the server and its child processes, and a second call does nothing. `Close` calls `Stop`. `IsAlive` returns false when the process has exited or the model is still loading. `Chat` posts the messages to `/v1/chat/completions` and returns the assistant text. `ChatStream` sets `stream` and yields each token from the server-sent events. Ending the loop stops that request. Both remove a `<think>` trace from the assistant text. `ChatWith` returns that trace in `Reply.Reasoning` when the server left it in the content. `Reasoning.Format` of `none` or `deepseek-legacy` keeps the tags. Set `Reasoning.Strip` to force either behavior.

`Options.Parallel` is passed as `--parallel`. `0` leaves llama-server's default. A 4-bit 12B model uses `1` so the process keeps a single KV cache slot. `Options.ReasoningBudget` is passed as `--reasoning-budget`. `0` leaves the server default. Qwen3.5-4B uses `1024`. Per-request reasoning format and `enable_thinking` stay on the chat request. `Options.NoMMProj` passes `--no-mmproj`, so a model file does not load a projector. `Options.Alias` is passed as `--alias` when `Start` launches the process.

`Start` with a fixed `Port` uses a llama-server that is already listening there when `/health` is ready and `/v1/models` lists this model. The id matches `Options.Alias`, the model path, or the file name. `Stop` unloads that model and stops the process. A different listener on the port returns an error. `Port` of `0` starts a new process. Cancelling the context passed to `Start` stops a process `Start` launched. `StartRouter` reuses a listener whose model list is empty.

`ChatWith` posts that request with reasoning options. `Reasoning.Effort` is sent as `reasoning_effort`. `EffortNone` disables thinking. The other `Effort` constants are the levels llama-server documents, and any other single token is passed through to the chat template. `Reasoning.Format` is `none`, `auto`, `deepseek`, or `deepseek-legacy`. `Reasoning.EnableThinking` sets `chat_template_kwargs.enable_thinking`. `Reply.Reasoning` carries `reasoning_content` when the server parses thoughts into that field.

`Stream` uses the same options and `Deltas` yields assistant text on `Content` and thinking text on `Reasoning`. `Reasoning.Control` arms `/v1/chat/completions/control` for that completion. `EndReasoning` sends action `reasoning_end`, which ends the thinking block so the model continues with the answer. Call it while ranging `Deltas`. It waits until the completion id has been read, and it can be called from another goroutine. `Close` stops the request.

`Sampling` sets the token limits on that same request. `Chat`, `ChatWith`, `ChatStream`, `Stream`, and `Keep` each take one `Sampling` value after their other arguments. `Temperature`, `TopK`, `TopP`, `MinP`, and `MaxTokens` are sent as `temperature`, `top_k`, `top_p`, `min_p`, and `max_tokens`. A nil field is omitted, so llama-server keeps its default. A non-nil field is sent, including zero. `TopK` of 0 disables that limit. `TopK` of 1 keeps only the most probable token, which is greedy sampling. A temperature of 0 is sent as 0. `TopP` of 1 disables that limit. `MinP` of 0 disables that limit. `MaxTokens` of -1 means no token limit, and 0 generates no tokens.

`Keep` sends the same streaming request with the header `X-Conversation-Id`. After `Close`, llama-server continues that generation. `Stream` leaves the header unset, and the server cancels the generation when the request ends. `Reuse` reads `GET /v1/stream` from a byte offset. Pass `Stream.Offset` so the next read starts on a line boundary. `Offset` counts each SSE line the parser has consumed, including its trailing newline. A blank line after `data: [DONE]` can stay unread, and reading it again yields no token. `Reuse` strips `<think>` traces from the bytes it receives. A think block that opened before that offset can appear in the content, because the resumed stream starts a new scan. `Shutdown` sends `DELETE /v1/stream` and cancels the kept generation. The call succeeds when that id is already gone. `Lookup` posts conversation ids to `/v1/streams/lookup`. An id also matches a session stored as `<id>::<model>`. An empty list returns no sessions and makes no request. A kept id holds the generation until `Shutdown`, or until llama-server discards the session.

`StartRouter` launches llama-server with no model loaded. That is the process the web UI uses to download weights. `Download` posts a Hugging Face tag, such as `ggml-org/gemma-3-4b-it-GGUF:Q4_K_M`, to `/models`. A nil sidecar list omits the field, and llama-server fetches `mmproj` when the repository has one. An empty list sends `[]` and downloads the model with no projector and no draft. `DownloadEvents` reads `/models/sse`. A `download_progress` event lists each weight file with the bytes received and the file size. `PauseDownload` keeps the partial files. `RemoveDownload` deletes them. `Models` returns the same list the web UI shows, including a download that is still in progress.

`WithModel` chooses which loaded model answers. The router reads the `model` field on `/v1/chat/completions` and forwards the request to that model. The name is the same tag passed to `Download`, such as `ggml-org/gemma-4-12B-it-GGUF:Q4_K_M` or `unsloth/Qwen3.5-4B-GGUF:Q4_K_M`. Cover and Application each keep a handle, so a router with both models loaded can answer from either one. `EndReasoning` sends that name on `/v1/chat/completions/control` so the router reaches the instance that is generating. An empty name omits the field. A server started with one GGUF file leaves it unset. A name that contains a newline is rejected. Stop on either handle stops the process.

```go
off := false
reply, err := srv.ChatWith(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
}, serve.Reasoning{
	Effort:         serve.EffortNone,
	EnableThinking: &off,
})
if err != nil {
	log.Fatal(err)
}
log.Println(reply.Content)
log.Println(reply.Reasoning)

stream, err := srv.Stream(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
}, serve.Reasoning{Control: true, Effort: serve.EffortHigh})
if err != nil {
	log.Fatal(err)
}
defer stream.Close()

var ended bool
for delta, err := range stream.Deltas() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(delta.Reasoning)
	fmt.Print(delta.Content)
	if delta.Reasoning != "" && !ended {
		ended = true
		if err := stream.EndReasoning(ctx); err != nil {
			log.Fatal(err)
		}
	}
}
```

`Sampling` is optional. These limits ask for at most 128 tokens, chosen from the 20 most probable.

```go
topK := 20
maxTokens := 128
reply, err := srv.ChatWith(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
}, serve.Reasoning{}, serve.Sampling{
	TopK:      &topK,
	MaxTokens: &maxTokens,
})
if err != nil {
	log.Fatal(err)
}
log.Println(reply.Content)
```

`Keep` uses an id you choose. `Close` drops the socket. `Reuse` continues from `Offset`, and `Shutdown` cancels the retained generation.

```go
const id = "chat-1"
stream, err := srv.Keep(ctx, id, []serve.Message{
	{Role: "user", Content: "Hello"},
}, serve.Reasoning{})
if err != nil {
	log.Fatal(err)
}
for delta, err := range stream.Deltas() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(delta.Content)
}
from := stream.Offset()
stream.Close()

again, err := srv.Reuse(ctx, id, from)
if err != nil {
	log.Fatal(err)
}
for delta, err := range again.Deltas() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(delta.Content)
}
again.Close()
if err := srv.Shutdown(ctx, id); err != nil {
	log.Fatal(err)
}
```

`StartRouter` is separate from a server that already has a GGUF loaded.

```go
router, err := serve.StartRouter(ctx, serve.Options{})
if err != nil {
	log.Fatal(err)
}
defer router.Stop()

const model = "ggml-org/gemma-3-4b-it-GGUF:Q4_K_M"
if err := router.Download(ctx, model, nil); err != nil {
	log.Fatal(err)
}
for event, err := range router.DownloadEvents(ctx) {
	if err != nil {
		log.Fatal(err)
	}
	for _, weight := range event.Weights {
		log.Printf("%s %d/%d", weight.URL, weight.Done, weight.Total)
	}
	if event.Event == "download_finished" {
		break
	}
}
```

Cover and Application each name the model that answers.

```go
cover, err := router.WithModel("ggml-org/gemma-4-12B-it-GGUF:Q4_K_M")
if err != nil {
	log.Fatal(err)
}
application, err := router.WithModel("unsloth/Qwen3.5-4B-GGUF:Q4_K_M")
if err != nil {
	log.Fatal(err)
}
reply, err := cover.Chat(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
})
if err != nil {
	log.Fatal(err)
}
log.Println(reply.Content)
reply, err = application.Chat(ctx, []serve.Message{
	{Role: "user", Content: "Hello"},
})
if err != nil {
	log.Fatal(err)
}
log.Println(reply.Content)
```

### Grok

Import `llm/grok` to call the Grok chat completions API at `https://api.x.ai/v1`. `New` takes the API key and sends it as a bearer token. The key is the value of `XAI_API_KEY` from the xAI console. `New` uses the model `grok-4.7`. `WithModel` selects another model id.

```go
maxTokens := 400
client, err := grok.New(os.Getenv("XAI_API_KEY"))
if err != nil {
	log.Fatal(err)
}

reply, err := client.Chat(ctx, grok.Turn{
	Messages: []grok.Message{
		{Role: "user", Content: "Hello"},
	},
	Effort: grok.EffortLow,
}, grok.Sampling{MaxTokens: &maxTokens})
if err != nil {
	log.Fatal(err)
}
log.Println(reply.Content)

for event, err := range client.ChatStream(ctx, grok.Turn{
	Messages: []grok.Message{
		{Role: "user", Content: "Hello"},
	},
	Effort: grok.EffortHigh,
}) {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(event.Text)
	if event.Reply != nil {
		log.Println(event.Reply.ID)
	}
}
```

`Chat` posts the turn to `https://api.x.ai/v1/chat/completions` and returns the assistant text in `Reply.Content`. `Reply.ID` is the completion id. Roles are `system`, `user`, and `assistant`. `Chat` and `ChatStream` remove a `<think>` trace from that text. A trace split across stream deltas is removed as well. The text inside the trace is dropped.

`Turn.Effort` is sent as `reasoning_effort`. Empty leaves the API default. `EffortLow`, `EffortHigh`, and `EffortXHigh` are the accepted values.

`Sampling` sets the limits on that request. `Chat` and `ChatStream` each take one `Sampling` value. `Temperature`, `TopP`, and `MaxTokens` are sent as `temperature`, `top_p`, and `max_completion_tokens`. `MaxTokens` caps the visible answer. A nil field is omitted. A non-nil field is sent, including zero. A negative `MaxTokens` is rejected.

`ChatStream` sets `stream` and yields each assistant text delta on `Event.Text`. The last event has an empty `Text` and the completed response on `Event.Reply`. Ending the loop stops that request. The client does not set its own timeout. Cancelling `ctx` stops the call.

A response other than 200 is a `StatusError`. `Status` is the HTTP status code. A caller uses `401` and `403` for a rejected key, `402` for missing credits, and `429` for a rate limit.

`New` rejects an empty key. `Chat` and `ChatStream` reject a nil context before they send the request. `WithBaseURL` points the client at another absolute http or https URL, which is how a test server stands in for `api.x.ai`.

To choose a tool from the catalog:

```go
cat, ok := install.CategoryByID("audio")
if !ok {
	log.Fatal("audio category is missing")
}
for _, tool := range cat.Tools {
	log.Printf("%s %s", tool.Name, tool.Note)
}

dir, err := install.ToolsDir()
if err != nil {
	log.Fatal(err)
}
log.Println(dir)
```

`ToolsDir` returns `$HOME/.llm/tools` and does not create it. `Categories` returns the text, image, and audio groups. `Tools` returns every tool in that order. `Category.Find` looks up one name inside a category.

Code inside this repository imports `llm/install` directly and does not need a `replace` line. Packages under `internal/` stay inside this module.

## Contributing

From the repository root:

```bash
go test ./...
go install ./cmd/llm
```

`task install` is the same install step. A compile needs network access so the install script can clone upstream source.

## License

This repository does not include a license file.
