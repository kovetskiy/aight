package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/liushuangls/go-anthropic/v2"
)

const (
	defaultModel = anthropic.ModelClaude3Dot5Sonnet20240620
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
  -t --token <token>  Anthropic API token. [default: $ANTHROPIC_API_KEY]
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

	err = dispatcher.readThread()
	if err != nil {
		log.Fatal(err)
	}

	//if len(dispatcher.thread) == 0 {
	//    err := dispatcher.WriteMessage(anthropic.Message{
	//        Role: anthropic.RoleUser,
	//        Content: []anthropic.MessageContent{
	//            anthropic.NewTextMessageContent(defaultSystemMessage),
	//        },
	//    })
	//    if err != nil {
	//        log.Fatal(err)
	//    }
	//}

	index := 0
	prompt := func() string {
		if index >= len(args.ValuePrompt) {
			return PromptStdin()
		}

		result := args.ValuePrompt[index]

		index++

		fmt.Fprintln(os.Stderr, result)

		return result
	}

	ask := len(dispatcher.thread) == 0
	if len(dispatcher.thread) > 0 {
		last := dispatcher.thread[len(dispatcher.thread)-1]
		if last.Role != anthropic.RoleUser {
			ask = true
		}
	}

	if ask {
		err := dispatcher.interact(prompt)
		if err != nil {
			log.Fatal(err)
		}
	}

	for {
		err = dispatcher.Communicate(prompt)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func PromptStdin() string {
	for {
		fmt.Println()
		fmt.Print("λ ")

		scanner := bufio.NewScanner(os.Stdin)

		var input string
		if scanner.Scan() {
			input = scanner.Text()
		}

		broke := strings.HasSuffix(input, "\n")
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if !broke {
			fmt.Println()
		}

		return input
	}
}
