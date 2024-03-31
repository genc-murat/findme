package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	for _, file := range files {
		filePath := filepath.Join(dir, file.Name())
		fmt.Println(file.Name())
		readFile(filePath, query, regex, r)
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
	var dirPath, query string
	var isRegex, isRecursive bool

	strategy := NewFileWalkerStrategy()
	strategy.Add(Current, &CurrentFolderWalker{})
	strategy.Add(Recursive, &RecursiveFolderWalker{})

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:  "search",
				Usage: "Search files in a directory",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "dir",
						Aliases:     []string{"d"},
						Usage:       "Directory to search in",
						Destination: &dirPath,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "query",
						Aliases:     []string{"q"},
						Usage:       "Search query",
						Destination: &query,
						Required:    true,
					},
					&cli.BoolFlag{
						Name:        "regex",
						Aliases:     []string{"r"},
						Usage:       "Use regular expression for search",
						Destination: &isRegex,
					},
					&cli.BoolFlag{
						Name:        "recursive",
						Aliases:     []string{"R"},
						Usage:       "Search recursively in subdirectories",
						Destination: &isRecursive,
					},
				},
				Action: func(c *cli.Context) error {
					var regex *regexp.Regexp
					if isRegex {
						regex, _ = regexp.Compile(query)
					}

					walkerType := Current
					if isRecursive {
						walkerType = Recursive
					}

					strategy.List(dirPath, query, isRegex, regex, walkerType)
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func readFile(fileName string, query string, regex bool, r *regexp.Regexp) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		fmt.Println("Error: File", fileName, "does not exist.")
		return
	}

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chunkChan := make(chan []byte)

	numWorkers := runtime.NumGoroutine()
	queryHash := calculateHash(query)

	for i := 0; i < numWorkers; i++ {
		go processChunkWorker(ctx, chunkChan, &linesPool, &stringPool, query, f.Name(), regex, re, queryHash) // Pass queryHash
	}

	r := bufio.NewReader(f)
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

		select {
		case chunkChan <- buf:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	close(chunkChan)
	return nil
}

func processChunkWorker(ctx context.Context, chunkChan <-chan []byte, linesPool *sync.Pool, stringPool *sync.Pool, query string, fileName string, regex bool, r *regexp.Regexp, queryHash uint32) { // Accept queryHash

	for chunk := range chunkChan {
		reader := bufio.NewReader(bytes.NewReader(chunk))
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println(err)
				break
			}

			line = strings.TrimRight(line, "\r\n")
			if len(line) == 0 {
				continue
			}

			if regex {
				if r.MatchString(line) {
					fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
				}
			} else {
				for i := 0; i <= len(line)-len(query); i++ {
					windowHash := calculateHash(line[i : i+len(query)])
					if windowHash == queryHash && line[i:i+len(query)] == query {
						fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
						break
					}
				}
			}
		}

		linesPool.Put(&chunk)
	}
}

func calculateHash(s string) uint32 {
	var hash uint32
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return hash
}
