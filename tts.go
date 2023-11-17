package main

import (
	"context"
	"io"
	"os/exec"

	"github.com/reconquest/executil-go"
	"github.com/reconquest/karma-go"
	"github.com/sashabaranov/go-openai"
)

func (dispatcher *Dispatcher) TextToSpeech(input string) error {
	requestRateLimit.Take()
	request := openai.CreateSpeechRequest{
		Model:          "tts-1",
		Voice:          openai.VoiceOnyx,
		ResponseFormat: openai.SpeechResponseFormatMp3,
		Speed:          2,
		Input:          input,
	}

	stream, err := dispatcher.client.CreateSpeech(context.Background(), request)
	if err != nil {
		return karma.Format(err, "create speech")
	}

	defer stream.Close()

	err = play(stream)
	if err != nil {
		return karma.Format(err, "play speech")
	}

	return nil
}

func play(stream io.Reader) error {
	cmd := exec.Command("mplayer", "-cache", "1024", "-")
	cmd.Stdin = stream

	_, _, err := executil.Run(cmd)
	if err != nil {
		return karma.Format(err, "run player")
	}

	return nil
}
