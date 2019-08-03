/*
	recursive grep "pattern" in a directory
	scan all text files by default

	env parameter: GGREPLIMIT : number of goroutines that are allowed to run concurrently
*/
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	//https://misc.flogisoft.com/bash/tip_colors_and_formatting
	COLOR_NONE  = "\033[0m"
	COLOR_DIR   = "\033[0;34m"
	COLOR_EXE   = "\033[0;36m"
	COLOR_ERROR = "\033[38;5;124m"
	COLOR_DIM   = "\033[38;5;243m"

	_VERSION = "1.0.0 rc2"
)

var GitBranch string
var Version string
var BuildDate string
var GitID string

// input values
type Options struct {
	mime, help, binError, timer   bool
	sort, directory, pattern, ext string
	limit                         uint64
}

// parse console input values
func argsParser() (Options, error) {
	ret := Options{
		sort:      "none",
		directory: "",
		limit:     32,
	}

	// channel: number of goroutines that are allowed to run concurrently
	// if too many: linux error: too many files opened
	limit, err := strconv.ParseUint(os.Getenv("GGREPLIMIT"), 10, 8)
	if err != nil || limit < 1 {
		ret.limit = 32
	} else {
		ret.limit = limit
	}

	for _, arg := range os.Args[1:] {
		if len(arg) < 1 {
			// user enter empty values : program.go "" "" "" !
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
			case 'b':
				ret.binError = true
			case 'm':
				ret.mime = true
			case 't':
				ret.timer = true
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
	_, err = os.Stat(ret.directory)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func main() {

	options, err := argsParser()
	if err != nil || options.help {
		usage(err, &options)
	}

	if options.timer {
		start := time.Now()
		fmt.Println(":: Time mesure")
		defer func() {
			elapsed := time.Since(start)
			//elapsed.Seconds
			fmt.Printf(":: Time duration: %.2f sec. = %s ", elapsed.Seconds(), elapsed)
		}()
		//defer f()
	}

	dir, _ := filepath.Abs(options.directory)
	fmt.Println("::", dir)

	pattern, err := regexp.Compile(options.pattern)
	if err != nil {
		usage(err, &options)
	}

	var mutex = sync.Mutex{}
	var waitg sync.WaitGroup
	sem := make(chan bool, options.limit)
	var total uint64 = 0
	var count uint64 = 0
	var nblines uint64 = 0

	// FIXME Walk not view symlinks !! is in doc https://golang.org/pkg/path/filepath/#Walk
	// TODO write my recursive function
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
			if strings.Contains(path, "__pycache__") ||
				//strings.Contains(path, "python") || ////////////////////// TEST
				strings.Contains(path, "/.git/") {
				return nil
			}
			if options.ext != "" && (options.ext != filepath.Ext(path)) {
				return nil
			}

			waitg.Add(1)
			go func(filename string, pattern *regexp.Regexp) {

				sem <- true
				defer func() { <-sem }()
				defer waitg.Done()

				freader, err := os.OpenFile(filename, os.O_RDONLY, 0444)
				if err != nil {
					//fmt.Println(err.Error)
					return
				}
				defer freader.Close()

				// test if is a text file ? only if scan all files
				if options.ext == "" && !isTxtSign(freader) {
					//println("binary:", long, filename)
					return
				}

				matchs := grep_stream(bufio.NewReader(freader), pattern)

				mutex.Lock()
				total += 1
				mutex.Unlock()
				// display results for one file
				if len(matchs) > 0 {
					var out = fmt.Sprintln(len(matchs), "in", COLOR_DIR, filename, COLOR_NONE)
					for _, match := range matchs {
						out += match.String()
					}
					mutex.Lock()
					count++
					nblines = nblines + uint64(len(matchs))
					fmt.Println(out)
					mutex.Unlock()
				}
			}(path, pattern)

			return nil
		})
	if err_d != nil {
		panic(err_d)
	}
	waitg.Wait()
	if count > 0 {
		fmt.Printf("\nscan %v files - found %v files - %v lines \n", total, count, nblines)
	}
}

// fields returned by grep_ function
type line struct {
	id  uint64
	txt string
}

func (l *line) String() string {
	return fmt.Sprintf("%v%5d%v %v\n", COLOR_DIM, l.id, COLOR_NONE, l.txt)
}

// grep stream
// return array of lines
func grep_stream(reader *bufio.Reader, pattern *regexp.Regexp) []line {

	returns := []line{}
	scanner := bufio.NewScanner(reader)
	var line_count uint64 = 1
	for scanner.Scan() {
		data := scanner.Text()
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

// read file for test if is not a binary
// from https://mimesniff.spec.whatwg.org/#binary-data-byte
func isTxtSign(reader *os.File) bool {
	buff := make([]byte, 1024)
	_, err := reader.Read(buff)
	if err != nil {
		return false
	}
	reader.Seek(0, 0)
	for _, b := range buff {
		if b == 0x00 {
			continue
		}
		if b <= 0x08 ||
			b == 0x0B ||
			0x0E <= b && b <= 0x1A ||
			0x1C <= b && b <= 0x1F {
			return false
		}
	}
	return true
}

/*
// for grep only text files - now use isTxtSign()
func test_file_type(filename string) bool {
	cmd := exec.Command("file", filename, "-b", "--mime-type")
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), "text")
} */

func usage(err error, options *Options) {
	if Version != "" {
		fmt.Printf("\n%s Version: %v %v %v %v\n", filepath.Base(os.Args[0]), Version, GitID, GitBranch, BuildDate)
	}
	fmt.Printf("Usage: %v \"regex PATTERN\" [directory] [extension]\n\n", filepath.Base(os.Args[0]))
	fmt.Printf("env GGREPLIMIT=%v (allowed to run concurrently)\n", options.limit)
	if err != nil {
		fmt.Println("\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

/*
	usage examples :
		go run ggrep.go  found /var/log log 	// only in .log files
		go run ggrep.go "for" ~/workspace/Manjaro/manjaro-check-repos
		GGREPLIMIT=1 go run ggrep.go "python" ~/.cache/

	make:
		go build -ldflags "-s -w
			-X main.GitID=$(git rev-parse --short HEAD 2>/dev/null) \
			-X main.GitBranch=$(git branch --show-current 2>/dev/null) \
			-X main.Version=${version} \
			-X main.BuildDate=$(date +%F)" \
			-o ggrep && upx ggrep
*/
