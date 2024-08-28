package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/invopop/jsonschema"
	"github.com/liushuangls/go-anthropic/v2"
	"github.com/reconquest/karma-go"
)

type ToolCallFunc func(anthropic.MessageContentToolUse) (any, error)

type Dispatcher struct {
	client *anthropic.Client

	baseModel string

	thread []anthropic.Message
	tools  []anthropic.ToolDefinition
	funcs  map[string]ToolCallFunc

	mutex sync.Mutex

	cwd     string
	verbose bool
}

func NewDispatcher(
	cwd string,
	model string,
	verbose bool,
	token string,
) *Dispatcher {
	client := anthropic.NewClient(token)

	thread := []anthropic.Message{}

	dispatcher := &Dispatcher{
		cwd:       cwd,
		baseModel: model,

		client: client,
		thread: thread,
		mutex:  sync.Mutex{},

		tools:   []anthropic.ToolDefinition{},
		funcs:   map[string]ToolCallFunc{},
		verbose: verbose,
	}

	dispatcher.RegisterTools()

	return dispatcher
}

func (dispatcher *Dispatcher) readThread() error {
	filename := filepath.Join(dispatcher.cwd, "thread.aight.json")

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &dispatcher.thread)
}

func (dispatcher *Dispatcher) saveThread() error {
	filename := filepath.Join(dispatcher.cwd, "thread.aight.json")

	data, err := json.MarshalIndent(dispatcher.thread, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

func (dispatcher *Dispatcher) sandbox(path string) (string, error) {
	if path == "/" {
		return dispatcher.sandbox(".")
	}

	if filepath.IsAbs(path) {
		return path, fmt.Errorf("path must be relative: %s", path)
	}

	if strings.Contains(path, "..") {
		return path, fmt.Errorf("path must not contain '..': %s", path)
	}

	return filepath.Join(dispatcher.cwd, path), nil
}

func register[T any](
	dispatcher *Dispatcher,
	name string,
	description string,
	fn func(T) (any, error),
) {
	schema := jsonschema.Reflect(new(T))

	var reflection *jsonschema.Schema
	for _, def := range schema.Definitions {
		reflection = def
		break
	}

	tool := anthropic.ToolDefinition{
		Name:        name,
		Description: description,
		InputSchema: reflection,
	}

	dispatcher.tools = append(dispatcher.tools, tool)
	dispatcher.funcs[name] = callTool(fn)
}

func (dispatcher *Dispatcher) handleToolCalls(toolUses []anthropic.MessageContentToolUse) error {
	type CallResult struct {
		Call   anthropic.MessageContentToolUse
		Result any
		Error  error
	}

	pipe := make(chan CallResult, len(toolUses))

	assistants := sync.WaitGroup{}
	for _, call := range toolUses {
		assistants.Add(1)

		go func(call anthropic.MessageContentToolUse) {
			defer assistants.Done()

			result, err := dispatcher.callFunction(call)

			pipe <- CallResult{
				Call:   call,
				Result: result,
				Error:  err,
			}
		}(call)
	}

	assistants.Wait()

	close(pipe)

	results := make([]CallResult, 0, len(toolUses))
	failures := []error{}
	for result := range pipe {
		results = append(results, result)

		if result.Error != nil {
			failures = append(
				failures,
				karma.Format(
					result.Error,
					"call: %s(%s)",
					result.Call.Name,
					result.Call.Input,
				),
			)

			result.Result = karma.Flatten(result.Error)
		}
	}

	if len(failures) > 0 {
		log.Println(
			karma.Collect(
				fmt.Errorf("%d/%d tool calls failed", len(failures), len(results)),
				failures...,
			),
		)
	}

	content := []anthropic.MessageContent{}
	for _, result := range results {
		var raw []byte
		var err error

		if result.Error != nil {
			raw = []byte(result.Error.Error())
		} else {
			raw, err = json.Marshal(result.Result)
			if err != nil {
				return err
			}
		}

		content = append(
			content,
			anthropic.NewToolResultMessageContent(
				result.Call.ID,
				string(raw),
				result.Error != nil,
			),
		)
	}

	dispatcher.WriteToolCall(anthropic.Message{
		Role:    anthropic.RoleUser,
		Content: content,
	})

	return nil
}

func (dispatcher *Dispatcher) callFunction(call anthropic.MessageContentToolUse) (any, error) {
	fn, ok := dispatcher.funcs[call.Name]
	if !ok {
		return nil, errors.New("function not found")
	}

	return fn(call)
}

func (dispatcher *Dispatcher) WriteMessage(msg anthropic.Message) error {
	dispatcher.mutex.Lock()
	defer dispatcher.mutex.Unlock()

	dispatcher.thread = append(dispatcher.thread, msg)
	dispatcher.saveThread()

	var role string
	switch msg.Role {
	case anthropic.RoleUser:
		role = color.BlueString("user")
		return nil
	case anthropic.RoleAssistant:
		role = color.RedString("assistant")
	}

	text := ""
	if msg.Content != nil && len(msg.Content) > 0 {
		text = msg.Content[0].GetText()
	} else {
		text = silentMarshal(msg.Content)
	}

	log.Printf(
		"{%s} %v",
		role,
		text,
	)

	return nil
}

func (dispatcher *Dispatcher) WriteToolCall(
	msg anthropic.Message,
) {
	dispatcher.mutex.Lock()
	defer dispatcher.mutex.Unlock()

	dispatcher.thread = append(dispatcher.thread, msg)

	role := color.MagentaString("tool")

	log.Printf(
		"{%s} %s",
		role,
		silentMarshal(msg.Content),
	)
}

var (
	ErrFinishReasonStop = errors.New("finish reason is stop")
)

func (dispatcher *Dispatcher) Communicate(prompt func() string) error {
	completion, err := dispatcher.complete()
	if err != nil {
		return karma.Format(err, "complete")
	}

	err = dispatcher.WriteMessage(anthropic.Message{
		Role:    anthropic.RoleAssistant,
		Content: completion.Content,
	})
	if err != nil {
		return karma.Format(err, "write message")
	}

	var toolUses []anthropic.MessageContentToolUse
	for _, content := range completion.Content {
		if content.Type == anthropic.MessagesContentTypeToolUse {
			toolUses = append(toolUses, *content.MessageContentToolUse)
		}
	}

	if len(toolUses) > 0 {
		err := dispatcher.handleToolCalls(toolUses)
		if err != nil {
			return karma.Format(err, "handle tool calls")
		}

		return nil
	}

	return dispatcher.interact(prompt)
}

func (dispatcher *Dispatcher) interact(prompt func() string) error {
	for {
		input := prompt()

		if input == "" {
			continue
		}

		err := dispatcher.WriteMessage(anthropic.Message{
			Role:    anthropic.RoleUser,
			Content: []anthropic.MessageContent{anthropic.NewTextMessageContent(input)},
		})
		if err != nil {
			return karma.Format(err, "write message")
		}

		break
	}

	return nil
}

func (dispatcher *Dispatcher) complete() (*anthropic.MessagesResponse, error) {
	for {
		requestRateLimit.Take()

		request := anthropic.MessagesRequest{
			Model:     dispatcher.baseModel,
			Messages:  dispatcher.thread,
			MaxTokens: 2000,
			Tools:     dispatcher.tools,
		}

		response, err := dispatcher.client.CreateMessages(context.Background(), request)
		if err != nil {
			time.Sleep(1 * time.Second)

			log.Printf("{%s} request error, retrying... | %s", request.Model, err)

			continue
		}

		return &response, nil
	}
}

func silentMarshal(value any) string {
	marshaled, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<marshal error: %s> %#v", err, value)
	}

	return string(marshaled)
}
