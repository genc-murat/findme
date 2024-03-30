package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

type FileWalkerType int

const (
	Current FileWalkerType = iota
	Recursive
)

type FileWalker interface {
	List(dir string, query string, regex bool, r *regexp.Regexp)
}

type CurrentFolderWalker struct{}

func (f *CurrentFolderWalker) List(dir string, query string, regex bool, r *regexp.Regexp) {
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, f := range files {
		fmt.Println(f.Name())
		readFile(dir+"\\"+f.Name(), query, regex, r)

	}
}

type RecursiveFolderWalker struct{}

func (f *RecursiveFolderWalker) List(dir string, query string, regex bool, r *regexp.Regexp) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		fmt.Println(path)
		readFile(path, query, regex, r)
		return nil
	})
	if err != nil {
		fmt.Println(err.Error())
	}
}

type FileWalkerStrategy struct {
	fileWalkers map[FileWalkerType]FileWalker
}

func NewFileWalkerStrategy() *FileWalkerStrategy {
	return &FileWalkerStrategy{
		fileWalkers: make(map[FileWalkerType]FileWalker),
	}
}

func (f *FileWalkerStrategy) Add(workerType FileWalkerType, fileWalker FileWalker) {
	f.fileWalkers[workerType] = fileWalker
}

func (f *FileWalkerStrategy) List(dir string, query string, regex bool, r *regexp.Regexp, walkerType FileWalkerType) {
	if _, ok := f.fileWalkers[walkerType]; !ok {
		fmt.Errorf("unknown walkertype")
	}
	f.fileWalkers[walkerType].List(dir, query, regex, r)
}

func main() {
	var dirPath string
	var query string
	var isRegex bool
	var isRecursive bool
	strategy := NewFileWalkerStrategy()
	strategy.Add(Current, &CurrentFolderWalker{})
	strategy.Add(Recursive, &RecursiveFolderWalker{})
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "dir",
				Value:       "",
				Usage:       "Directory",
				Destination: &dirPath,
			}, &cli.StringFlag{
				Name:        "query",
				Value:       "",
				Usage:       "Search...",
				Destination: &query,
			}, &cli.BoolFlag{
				Name:        "regex",
				Value:       false,
				Usage:       "Regex...",
				Destination: &isRegex,
			}, &cli.BoolFlag{
				Name:        "recursive",
				Value:       false,
				Usage:       "Scan all subfolders",
				Destination: &isRecursive,
			},
		},
		Action: func(c *cli.Context) error {
			var regex *regexp.Regexp
			if isRegex {
				regex, _ = regexp.Compile(query)
			}

			var walkerType FileWalkerType = Current
			if isRecursive {
				walkerType = Recursive
			}

			strategy.List(dirPath, query, isRegex, regex, walkerType)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func readFile(fileName string, query string, regex bool, r *regexp.Regexp) {

	file, err := os.Open(fileName)

	if err != nil {
		fmt.Println(err)
		return
	}

	defer file.Close()
	Process(file, query, regex, r)

}

func Process(f *os.File, query string, regex bool, re *regexp.Regexp) error {

	linesPool := sync.Pool{New: func() interface{} {
		lines := make([]byte, 250*1024)
		return lines
	}}

	stringPool := sync.Pool{New: func() interface{} {
		lines := ""
		return lines
	}}

	r := bufio.NewReader(f)

	var waitGroupFiles sync.WaitGroup

	for {
		buf := linesPool.Get().([]byte)

		n, err := r.Read(buf)
		buf = buf[:n]

		if n == 0 {
			if err != nil {
				fmt.Println(err)
				break
			}
			if err == io.EOF {
				break
			}
			return err
		}

		nextUntillNewline, err := r.ReadBytes('\n')

		if err != io.EOF {
			buf = append(buf, nextUntillNewline...)
		}

		waitGroupFiles.Add(1)
		go func() {
			ProcessChunk(buf, &linesPool, &stringPool, query, f.Name(), regex, re)
			waitGroupFiles.Done()
		}()

	}

	waitGroupFiles.Wait()
	return nil
}

func ProcessChunk(chunk []byte, linesPool *sync.Pool, stringPool *sync.Pool, query string, fileName string, regex bool, r *regexp.Regexp) {

	var waitGroupLines sync.WaitGroup

	lines := string(chunk)

	linesPool.Put(&chunk)

	linesSlice := strings.Split(lines, "\n")

	chunkSize := 300
	n := len(linesSlice)
	noOfThread := n / chunkSize

	if n%chunkSize != 0 {
		noOfThread++
	}

	for i := 0; i < (noOfThread); i++ {

		waitGroupLines.Add(1)
		go func(s int, e int) {
			defer waitGroupLines.Done()
			for i := s; i < e; i++ {
				text := linesSlice[i]
				if len(text) == 0 {
					continue
				}

				if regex {
					if r.MatchString(text) {
						fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
					}
				} else {
					if strings.Contains(text, query) {
						fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
					}
				}

			}

		}(i*chunkSize, int(math.Min(float64((i+1)*chunkSize), float64(len(linesSlice)))))
	}

	waitGroupLines.Wait()
	linesSlice = nil
}
