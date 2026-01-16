package prompt

import (
	_ "embed"
)

var (
	//go:embed pin_github_better.txt
	PinGithubBetter string

	//go:embed refer_to_github.txt
	ReferToGithub string

	//go:embed rust_template.txt
	RustTemplate string

	//go:embed ui_debug_template.txt
	UIDebugTemplate string

	//go:embed prompt_mentor.txt
	PromptMentorTemplate string
)
