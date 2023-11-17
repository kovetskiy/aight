package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/invopop/jsonschema"
	"github.com/reconquest/karma-go"
	"github.com/sashabaranov/go-openai"
)

type ToolCallFunc func(openai.ToolCall) (any, error)

type Dispatcher struct {
	client *openai.Client

	baseModel string

	thread []openai.ChatCompletionMessage
	tools  []openai.Tool
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
	client := openai.NewClient(token)

	thread := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: defaultSystemMessage,
		},
	}

	dispatcher := &Dispatcher{
		cwd:       cwd,
		baseModel: model,

		client: client,
		thread: thread,
		mutex:  sync.Mutex{},

		tools:   []openai.Tool{},
		funcs:   map[string]ToolCallFunc{},
		verbose: verbose,
	}

	dispatcher.RegisterTools()

	return dispatcher
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

	tool := openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: openai.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  reflection,
		},
	}

	dispatcher.tools = append(dispatcher.tools, tool)
	dispatcher.funcs[name] = callTool(fn)
}

func (dispatcher *Dispatcher) handleToolCalls(choice openai.ChatCompletionChoice) error {
	type CallResult struct {
		Call   openai.ToolCall
		Result any
		Error  error
	}

	pipe := make(chan CallResult, len(choice.Message.ToolCalls))

	assistants := sync.WaitGroup{}
	for _, call := range choice.Message.ToolCalls {
		assistants.Add(1)

		go func(call openai.ToolCall) {
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

	results := make([]CallResult, 0, len(choice.Message.ToolCalls))
	failures := []error{}
	for result := range pipe {
		results = append(results, result)

		if result.Error != nil {
			failures = append(
				failures,
				karma.Format(
					result.Error,
					"call: %s(%s)",
					result.Call.Function.Name,
					result.Call.Function.Arguments,
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

	for _, result := range results {
		dispatcher.WriteToolCall(openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: result.Call.ID,
			Content:    fmt.Sprint(result.Result),
		}, result.Call)
	}

	return nil
}

func (dispatcher *Dispatcher) callFunction(call openai.ToolCall) (any, error) {
	fn, ok := dispatcher.funcs[call.Function.Name]
	if !ok {
		return nil, errors.New("function not found")
	}

	return fn(call)
}

func (dispatcher *Dispatcher) Write(msg openai.ChatCompletionMessage) {
	dispatcher.mutex.Lock()
	defer dispatcher.mutex.Unlock()

	dispatcher.thread = append(dispatcher.thread, msg)

	if len(msg.ToolCalls) == 0 {
		var role string
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			role = color.BlueString("user")
		case openai.ChatMessageRoleSystem:
			if !dispatcher.verbose {
				return
			}

			role = color.GreenString("system")
		case openai.ChatMessageRoleAssistant:
			role = color.RedString("assistant")
		}

		log.Printf(
			"{%s} %v",
			role,
			msg.Content,
		)
	}
}

func (dispatcher *Dispatcher) WriteToolCall(
	msg openai.ChatCompletionMessage,
	call openai.ToolCall,
) {
	dispatcher.mutex.Lock()
	defer dispatcher.mutex.Unlock()

	dispatcher.thread = append(dispatcher.thread, msg)

	if msg.ToolCallID != "" {
		role := color.MagentaString("tool")

		log.Printf(
			"{%s: %s} %s",
			role,
			call.Function.Name,
			silentMarshal(msg.Content),
		)

		return
	}
}

var (
	ErrFinishReasonStop = errors.New("finish reason is stop")
)

func (dispatcher *Dispatcher) Interact(prompt func() string) error {
	completion, err := dispatcher.complete()
	if err != nil {
		return karma.Format(err, "complete")
	}

	choice := completion.Choices[0]

	dispatcher.Write(choice.Message)

	//if choice.Message.Content != "" {
	//    err := dispatcher.TextToSpeech(choice.Message.Content)
	//    if err != nil {
	//        log.Println(karma.Format(err, "text to speech"))
	//    }
	//}

	switch choice.FinishReason {
	case openai.FinishReasonToolCalls:
		return dispatcher.handleToolCalls(choice)
	case openai.FinishReasonStop:
		if choice.Message.Content == "CONFIRM" {
			return dispatcher.interact(func() string { return "CONFIRM" })
		}

		return dispatcher.interact(prompt)

	default:
		return fmt.Errorf("unexpected finish reason: %s", choice.FinishReason)
	}
}

func (dispatcher *Dispatcher) interact(prompt func() string) error {
	for {
		input := prompt()

		if input == "" {
			continue
		}

		dispatcher.Write(openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: input,
		})

		break
	}

	return nil
}

func (dispatcher *Dispatcher) complete() (*openai.ChatCompletionResponse, error) {
	for {
		requestRateLimit.Take()

		request := openai.ChatCompletionRequest{
			Model:    dispatcher.baseModel,
			Messages: dispatcher.thread,
			Tools:    dispatcher.tools,
		}

		response, err := dispatcher.client.CreateChatCompletion(context.Background(), request)
		if err != nil {
			time.Sleep(1 * time.Second)

			log.Printf("{%s} request error, retrying... | %s", request.Model, err)

			continue
			//return nil, err
		}

		if len(response.Choices) == 0 {
			return nil, errors.New("no choices returned")
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
