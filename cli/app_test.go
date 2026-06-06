package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tauliang/DnDGenie/cli/config"
)

func newTestApp(t *testing.T, stdin string) (*App, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.json")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := NewApp(strings.NewReader(stdin), stdout, stderr, configPath)
	return app, stdout, stderr, configPath
}

// helper function, returns the last element of a slice
func last[T any](s []T) T {
	return s[len(s)-1]
}

type fakeChatClient struct {
	response string
	messages []chatMessage
	calls    int
}

func (f *fakeChatClient) Send(_ context.Context, messages []chatMessage) (string, error) {
	f.calls++
	f.messages = append([]chatMessage(nil), messages...)
	return f.response, nil
}

type doerFunc func(req *http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestConnectLMStudioNormalizesBareEndpoint(t *testing.T) {
	app, stdout, stderr, configPath := newTestApp(t, "")

	code := app.Run([]string{"/connect", "lmstudio", "--url", "http://127.0.0.1:1234"})
	assert.Equal(t, 0, code, stderr.String())

	conf, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, providerLMStudio, conf.Provider)
	assert.Equal(t, "http://127.0.0.1:1234/v1", conf.Endpoint)
	assert.Contains(t, stdout.String(), "Connected to lmstudio")
}

func TestConnectOllamaUsesDefaultEndpoint(t *testing.T) {
	app, _, stderr, configPath := newTestApp(t, "")

	code := app.Run([]string{"/connect", "ollama"})
	assert.Equal(t, 0, code, stderr.String())

	conf, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, defaultOllamaEndpoint, conf.Endpoint)
}

func TestModelsConfigureChatAndEmbeddingModels(t *testing.T) {
	app, stdout, stderr, configPath := newTestApp(t, "")

	code := app.Run([]string{"connect", "lmstudio"})
	assert.Equal(t, 0, code, "connect failed", stderr.String())

	stdout.Reset()
	code = app.Run([]string{"models", "--chat", "glm-5.0", "--embedding=text-embedding-nomic-embed-text-v1.5"})
	assert.Equal(t, 0, code, "models failed", stderr.String())

	conf, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "glm-5.0", conf.ChatModel)
	assert.Equal(t, "text-embedding-nomic-embed-text-v1.5", conf.EmbeddingModel)
	assert.Contains(t, stdout.String(), "Configured models")
}

func TestModelsPrintsExistingConfig(t *testing.T) {
	app, stdout, stderr, _ := newTestApp(t, "")

	code := app.Run([]string{"connect", "lmstudio"})
	assert.Equal(t, 0, code, "connect failed", stderr.String())

	code = app.Run([]string{"models", "--chat", "chatty", "--embedding", "embedder"})
	assert.Equal(t, 0, code, "models failed", stderr.String())
	stdout.Reset()

	code = app.Run([]string{"models"})
	assert.Equal(t, 0, code, "models print failed", stderr.String())

	output := stdout.String()
	assert.Contains(t, output, "Chat model: chatty", "stdout missing chat model")
	assert.Contains(t, output, "Embedding model: embedder", "stdout missing embedding model")
}

func TestInteractiveModeProcessesCommands(t *testing.T) {
	app, stdout, stderr, configPath := newTestApp(
		t,
		"/connect lmstudio --url http://localhost:1234\nmodels --chat chat --embedding embed\nstatus\n/quit\n",
	)

	code := app.Run(nil)
	assert.Equal(t, 0, code, stderr.String())

	conf, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "chat", conf.ChatModel)
	assert.Equal(t, "embed", conf.EmbeddingModel)

	output := stdout.String()
	assert.Contains(t, output, "dndx chat", "stdout missing banner")
	assert.Contains(t, output, "Provider: lmstudio", "stdout missing status")
}

func TestInteractivePromptBlinksUnderscore(t *testing.T) {
	assert.Contains(t, interactivePrompt, ansiBlink+"_", "prompt should blink underscore")
	assert.Contains(t, interactivePrompt, "_"+ansiReset, "prompt should reset after underscore")
}

func TestInteractiveModeSendsPlainTextToChat(t *testing.T) {
	question := "provide a brief random encounter table for 3 first-level characters. They are in the woods."
	app, stdout, stderr, configPath := newTestApp(t, question+"\n/quit\n")

	err := config.SaveConfig(configPath, config.Config{
		Provider:  providerLMStudio,
		Endpoint:  defaultLMStudioEndpoint,
		ChatModel: "glm-5.0",
	})
	require.NoError(t, err)

	fake := &fakeChatClient{response: "1. Three nervous scouts cross the trail."}
	var factoryConfig config.Config
	app.chatFactory = func(conf config.Config) (chatClient, error) {
		factoryConfig = conf
		return fake, nil
	}

	code := app.Run(nil)
	assert.Equal(t, 0, code, stderr.String())
	assert.Equal(t, 1, fake.calls, "chat called right number of times")
	assert.Equal(t, "glm-5.0", factoryConfig.ChatModel)
	assert.Len(t, fake.messages, 2)
	assert.Equal(t, "system", fake.messages[0].Role)
	assert.Equal(t, "user", fake.messages[1].Role)
	assert.Equal(t, question, fake.messages[1].Content)

	output := stdout.String()
	assert.Contains(t, output, interactivePrompt, "stdout missing prompt")
	assert.Contains(t, output, "Thinking...", "stdout missing thinking line")
	assert.Contains(t, output, "Three nervous scouts", "stdout missing response")
}

