package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/reconquest/executil-go"
	"github.com/reconquest/karma-go"
)

func (dispatcher *Dispatcher) RegisterTools() {
	register(
		dispatcher,
		"fs_list", "Filesystem: List files in the given path",
		guardError(dispatcher.listFiles),
	)

	register(
		dispatcher,
		"fs_tree", "Filesystem: List files in the given path recursively. Useful for starting point.",
		guardError(dispatcher.treeFiles),
	)

	register(
		dispatcher,
		"fs_read", "Filesystem: Read file by the given path. Avoid using this for large files. Avoid using it for non-text files like images.",
		guardError(dispatcher.readFile),
	)

	register(
		dispatcher,
		"fs_write", "Filesystem: Write file by the given path.",
		guardError(dispatcher.writeFile),
	)

	register(
		dispatcher,
		"fs_move", "Filesystem: Move file",
		guardError(dispatcher.moveFile),
	)

	register(
		dispatcher,
		"fs_remove", "Filesystem: Remove file",
		guardError(dispatcher.removeFile),
	)

	register(
		dispatcher,
		"sql_exec", "SQLite: execute statement and return result (rows affected, last insert id)",
		guardError(dispatcher.sqlExec),
	)

	register(
		dispatcher,
		"sql_query", "SQLite: execute query and return result (rows)",
		guardError(dispatcher.sqlQuery),
	)

	register(
		dispatcher,
		"python_execute", "Execute python code. This is especially useful for math.",
		guardError(dispatcher.python),
	)

	//register(
	//    dispatcher,
	//    "http_request", "Make HTTP request and return response",
	//    guardError(dispatcher.httpRequest),
	//)

	//register(
	//    dispatcher,
	//    "ai_summarize", "Summarize text using AI",
	//    guardError(dispatcher.aiSummarize),
	//)

	//register(
	//    dispatcher,
	//    "ollama_run", "Run ollama ML model. This is especially useful for code generation.",
	//    guardError(dispatcher.ollamaRun),
	//)
}

type ListFilesArguments struct {
	Path string `json:"path"`
}

func (dispatcher *Dispatcher) listFiles(args ListFilesArguments) (any, error) {
	type File struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir,omitempty"`
		Size int64  `json:"size,omitempty"`
	}

	result := []File{}

	path, err := dispatcher.sandbox(args.Path)
	if err != nil {
		return nil, err
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return nil, karma.Format(err, "read dir: %s", path)
	}

	for _, file := range files {
		var size int64
		if !file.IsDir() {
			info, err := file.Info()
			if err != nil {
				return nil, karma.Format(err, "get file info")
			}

			size = info.Size()
		}

		result = append(result, File{
			Name: file.Name(),
			Dir:  file.IsDir(),
			Size: size,
		})
	}

	return result, nil
}

type ReadFileArguments struct {
	Path string `json:"path"`
}

func (dispatcher *Dispatcher) readFile(args ReadFileArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Path)
	if err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return string(contents), nil
}

type WriteFileArguments struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
	Append   bool   `json:"append"`
}

func (dispatcher *Dispatcher) writeFile(args WriteFileArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Path)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return nil, karma.Format(err, "create directory: %s", filepath.Dir(path))
	}

	flags := os.O_CREATE | os.O_WRONLY
	if args.Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	fd, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, karma.Format(err, "open file: %s", path)
	}

	defer fd.Close()

	_, err = fd.WriteString(args.Contents)
	if err != nil {
		return nil, karma.Format(err, "write file: %s", path)
	}

	return true, nil
}

type MoveFileArguments struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (dispatcher *Dispatcher) moveFile(args MoveFileArguments) (any, error) {
	from, err := dispatcher.sandbox(args.From)
	if err != nil {
		return nil, err
	}

	to, err := dispatcher.sandbox(args.To)
	if err != nil {
		return nil, err
	}

	err = os.Rename(from, to)
	if err != nil {
		return nil, karma.Format(err, "rename file")
	}

	return true, nil
}

type RemoveFileArguments struct {
	Path string `json:"path"`
}

func (dispatcher *Dispatcher) removeFile(args RemoveFileArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Path)
	if err != nil {
		return nil, err
	}

	err = os.Remove(path)
	if err != nil {
		return nil, karma.Format(err, "remove file")
	}

	return true, nil
}

type TreeFilesArguments struct {
	Path string `json:"path"`
}

func (dispatcher *Dispatcher) treeFiles(args TreeFilesArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Path)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("tree", "-J", path)

	stdout, _, err := executil.Run(cmd)
	if err != nil {
		return nil, karma.Format(err, "run tree")
	}

	var result any
	err = json.Unmarshal(stdout, &result)
	if err != nil {
		return nil, karma.Format(err, "unmarshal tree")
	}

	return result, nil
}

type SQLExecArguments struct {
	Database string `json:"database"`
	Query    string `json:"query"`
}

func (arguments SQLExecArguments) String() string {
	return arguments.Query
}

