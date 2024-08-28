package main

import (
	"encoding/json"
	"log"

	"github.com/fatih/color"
	"github.com/liushuangls/go-anthropic/v2"
	"github.com/reconquest/karma-go"
)

func callTool[T any](fn func(T) (any, error)) ToolCallFunc {
	return func(call anthropic.MessageContentToolUse) (any, error) {
		var value T
		err := json.Unmarshal([]byte(call.Input), &value)
		if err != nil {
			return nil, karma.Format(err, "decode json of %s", call.Input)
		}

		role := color.CyanString("assistant")

		log.Printf("{%s} %s: %+v", role, call.Name, value)

		return fn(value)
	}
}
