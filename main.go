package main

import (
	"runtime"

	"llm/cmd"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	cmd.Execute()
}
