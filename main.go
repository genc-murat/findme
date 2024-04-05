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
	"github.com/spaolacci/murmur3"
	"github.com/urfave/cli/v2"
)

type FileWalkerType int

const (
	Current FileWalkerType = iota
	Recursive
)

type FileWalker interface {
	List(dir string, query string, regex bool, r *regexp.Regexp, caseInsensitive, wholeWord bool)
}

type CurrentFolderWalker struct{}

func (f *CurrentFolderWalker) List(dir string, query string, regex bool, r *regexp.Regexp, caseInsensitive, wholeWord bool) {
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, file := range files {
		filePath := filepath.Join(dir, file.Name())
		fmt.Println(file.Name())
		readFile(filePath, query, regex, r, caseInsensitive, wholeWord)
	}
}

type RecursiveFolderWalker struct{}

func (f *RecursiveFolderWalker) List(dir string, query string, regex bool, r *regexp.Regexp, caseInsensitive, wholeWord bool) {
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, file := range files {
		filePath := filepath.Join(dir, file.Name())
		fmt.Println(file.Name())
		readFile(filePath, query, regex, r, caseInsensitive, wholeWord)
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

func (f *FileWalkerStrategy) List(dir string, query string, regex bool, r *regexp.Regexp, walkerType FileWalkerType, caseInsensitive, wholeWord bool) {
	if _, ok := f.fileWalkers[walkerType]; !ok {
		fmt.Errorf("unknown walkertype")
	}
	f.fileWalkers[walkerType].List(dir, query, regex, r, caseInsensitive, wholeWord)
}

func main() {
	var dirPath, query string
	var isRegex, isRecursive, caseInsensitive, wholeWord bool

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
					&cli.BoolFlag{
						Name:        "case-insensitive",
						Aliases:     []string{"i"},
						Usage:       "Perform case-insensitive search",
						Destination: &caseInsensitive,
					},
					&cli.BoolFlag{
						Name:        "whole-word",
						Aliases:     []string{"w"},
						Usage:       "Match whole words only",
						Destination: &wholeWord,
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

					err := parallelListAndRead(dirPath, query, isRegex, regex, walkerType, caseInsensitive, wholeWord)
					return err
				},
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func parallelListAndRead(dirPath, query string, regex bool, r *regexp.Regexp, walkerType FileWalkerType, caseInsensitive, wholeWord bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to send file paths for reading
	fileChan := make(chan string)

	// Start goroutines to list files concurrently
	var wgList sync.WaitGroup
	numWorkers := runtime.NumGoroutine()
	for i := 0; i < numWorkers; i++ {
		wgList.Add(1)
		go listFiles(ctx, dirPath, query, regex, r, walkerType, fileChan, &wgList)
	}

	// Start goroutines to read files concurrently
	var wgRead sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wgRead.Add(1)
		go readFileWorker(ctx, fileChan, query, regex, r, &wgRead, caseInsensitive, wholeWord)
	}

	// Wait for file listing to complete
	wgList.Wait()
	close(fileChan)

	// Wait for file reading to complete
	wgRead.Wait()
	return nil
}

// listFiles lists files based on the walkerType and sends file paths to the channel.
func listFiles(ctx context.Context, dirPath, query string, regex bool, r *regexp.Regexp, walkerType FileWalkerType, fileChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	strategy := NewFileWalkerStrategy()
	strategy.Add(Current, &CurrentFolderWalker{})
	strategy.Add(Recursive, &RecursiveFolderWalker{})
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			select {
			case fileChan <- path:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err.Error())
	}
}

func readFileWorker(ctx context.Context, fileChan <-chan string, query string, regex bool, r *regexp.Regexp, wg *sync.WaitGroup, caseInsensitive, wholeWord bool) {
	defer wg.Done()

	for {
		select {
		case fileName, ok := <-fileChan:
			if !ok {
				return // Channel closed
			}
			readFile(fileName, query, regex, r, caseInsensitive, wholeWord)

		case <-ctx.Done():
			return // Context canceled
		}
	}
}

func readFile(fileName string, query string, regex bool, r *regexp.Regexp, caseInsensitive, wholeWord bool) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		fmt.Printf("Error: File %s does not exist.\n", fileName)
		return
	}
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Printf("Error opening file %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	// Use bufio.Reader for efficient file reading
	reader := bufio.NewReader(file)
	Process(reader, query, regex, r, fileName, caseInsensitive, wholeWord)
}

func Process(reader *bufio.Reader, query string, regex bool, re *regexp.Regexp, fileName string, caseInsensitive, wholeWord bool) error {
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

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go processChunkWorker(ctx, chunkChan, &linesPool, &stringPool, query, fileName, regex, re, queryHash, &wg, caseInsensitive, wholeWord)
	}

	for {
		buf := linesPool.Get().([]byte)
		n, err := reader.Read(buf)
		buf = buf[:n]
		if n == 0 {
			if err != nil {
				if err != io.EOF {
					fmt.Println(err)
				}
				break
			}
			return err
		}

		nextUntilNewline, err := reader.ReadBytes('\n')
		if err != io.EOF {
			buf = append(buf, nextUntilNewline...)
		}

		select {
		case chunkChan <- buf:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	close(chunkChan)
	wg.Wait()
	return nil
}

func processChunkWorker(ctx context.Context, chunkChan <-chan []byte, linesPool *sync.Pool, stringPool *sync.Pool, query string, fileName string, regex bool, r *regexp.Regexp, queryHash uint32, wg *sync.WaitGroup, caseInsensitive, wholeWord bool) {
	defer wg.Done()

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				return
			}

			scanner := bufio.NewScanner(bytes.NewReader(chunk))
			for scanner.Scan() {
				line := scanner.Text()
				line = strings.TrimRight(line, "\r\n")
				if len(line) == 0 {
					continue
				}

				var lineStr string
				if v := stringPool.Get(); v != nil {
					lineStr = v.(string)
				} else {
					lineStr = ""
				}
				lineStr = line

				if regex {
					if r.MatchString(lineStr) {
						fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
					}
				} else {
					if caseInsensitive {
						lineStr = strings.ToLower(lineStr)
						query = strings.ToLower(query)
					}

					if wholeWord {
						query = fmt.Sprintf("\\b%s\\b", query)
						r, _ = regexp.Compile(query)
						if r.MatchString(lineStr) {
							fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
						}
					} else {
						for i := 0; i <= len(lineStr)-len(query); i++ {
							windowHash := calculateHash(lineStr[i : i+len(query)])
							if windowHash == queryHash && lineStr[i:i+len(query)] == query {
								fmt.Println(color.Error.Sprintf("%s %s", query, fileName))
								break
							}
						}
					}
				}

				stringPool.Put(&lineStr)
			}

			if err := scanner.Err(); err != nil {
				fmt.Printf("Error scanning chunk: %v\n", err)
			}

			linesPool.Put(&chunk)

		case <-ctx.Done():
			return
		}
	}
}

func calculateHash(s string) uint32 {
	return murmur3.Sum32([]byte(s))
}
