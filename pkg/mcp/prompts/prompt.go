package prompts

// Prompt interface for MCP prompts.
//
// A prompt is a named, user-invocable piece of text a client can surface as a
// command. Unlike a tool, the server does not act on it: it returns messages
// for the model to follow.
type Prompt interface {
	Definition() PromptDefinition
	Get(args map[string]string) (GetResult, error)
}

// PromptDefinition describes an MCP prompt as returned by prompts/list.
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes one argument a prompt accepts.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// GetResult is the prompts/get response payload.
type GetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage is a single message in a prompt's result.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent holds the message body. Only text content is supported.
type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// textMessage builds the single-user-message shape every prompt here returns.
func textMessage(text string) []PromptMessage {
	return []PromptMessage{{
		Role:    "user",
		Content: PromptContent{Type: "text", Text: text},
	}}
}