func (dispatcher *Dispatcher) sqlExec(args SQLExecArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Database)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, karma.Format(err, "open database")
	}

	defer db.Close()

	result, err := db.Exec(args.Query)
	if err != nil {
		return nil, karma.Format(err, "execute query")
	}

	reply := map[string]any{}

	reply["last_insert_id"], err = result.LastInsertId()
	if err != nil {
		reply["last_insert_id_error"] = err.Error()
	}

	reply["rows_affected"], err = result.RowsAffected()
	if err != nil {
		reply["rows_affected_error"] = err.Error()
	}

	return reply, nil
}

type SQLQueryArguments struct {
	Database string `json:"database"`
	Query    string `json:"query"`
}

func (arguments SQLQueryArguments) String() string {
	return arguments.Query
}

func (dispatcher *Dispatcher) sqlQuery(args SQLQueryArguments) (any, error) {
	path, err := dispatcher.sandbox(args.Database)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, karma.Format(err, "open database")
	}

	defer db.Close()

	rows, err := db.Query(args.Query)
	if err != nil {
		return nil, karma.Format(err, "execute query")
	}

	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, karma.Format(err, "get columns")
	}

	result := []map[string]any{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))

		for i := range columns {
			pointers[i] = &values[i]
		}

		err = rows.Scan(pointers...)
		if err != nil {
			return nil, karma.Format(err, "scan row")
		}

		row := map[string]any{}

		for i, column := range columns {
			row[column] = values[i]
		}

		result = append(result, row)
	}

	return result, nil
}

type PythonArguments struct {
	ScriptName string `json:"script_name"`
	Code       string `json:"code"`
}

func (arguments PythonArguments) String() string {
	return arguments.ScriptName + "\n" + arguments.Code
}

func (dispatcher *Dispatcher) python(args PythonArguments) (any, error) {
	path, err := dispatcher.sandbox(args.ScriptName)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(path, ".py") {
		path += ".py"
	}

	err = os.WriteFile(path, []byte(args.Code), 0644)
	if err != nil {
		return nil, karma.Format(err, "write python code")
	}

	stdout, stderr, err := executil.Run(
		exec.Command("python3", path),
	)
	if err != nil {
		return nil, karma.Format(err, "run python code")
	}

	if len(stdout) == 0 && len(stderr) == 0 {
		return "ok", nil
	}

	return map[string]string{
		"stdout": string(stdout),
		"stderr": string(stderr),
	}, nil
}

type OllamaRunArguments struct {
	Prompt string `json:"prompt"`
}

func (dispatcher *Dispatcher) ollamaRun(args OllamaRunArguments) (any, error) {
	model := "codellama"

	cmd := exec.Command("ollama", "run", model, args.Prompt)

	stdout, _, err := executil.Run(cmd)
	if err != nil {
		return nil, karma.Format(err, "run ollama")
	}

	return string(stdout), nil
}

type ASArguments struct {
	Text string `json:"text"`
}

func (dispatcher *Dispatcher) aiSummarize(args ASArguments) (any, error) {
	return dispatcher.ai("Summarize the following text: " + args.Text)
}

func (dispatcher *Dispatcher) ai(prompt string) (any, error) {
	return nil, nil
	//request := openai.ChatCompletionRequest{
	//    Model: openai.GPT432K0613,
	//}

}

// guardError returns error as first argument if it is not nil
func guardError[T any](fn func(T) (any, error)) func(T) (any, error) {
	return func(x T) (any, error) {
		v, err := fn(x)
		if err != nil {
			return karma.Flatten(err), nil
		}

		return v, nil
	}
}

type HTTPRequestArguments struct {
	Endpoint string            `json:"endpoint"`
	Query    map[string]string `json:"query,omitempty"`
	Method   string            `json:"method,omitempty"`
	Body     string            `json:"body,omitempty"`
}

func (arguments HTTPRequestArguments) String() string {
	return fmt.Sprintf(
		"endpoint=%v query=%v method=%v body=%v",
		arguments.Endpoint,
		arguments.Query,
		arguments.Method,
		arguments.Body,
	)
}

func (dispatcher *Dispatcher) httpRequest(args HTTPRequestArguments) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if len(args.Query) > 0 {
		query := url.Values{}
		for key, value := range args.Query {
			query.Add(key, value)
		}

		if strings.Contains(endpoint, "?") {
			endpoint += "&" + query.Encode()
		} else {
			endpoint += "?" + query.Encode()
		}
	}

	var payload io.Reader
	var headers http.Header
	if args.Body != "" {
		payload = bytes.NewBufferString(args.Body)

		headers = http.Header{}
		headers.Set("Content-Type", "application/json")
	}

	request, err := http.NewRequestWithContext(ctx, args.Method, endpoint, payload)
	if err != nil {
		return nil, karma.Format(err, "create request")
	}

	request.Header = headers

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, karma.Format(err, "send request")
	}

	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("can't read response body: %s", err)
	}

	result := map[string]any{
		"status": response.StatusCode,
		"body":   string(body),
	}

	return result, nil
}
