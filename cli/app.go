package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tauliang/DnDGenie/cli/config"
)

const (
	ansiBlink         = "\x1b[5m"
	ansiReset         = "\x1b[0m"
	interactivePrompt = "dndx " + ansiBlink + "_" + ansiReset + " "
)

type App struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	configPath  string
	chatFactory chatClientFactory
	chatHistory []chatMessage
}

type usageError string

func (e usageError) Error() string {
	return string(e)
}

func errUsage(message string) error {
	return usageError(message)
}

func NewApp(stdin io.Reader, stdout io.Writer, stderr io.Writer, configPath string) *App {
	return &App{
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		configPath:  configPath,
		chatFactory: newChatClient,
	}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		return a.runInteractive()
	}

	if err := a.runCommand(args); err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		var usage usageError
		if errors.As(err, &usage) {
			fmt.Fprintln(a.stderr, "Run dndx help for usage.")
		}
		return 1
	}
	return 0
}

func (a *App) runInteractive() int {
	fmt.Fprintln(a.stdout, "dndx chat. Type a message, /help for commands, /quit to exit.")
	scanner := bufio.NewScanner(a.stdin)

	for {
		fmt.Fprint(a.stdout, interactivePrompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(a.stderr, "error: %v\n", err)
				return 1
			}
			fmt.Fprintln(a.stdout)
			return 0
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "quit" || line == "/exit" || line == "exit" {
			return 0
		}

		if err := a.runInteractiveLine(line); err != nil {
			fmt.Fprintf(a.stderr, "error: %v\n", err)
		}
	}
}

func (a *App) runInteractiveLine(line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	if strings.HasPrefix(fields[0], "/") || isCommandName(fields[0]) {
		return a.runCommand(fields)
	}
	return a.runChat(line)
}

func (a *App) runCommand(args []string) error {
	command := strings.TrimPrefix(strings.ToLower(args[0]), "/")

	switch command {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "connect":
		return a.runConnect(args[1:])
	case "models":
		return a.runModels(args[1:])
	case "status":
		return a.runStatus()
	case "chat":
		return a.runChat(strings.Join(args[1:], " "))
	default:
		return errUsage(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func isCommandName(command string) bool {
	switch strings.TrimPrefix(strings.ToLower(command), "/") {
	case "help", "-h", "--help", "connect", "models", "status", "chat":
		return true
	default:
		return false
	}
}

func (a *App) runConnect(args []string) error {
	positionals, options, err := parseOptions(args, map[string]bool{"url": true})
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage("usage: dndx /connect <lmstudio|ollama> [--url URL]")
	}

	provider, err := normalizeProvider(positionals[0])
	if err != nil {
		return err
	}

	conf, err := config.Load(a.configPath)
	if err != nil {
		return err
	}

	conf.Provider = provider
	conf.Endpoint = normalizeEndpoint(provider, options["url"])
	if err := config.Save(a.configPath, conf); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Connected to %s at %s\n", conf.Provider, conf.Endpoint)
	return nil
}

func (a *App) runModels(args []string) error {
	positionals, options, err := parseOptions(args, map[string]bool{
		"chat":      true,
		"embedding": true,
	})
	if err != nil {
		return err
	}
	if len(positionals) > 0 {
		return errUsage("usage: dndx models [--chat MODEL] [--embedding MODEL]")
	}

	conf, err := config.Load(a.configPath)
	if err != nil {
		return err
	}

	if len(options) == 0 {
		a.printConfig(conf)
		return nil
	}

	if chatModel, ok := options["chat"]; ok {
		conf.ChatModel = chatModel
	}
	if embeddingModel, ok := options["embedding"]; ok {
		conf.EmbeddingModel = embeddingModel
	}

	if err := config.Save(a.configPath, conf); err != nil {
		return err
	}

	fmt.Fprintln(a.stdout, "Configured models:")
	fmt.Fprintf(a.stdout, "  chat: %s\n", valueOrPlaceholder(conf.ChatModel))
	fmt.Fprintf(a.stdout, "  embedding: %s\n", valueOrPlaceholder(conf.EmbeddingModel))
	return nil
}

func (a *App) runStatus() error {
	conf, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	a.printConfig(conf)
	return nil
}

func (a *App) runChat(question string) error {
	question = strings.TrimSpace(question)
	if question == "" {
		return errUsage("usage: dndx chat QUESTION")
	}

	conf, err := config.Load(a.configPath)
	if err != nil {
		return err
	}

	client, err := a.chatFactory(conf)
	if err != nil {
		return err
	}

	userMessage := chatMessage{Role: "user", Content: question}
	messages := append([]chatMessage{{Role: "system", Content: dndChatSystemPrompt}}, a.chatHistory...)
	messages = append(messages, userMessage)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintln(a.stdout, "Thinking...")
	answer, err := client.Send(ctx, messages)
	if err != nil {
		return err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = emptyChatResponseMessage
	}

	a.chatHistory = append(a.chatHistory, userMessage, chatMessage{Role: "assistant", Content: answer})
	fmt.Fprintln(a.stdout, answer)
	fmt.Fprintln(a.stdout)
	return nil
}

func (a *App) printHelp() {
	fmt.Fprint(a.stdout, `dndx - DnDGenie command line companion

Commands:
  /connect <lmstudio|ollama> [--url URL]
      Configure the local model endpoint. LM Studio defaults to
      http://127.0.0.1:1234/v1 and Ollama defaults to http://127.0.0.1:11434.

  models [--chat MODEL] [--embedding MODEL]
      Show or configure the chat and embedding models.

  status
      Show the current provider, endpoint, and model configuration.

  chat QUESTION
      Send a question to the configured local chat model.

Interactive:
  Run dndx with no arguments to open a chat prompt. Type plain text to chat.
  Slash commands such as /connect, /help, and /quit are handled locally.
`)
}

func (a *App) printConfig(conf config.Config) {
	if config.IsEmpty(conf) {
		fmt.Fprintln(a.stdout, "No dndx configuration found. Run /connect first.")
		fmt.Fprintf(a.stdout, "Config path: %s\n", a.configPath)
		return
	}

	fmt.Fprintf(a.stdout, "Provider: %s\n", valueOrPlaceholder(conf.Provider))
	fmt.Fprintf(a.stdout, "Endpoint: %s\n", valueOrPlaceholder(conf.Endpoint))
	fmt.Fprintf(a.stdout, "Chat model: %s\n", valueOrPlaceholder(conf.ChatModel))
	fmt.Fprintf(a.stdout, "Embedding model: %s\n", valueOrPlaceholder(conf.EmbeddingModel))
	fmt.Fprintf(a.stdout, "Config path: %s\n", a.configPath)
}

func valueOrPlaceholder(value string) string {
	if value == "" {
		return "(not configured)"
	}
	return value
}

func parseOptions(args []string, allowed map[string]bool) ([]string, map[string]string, error) {
	positionals := make([]string, 0, len(args))
	options := map[string]string{}

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}

		keyValue := strings.TrimPrefix(arg, "--")
		key := keyValue
		value := ""
		if split := strings.Index(keyValue, "="); split >= 0 {
			key = keyValue[:split]
			value = keyValue[split+1:]
		} else {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, nil, errUsage(fmt.Sprintf("missing value for --%s", key))
			}
			value = args[index+1]
			index++
		}

		if !allowed[key] {
			return nil, nil, errUsage(fmt.Sprintf("unknown option --%s", key))
		}
		if value == "" {
			return nil, nil, errUsage(fmt.Sprintf("empty value for --%s", key))
		}
		options[key] = value
	}

	return positionals, options, nil
}
