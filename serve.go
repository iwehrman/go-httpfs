package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Stats struct {
	Name  string
	Size  int64
	Mtime time.Time
	IsDir bool
}

func main() {

	root, err := os.Getwd()

	if err != nil {
		fmt.Println("Unable to determine root:", err)
		os.Exit(1)
	}

	fmt.Println("Root:", root)

	handleGet := func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		path := query.Get("path")

		if len(path) == 0 || string([]rune(path)[0]) != "/" {
			path = "/" + path
		}

		path = filepath.Clean(path)

		log.Println("Serving:", path)

		fullPath := root + path

		file, err := os.Open(fullPath)
		if err != nil {
			w.WriteHeader(404)
			fmt.Fprintf(w, "File not found: %v", err)
			return
		}

		fileinfo, err := file.Stat()
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "Can't stat file: %v", err)
			return
		}

		if fileinfo.IsDir() {
			infos, err := ioutil.ReadDir(fullPath)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Can't read dir: %v", err)
				return
			}

			stats := make([]*Stats, len(infos))

			for index, info := range infos {
				stat := &Stats{Name: info.Name(), Size: info.Size(), Mtime: info.ModTime(), IsDir: info.IsDir()}
				stats[index] = stat
				log.Printf("Entry: %+v", stat)
				j, _ := json.Marshal(stat)
				log.Printf("JSON: %s", j)
			}

			header := w.Header()
			header.Set("Content-Type", "application/json")

			encodedStats, err := json.Marshal(stats)
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Can't encode stats: %v", err)
			}

			if count, err := w.Write(encodedStats); err != nil {
				log.Printf("Only wrote %v bytes before error: %v\n", count, err)
			} else {
				log.Printf("Wrote %v bytes\n", count)
			}

		} else {

		}

	}

	http.HandleFunc("/get", handleGet)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