func TestDirectChatCommandSendsQuestion(t *testing.T) {
	app, stdout, stderr, configPath := newTestApp(t, "")

	err := config.SaveConfig(configPath, config.Config{
		Provider:  providerLMStudio,
		Endpoint:  defaultLMStudioEndpoint,
		ChatModel: "glm-5.0",
	})
	require.NoError(t, err)

	fake := &fakeChatClient{response: "Roll 1d4 wolves."}
	app.chatFactory = func(_ config.Config) (chatClient, error) {
		return fake, nil
	}

	code := app.Run([]string{"chat", "provide", "a", "brief", "encounter"})
	assert.Equal(t, 0, code, stderr.String())
	assert.Equal(t, 1, fake.calls, "chat called once")
	assert.Equal(t, "provide a brief encounter", last(fake.messages).Content)
	assert.Contains(t, stdout.String(), "Roll 1d4 wolves")
}

func TestChatRequiresConfiguredEndpoint(t *testing.T) {
	app, _, stderr, _ := newTestApp(t, "")

	code := app.Run([]string{"chat", "hello"})
	assert.NotEqual(t, 0, code, "expected failure")
	assert.Contains(t, stderr.String(), "no model endpoint configured")
}

func TestUnknownProviderReturnsUsageError(t *testing.T) {
	app, _, stderr, _ := newTestApp(t, "")

	code := app.Run([]string{"/connect", "kobold"})
	assert.NotEqual(t, 0, code, "expected failure")
	assert.Contains(t, stderr.String(), "provider must be lmstudio or ollama")
}

func TestConfigPathFromEnvironment(t *testing.T) {
	val := filepath.Join("tmp", "dndx.json")
	t.Setenv("DNDX_CONFIG", val)
	assert.Equal(t, val, configPathFromEnv())
}

func TestLoadConfigMissingFileReturnsEmptyConfig(t *testing.T) {
	conf, err := config.LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	assert.True(t, config.IsEmptyConfig(conf), "config should be empty")
}

func TestSaveConfigCreatesPrivateConfigFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nested", "config.json")
	conf := config.Config{Provider: providerLMStudio, Endpoint: defaultLMStudioEndpoint}

	err := config.SaveConfig(configPath, conf)
	require.NoError(t, err)

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, 0o600, int(info.Mode()))
}

func TestOpenAIChatClientSendsChatCompletionRequest(t *testing.T) {
	client := openAIChatClient{
		endpoint: "http://lmstudio.test/v1/chat/completions",
		model:    "glm-5.0",
		doer: doerFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "/v1/chat/completions", req.URL.Path)

			var payload struct {
				Model    string        `json:"model"`
				Messages []chatMessage `json:"messages"`
				Stream   bool          `json:"stream"`
			}
			err := json.NewDecoder(req.Body).Decode(&payload)
			require.NoError(t, err)

			assert.Equal(t, "glm-5.0", payload.Model)
			assert.Len(t, payload.Messages, 1)
			assert.Equal(t, "encounter", payload.Messages[0].Content)
			assert.False(t, payload.Stream, "payload.Stream")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"Roll 1d4 wolves."}}]}`)),
			}, nil
		}),
	}

	answer, err := client.Send(context.Background(), []chatMessage{{Role: "user", Content: "encounter"}})
	require.NoError(t, err)
	assert.Equal(t, "Roll 1d4 wolves.", answer)
}

func TestOllamaChatClientSendsNativeChatRequest(t *testing.T) {
	client := ollamaChatClient{
		endpoint: "http://ollama.test/api/chat",
		model:    "llama3.2",
		doer: doerFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "/api/chat", req.URL.Path)

			var payload struct {
				Model    string        `json:"model"`
				Messages []chatMessage `json:"messages"`
				Stream   bool          `json:"stream"`
			}
			err := json.NewDecoder(req.Body).Decode(&payload)
			require.NoError(t, err)

			assert.Equal(t, "llama3.2", payload.Model)
			assert.Len(t, payload.Messages, 1)
			assert.Equal(t, "encounter", payload.Messages[0].Content)
			assert.False(t, payload.Stream, "payload.Stream")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":{"role":"assistant","content":"Use two scouts and a lost map."}}`)),
			}, nil
		}),
	}

	answer, err := client.Send(context.Background(), []chatMessage{{Role: "user", Content: "encounter"}})
	require.NoError(t, err)
	assert.Equal(t, "Use two scouts and a lost map.", answer)
}
