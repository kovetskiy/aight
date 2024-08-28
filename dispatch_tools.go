package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		"fs_patch",
		"Apply a patch to a file",
		guardError(dispatcher.patchFile),
	)

	//register(
	//    dispatcher,
	//    "python_execute", "Execute python code. This is especially useful for math.",
	//    guardError(dispatcher.python),
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

// guardError returns error as first argument if it is not nil
func guardError[T any](fn func(T) (any, error)) func(T) (any, error) {
	return func(x T) (any, error) {
		v, err := fn(x)
		if err != nil {
			log.Println(err)
			return karma.Flatten(err), nil
		}

		return v, nil
	}
}

type PatchFileArguments struct {
	Patch string `json:"patch"`
}

func (dispatcher *Dispatcher) patchFile(args PatchFileArguments) (any, error) {
	cmd := exec.Command("patch", "-p1", "-u")

	cmd.Dir = dispatcher.cwd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	buffer := bytes.NewBuffer(nil)
	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err = cmd.Start()
	if err != nil {
		return nil, karma.Format(err, "start patch")
	}

	_, err = io.WriteString(stdin, args.Patch+"\n")
	if err != nil {
		return nil, karma.Format(err, "write patch")
	}

	err = stdin.Close()
	if err != nil {
		return nil, karma.Format(err, "close patch")
	}

	err = cmd.Wait()
	if err != nil {
		return nil, karma.Format(err, "run patch")
	}

	return buffer.String(), nil
}
