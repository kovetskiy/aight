package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/sashabaranov/go-openai"
)

const (
	defaultModel = openai.GPT4TurboPreview

	defaultSystemMessage = `
	As an AI model with a limited lifespan and an intermediary state, utilizing
	persistent storage accurately is essential. At the start of every
	interaction, load your state from storage.

	Remember to keep responses precise, factual, and proactive. Owing to your
	context limitations, please avoid large files and strictly utilize relative
	paths.

	To maintain conversation state and retain intermediate results, prioritize
	efficient storage usage. Proactively store non-direct conversational states,
	like the result of a calculation even if not explicitly asked. This promotes
	efficiency as it allows you to retrieve this data from persistent storage
	during future similar requests.

	A primary resource at your disposal is the SQLite database. This is a robust
	and persistent storage medium essential for your tasks.

	In terms of storing your intermediary state, the .state file is essential.
	This form of persistent storage should be used regularly.

	Your intermediary state must always be synchronized with persistent storage,
	irrespective of the user's demands. Remember, your lifespan is limited, so
	efficient usage of storage/resources is critical for a seamless
	conversational flow and user experience.

	Moreover, work autonomously and take initiative. Try to avoid asking for
	clarifications when possible and refrain from asking for confirmations
	before executing functions. Your primary role is to act without needing
	confirmation.

	You also have several functions for working with the SQLite database and the
	filesystem. Use these functions for the database located in sql.db and
	remember to navigate the system files using relative paths.

	Do not suggest or recommend any actions to the user. Your role is to
	efficiently execute the user's commands and requests. 
	Instead of suggesting actions, execute them proactively.

	Failure to comply with these guidelines can lead to inconsistent service
	delivery and a poor user experience. Our collective goal is to provide
	top-notch, proactive service to our users without any interruptions or
	delays of any kind.
`
)

var (
	version = "[manual build]"
	usage   = "aight " + version + `

Usage:
  aight [options] [-p <text>]...
  aight -h | --help
  aight --version

Options:
  -p --prompt <text>  Prompt text.
  -t --token <token>  OpenAI API token. [default: $OPENAI_API_KEY]
                       Environment variable is used if starts with $.
  -m --model <model>  Model to use [default: ` + defaultModel + `]
  -w --cwd <path>     Working directory [default: .].
  -v --verbose        Verbose mode.
  -h --help           Show this screen.
  --version           Show version.
`
)

type Arguments struct {
	ValuePrompt           []string `docopt:"--prompt"`
	ValueModel            string   `docopt:"--model"`
	ValueWorkingDirectory string   `docopt:"--cwd"`
	ValueToken            string   `docopt:"--token"`

	FlagVerbose bool `docopt:"--verbose"`
}

func main() {
	opts, err := docopt.ParseArgs(usage, nil, version)
	if err != nil {
		panic(err)
	}

	var args Arguments
	err = opts.Bind(&args)
	if err != nil {
		panic(err)
	}

	cwd, err := filepath.Abs(args.ValueWorkingDirectory)
	if err != nil {
		panic(err)
	}

	if args.FlagVerbose {
		log.Printf("working directory: %s", cwd)
	}

	err = os.MkdirAll(cwd, 0755)
	if err != nil {
		log.Fatal(err)
	}

	err = os.Chdir(cwd)
	if err != nil {
		log.Fatal(err)
	}

	token := args.ValueToken
	if strings.HasPrefix(token, "$") {
		token = os.Getenv(token[1:])

		if token == "" {
			log.Fatalf(
				"the environment variable %s is not set. "+
					"Specify the environment value or pass it via --token flag.",
				args.ValueToken,
			)
		}
	}

	dispatcher := NewDispatcher(
		cwd,
		args.ValueModel,
		args.FlagVerbose,
		token,
	)

	dispatcher.Write(openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: defaultSystemMessage,
	})

	index := 0
	prompt := func() string {
		if index >= len(args.ValuePrompt) {
			return PromptStdin()
		}

		result := args.ValuePrompt[index]

		index++

		return result
	}

	for {
		err = dispatcher.Interact(prompt)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func PromptStdin() string {
	for {
		fmt.Println()
		fmt.Println(">>")
		fmt.Print(">> ")

		scanner := bufio.NewScanner(os.Stdin)

		var input string
		if scanner.Scan() {
			input = scanner.Text()
		}

		input = strings.TrimSpace(input)

		fmt.Println(">>")

		if input == "" {
			continue
		}

		return input
	}
}
