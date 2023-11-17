package main

import (
	"encoding/json"
	"log"

	"github.com/fatih/color"
	"github.com/reconquest/karma-go"
	"github.com/sashabaranov/go-openai"
)

func callTool[T any](fn func(T) (any, error)) ToolCallFunc {
	return func(call openai.ToolCall) (any, error) {
		var value T
		err := json.Unmarshal([]byte(call.Function.Arguments), &value)
		if err != nil {
			return nil, karma.Format(err, "decode json of %s", call.Function.Arguments)
		}

		role := color.CyanString("assistant")

		log.Printf("{%s} %s: %+v", role, call.Function.Name, value)

		return fn(value)
	}
}
