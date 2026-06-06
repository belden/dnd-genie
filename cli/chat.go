package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tauliang/DnDGenie/cli/config"
)

const (
	dndChatSystemPrompt      = "You are DnDGenie, a concise tabletop RPG assistant. Help the user prepare and run Dungeons & Dragons at the table. Prefer practical, table-ready answers."
	emptyChatResponseMessage = "The chat model returned an empty answer. Try a different chat model, or confirm the model is loaded in your local model server."
	defaultChatMaxTokens     = 2048
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatClient interface {
	Send(ctx context.Context, messages []chatMessage) (string, error)
}

type chatClientFactory func(conf config.Config) (chatClient, error)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type openAIChatClient struct {
	endpoint string
	model    string
	doer     httpDoer
}

type ollamaChatClient struct {
	endpoint string
	model    string
	doer     httpDoer
}

func newChatClient(conf config.Config) (chatClient, error) {
	if conf.Provider == "" || conf.Endpoint == "" {
		return nil, errUsage("no model endpoint configured; run /connect lmstudio first")
	}
	if conf.ChatModel == "" {
		return nil, errUsage("no chat model configured; run models --chat MODEL first")
	}

	doer := &http.Client{Timeout: 5 * time.Minute}
	switch conf.Provider {
	case providerLMStudio:
		return openAIChatClient{
			endpoint: strings.TrimRight(conf.Endpoint, "/") + "/chat/completions",
			model:    conf.ChatModel,
			doer:     doer,
		}, nil
	case providerOllama:
		return ollamaChatClient{
			endpoint: strings.TrimRight(conf.Endpoint, "/") + "/api/chat",
			model:    conf.ChatModel,
			doer:     doer,
		}, nil
	default:
		return nil, errUsage("provider must be lmstudio or ollama")
	}
}

func (c openAIChatClient) Send(ctx context.Context, messages []chatMessage) (string, error) {
	requestBody := struct {
		Model       string        `json:"model"`
		Messages    []chatMessage `json:"messages"`
		Stream      bool          `json:"stream"`
		Temperature float64       `json:"temperature"`
		MaxTokens   int           `json:"max_tokens"`
	}{
		Model:       c.model,
		Messages:    messages,
		Stream:      false,
		Temperature: 0.4,
		MaxTokens:   defaultChatMaxTokens,
	}

	var responseBody struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, c.doer, c.endpoint, requestBody, &responseBody); err != nil {
		return "", err
	}
	if len(responseBody.Choices) == 0 {
		return "", nil
	}
	return responseBody.Choices[0].Message.Content, nil
}

func (c ollamaChatClient) Send(ctx context.Context, messages []chatMessage) (string, error) {
	requestBody := struct {
		Model    string        `json:"model"`
		Messages []chatMessage `json:"messages"`
		Stream   bool          `json:"stream"`
	}{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}

	var responseBody struct {
		Message chatMessage `json:"message"`
	}
	if err := postJSON(ctx, c.doer, c.endpoint, requestBody, &responseBody); err != nil {
		return "", err
	}
	return responseBody.Message.Content, nil
}

func postJSON(ctx context.Context, doer httpDoer, endpoint string, requestBody any, responseBody any) error {
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer lm-studio")

	response, err := doer.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("chat request failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}

	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode chat response: %w", err)
	}
	return nil
}
