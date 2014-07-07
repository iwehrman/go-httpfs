package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const thumbDir string = "/.thumbs"

var root string

type Stats struct {
	Name  string    `json:"name"`
	Size  int64     `json:"size"`
	Mtime time.Time `json:"mtime"`
	IsDir bool      `json:"isDir"`
}

func serveDirectoryAtPath(fullPath string, w http.ResponseWriter, r *http.Request) {
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	if !fileInfo.IsDir() {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	infos, err := ioutil.ReadDir(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Can't read dir: %v", err)
		return
	}

	stats := make([]*Stats, len(infos))

	for index, info := range infos {
		stat := &Stats{Name: info.Name(), Size: info.Size(), Mtime: info.ModTime(), IsDir: info.IsDir()}
		stats[index] = stat
		// log.Printf("Entry: %+v", stat)
		// j, _ := json.Marshal(stat)
		// log.Printf("JSON: %s", j)
	}

	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Access-Control-Allow-Origin", "*")

	encodedStats, err := json.Marshal(stats)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Can't encode stats: %v", err)
	}

	if count, err := w.Write(encodedStats); err != nil {
		log.Printf("Only wrote %v bytes before error: %v\n", count, err)
	} else {
		log.Printf("Wrote %v bytes\n", count)
	}
}

func serveFile(file *os.File, fileInfo os.FileInfo, w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Content-Disposition", "filename="+fileInfo.Name())

	if count, err := io.Copy(w, file); err != nil {
		log.Printf("Only wrote %v bytes before error: %v\n", count, err)
	} else {
		log.Printf("Wrote %v bytes\n", count)
	}
}

func serveFileAtPath(fullPath string, fileInfoPtr *os.FileInfo, w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(w, "File not found: %v", err)
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	var fileInfo os.FileInfo
	if fileInfoPtr != nil {
		fileInfo = *fileInfoPtr
	} else {
		fileInfo, err = file.Stat()
		if err != nil {
			file.Close()
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Can't stat file: %v", err)
			return
		}
	}

	if fileInfo.IsDir() {
		file.Close()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	serveFile(file, fileInfo, w, r)
}

func getPathFromRequest(r *http.Request) string {
	query := r.URL.Query()
	path := query.Get("path")

	if len(path) == 0 || string([]rune(path)[0]) != "/" {
		path = "/" + path
	}

	return filepath.Clean(path)
}

func getFullPathFromRequest(r *http.Request) string {
	return root + getPathFromRequest(r)
}

func getThumbPathFromRequest(r *http.Request) string {
	return root + thumbDir + getPathFromRequest(r)
}

func handleGet(w http.ResponseWriter, r *http.Request) {

	fullPath := getFullPathFromRequest(r)

	file, err := os.Open(fullPath)
	if err != nil {
		w.WriteHeader(404)
		fmt.Fprintf(w, "File not found: %v", err)
		return
	}

	fileInfo, err := file.Stat()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Can't stat file: %v", err)
		return
	}

	if fileInfo.IsDir() {
		serveDirectoryAtPath(fullPath, w, r)
	} else {
		serveFile(file, fileInfo, w, r)
	}
}

func handleThumb(w http.ResponseWriter, r *http.Request) {
	fullPath := getFullPathFromRequest(r)
	thumbPath := getThumbPathFromRequest(r)

	if fileInfo, err := os.Stat(thumbPath); err != nil {
		if os.IsNotExist(err) {
			thumbDir := filepath.Dir(thumbPath)
			if err := os.MkdirAll(thumbDir, 0755); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			cmd := exec.Command("convert", "-thumbnail", "400x400", fullPath, thumbPath)
			if err := cmd.Run(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Print("Unable to create thumbnail", err)
				return
			}

			serveFileAtPath(thumbPath, nil, w, r)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			log.Print("Unable to stat thumbnail", err)
			return
		}
	} else {
		serveFileAtPath(thumbPath, &fileInfo, w, r)
	}

}

func handleReaddir(w http.ResponseWriter, r *http.Request) {
	fullPath := getFullPathFromRequest(r)
	serveDirectoryAtPath(fullPath, w, r)
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	fullPath := getFullPathFromRequest(r)
	serveFileAtPath(fullPath, nil, w, r)
}

func initThumbDir() {
	thumbPath := root + thumbDir
	if _, err := os.Stat(thumbPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(thumbPath, 0755); err != nil {
				log.Fatal("Unable to create thumb directory:", err)
			}
		} else {
			log.Fatal("Unable to stat thumb directory:", err)
		}
	}
}

func serve() {
	http.HandleFunc("/read", handleRead)
	http.HandleFunc("/readdir", handleReaddir)
	http.HandleFunc("/get", handleGet)
	http.HandleFunc("/thumb", handleThumb)

	log.Fatal(http.ListenAndServe(":9595", nil))
}

func main() {
	if _root, err := os.Getwd(); err != nil {
		log.Fatal("Unable to determine root")
	} else {
		root = _root
	}

	fmt.Println("Root:", root)

	initThumbDir()

	serve()
}
