// Package ai provides a minimal client for OpenAI-compatible chat
// completion APIs. The base URL is configurable, so any compatible
// provider (OpenAI, local llama.cpp, Ollama, vLLM, ...) can be used.
package ai

import "context"

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single entry of a chat completion conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Client is the abstraction the tutor uses to talk to a language model.
type Client interface {
	// Complete sends the conversation and returns the assistant reply.
	Complete(ctx context.Context, messages []Message) (string, error)
}
