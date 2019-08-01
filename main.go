/*
	recursive grep "pattern" in a directory
	scan all text files by default
*/
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	//https://misc.flogisoft.com/bash/tip_colors_and_formatting
	COLOR_NONE  = "\033[0m"
	COLOR_DIR   = "\033[0;34m"
	COLOR_EXE   = "\033[0;36m"
	COLOR_ERROR = "\033[38;5;124m"
	COLOR_DIM   = "\033[38;5;243m"

	_VERSION = "1.0.0 rc1"
)

var GitBranch string
var Version string
var BuildDate string
var GitID string

// input values
type Options struct {
	mime, help                    bool
	sort, directory, pattern, ext string
}

// parse console input values
func argsParser() (Options, error) {
	ret := Options{
		sort:      "none",
		directory: "",
	}

	for _, arg := range os.Args[1:] {
		if len(arg) < 1 {
			// user enter: program.go "" "" "" !
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			// first is regex
			if ret.pattern == "" {
				arg := strings.ReplaceAll(arg, "(", "\\(")
				ret.pattern = strings.ReplaceAll(arg, ")", "\\)")
				continue
			} else if ret.directory == "" {
				// path ?
				if !strings.HasPrefix(arg, "/") {
					ret.directory, _ = os.Getwd()
					ret.directory += "/" + arg
					ret.directory = filepath.Dir(ret.directory)
				} else {
					ret.directory = arg
				}
				_, err := os.Stat(ret.directory)
				if os.IsNotExist(err) {
					ret.directory, _ = os.Getwd()
					return ret, err
				}
				continue
			} else if ret.directory != "" {
				// one optional extension filter
				if arg[:1] == "." {
					ret.ext = arg
				} else {
					ret.ext = "." + arg
				}
				continue
			}
		}
		// test options, not used here
		for _, ch := range arg[1:] {
			switch ch {
			case 'h':
				ret.help = true
			case 'm':
				ret.mime = true
			case 't':
				ret.sort = "time"
			case 's':
				ret.sort = "size"
			}
		}
	}
	if len(ret.pattern) < 3 {
		return ret, errors.New("console value: regex pattern too short")
	}
	if ret.directory == "" {
		ret.directory, _ = os.Getwd()
	}
	_, err := os.Stat(ret.directory)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

// fields returned by grep_ function
type line struct {
	id  uint64
	txt string
}

func stringInSlice(a string, list *[]string) bool {
	for _, b := range *list {
		if b == a {
			return true
		}
	}
	return false
}

// for grep only text files
func test_file_type(filename string) bool {
	cmd := exec.Command("file", filename, "-b", "--mime-type")
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), "text")
}

// grep stream
// return array of lines
func grep_stream(reader *bufio.Reader, pattern *regexp.Regexp, url *string) []line {

	returns := []line{}

	scanner := bufio.NewScanner(reader)
	var line_count uint64 = 1
	var char int
	for scanner.Scan() {
		data := scanner.Text()
		if line_count < 2 {
			if len(data) < 1 {
				char = 40
			} else {
				char = int(data[0])
			}
		}
		if len(data) > 2040 || ((line_count < 2) && (char < 32 || char > 122)) {
			if stringInSlice("-b", &os.Args) {
				println(COLOR_ERROR, "binary file ? seek !", url, COLOR_NONE)
			}
			return []line{} // too long is a binary file ?
		}

		if found := pattern.FindString(data); found != "" {
			data = strings.ReplaceAll(data, found, COLOR_EXE+found+COLOR_NONE)
			if strings.HasPrefix(data, "    ") {
				data = data[4:]
			}
			returns = append(returns, line{id: line_count, txt: data})
		}
		line_count++
	}

	return returns
}

// grep file
// return array of lines
func grep_file(filename string, pattern *regexp.Regexp) []line {

	returns := []line{}

	if !test_file_type(filename) {
		return returns
	}

	freader, err := os.OpenFile(filename, os.O_RDONLY, 0444)
	if err != nil {
		//fmt.Println(err.Error)
		return returns
	}
	defer freader.Close()

	return grep_stream(bufio.NewReader(freader), pattern, &filename)

}

func usage(err error) {
	fmt.Printf("\n%s Version: %v %v %v %v\n", filepath.Base(os.Args[0]), Version, GitID, GitBranch, BuildDate)
	fmt.Printf("Usage: %v \"regex PATTERN\" [directory] [extension]\n\n", filepath.Base(os.Args[0]))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {

	options, err := argsParser()
	if err != nil || options.help {
		usage(err)
	}

	pattern, err := regexp.Compile(options.pattern)
	if err != nil {
		usage(err)
	}

	var mutex = sync.Mutex{}
	var waitg sync.WaitGroup

	err_d := filepath.Walk(options.directory,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// bad permission for one file
				fmt.Println(COLOR_ERROR, err, COLOR_NONE)
				return nil
			}
			if strings.HasPrefix(info.Name(), ".") || info.IsDir() || info.Size() < 3 {
				return nil
			}
			if strings.Contains(path, "__pycache__") || strings.Contains(path, "/.git/") {
				return nil
			}
			if options.ext != "" && (options.ext != filepath.Ext(path)) {
				return nil
			}

			waitg.Add(1)
			matchs := grep_file(path, pattern)
			waitg.Done()

			// display results for one file
			if len(matchs) > 0 {
				var out = fmt.Sprintln(len(matchs), "in", COLOR_DIR, path, COLOR_NONE)
				for _, line := range matchs {
					out += fmt.Sprintf("%v%5d%v %v\n", COLOR_DIM, line.id, COLOR_NONE, line.txt)
				}
				// do not mix files in tty
				mutex.Lock() // ??? required
				fmt.Println(out)
				mutex.Unlock()
			}

			return nil
		})
	if err_d != nil {
		panic(err_d)
	}
	waitg.Wait()

}

/*
	usage examples :
		go run ggrep.go  found /var/log log 	// only in .log files
		go run ggrep.go "for" ~/workspace/Manjaro/manjaro-check-repos
		go run ggrep.go "python" ~/.cache/

	make:
		go build -ldflags "-s -w
			-X main.GitID=$(git rev-parse --short HEAD 2>/dev/null) \
			-X main.GitBranch=$(git branch --show-current 2>/dev/null) \
			-X main.Version=${version} \
			-X main.BuildDate=$(date +%F)" \
			-o ggrep && upx ggrep
*/
