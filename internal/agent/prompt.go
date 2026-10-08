package agent

import _ "embed"

// SystemPrompt is the global agent system prompt, embedded in the binary
// (Q-AGENT-8). It is the single place to change the agent's instructions: the
// product owner's text replaces the contents of system_prompt.txt.
//
//go:embed system_prompt.txt
var SystemPrompt string
