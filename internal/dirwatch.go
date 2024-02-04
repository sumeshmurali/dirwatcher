// dirwatch contains the logic to watch a given directory for modifications

package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

type Report struct {
	Occurrence   int      // number of times a word has appeared on the directory
	NewFiles     []string // New files added since the previous execution
	RemovedFiles []string // Files removed since the previous execution
	Files        []string // All the files found in the directory
}

// patternCount returns the match count of the regular expression in the given file
func patternCount(file string, r *regexp.Regexp) int {
	b, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	res := r.FindAllIndex(b, -1)
	return len(res)
}

// WatchDirectory traverses the directory and reports the details
func WatchDirectory(dir string, s string, prevFiles []string) Report {

	r := regexp.MustCompile(s)

	in := make(chan string)
	foundFiles := make(map[string]bool)
	newFiles := []string{}

	prevFilesMap := make(map[string]bool)
	for _, file := range prevFiles {
		prevFilesMap[file] = true
	}

	// The below code uses a simple pipeline pattern with the following nodes to generate the report
	// TODO improve pipeline to use buffered channels and worker pools to further speedup the report generation
	// Node 1 - Walks the directory and pushs any files found to in channel
	go func() {
		defer close(in)
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			foundFiles[path] = true
			// Checking if the file already found in the previous run
			_, ok := prevFilesMap[path]
			if !ok {
				newFiles = append(newFiles, path)
			}
			in <- path
			return nil
		})

		if err != nil {
			panic(err)
		}
	}()
	// Node 2 - Takes files as input from the in channel and returns the word occurence to counts channel
	counts := make(chan int)
	go func() {
		defer close(counts)
		for f := range in {
			counts <- patternCount(f, r)
		}
	}()

	// Node 3 - Simply iterates of the counts channel to aggregate word counts to counter
	counter := 0
	for c := range counts {
		counter += c
	}

	deletedFiles := []string{}
	// Find the files that are missing in the current run
	for k := range prevFilesMap {
		_, ok := foundFiles[k]
		if !ok {
			// deleted file
			deletedFiles = append(deletedFiles, k)
		}
	}
	// converting to a list
	files := []string{}
	for k := range foundFiles {
		files = append(files, k)
	}

	return Report{
		Occurrence:   counter,
		NewFiles:     newFiles,
		RemovedFiles: deletedFiles,
		Files:        files,
	}
}
