package main

import (
	"encoding/json"
	"fmt"
	"github.com/iwehrman/serve/convert"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const thumbDir string = "/.thumbs"
const retinaThumbDir string = "/.thumbs@2x"

var root string

type Stats struct {
	Name  string    `json:"name"`
	Size  int64     `json:"size"`
	Mtime time.Time `json:"mtime"`
	IsDir bool      `json:"isDir"`
}

func hasPreview(r *http.Request) bool {
	query := r.URL.Query()
	_, present := query["preview"]
	return present
}

func hasRetina(r *http.Request) bool {
	query := r.URL.Query()
	_, present := query["retina"]
	return present
}

func getPathFromRequest(r *http.Request) string {
	query := r.URL.Query()
	return query.Get("path")
}

func getFullPathFromRequest(r *http.Request) string {
	path := getPathFromRequest(r)
	return root + path
}

func getThumbPathFromRequest(r *http.Request) (string, bool) {
	path := getPathFromRequest(r)
	thumbPath := root
	retina := hasRetina(r)

	if retina {
		thumbPath = thumbPath + retinaThumbDir
	} else {
		thumbPath = thumbPath + thumbDir
	}

	thumbPath = thumbPath + path

	return thumbPath, retina
}

func canonicalizePath(query url.Values) bool {
	path := query.Get("path")
	isCanon := true

	if len(path) == 0 || string([]rune(path)[0]) != "/" {
		path = "/" + path
		isCanon = false
	}

	canonPath := filepath.Clean(path)
	isCanon = isCanon && (path == canonPath)

	if !isCanon {
		query.Set("path", canonPath)
	}

	return isCanon
}

func canonicalizeBoolean(query url.Values, key string) bool {
	canon := true

	if _, present := query[key]; present {
		value := query.Get(key)
		if value == "" || value == "0" {
			query.Del(key)
			canon = false
		} else if value != "1" {
			query.Set(key, "1")
			canon = false
		}
	}

	return canon
}

func canonicalizeRetina(query url.Values) bool {
	return canonicalizeBoolean(query, "retina")
}

func canonicalizePreview(query url.Values) bool {
	return canonicalizeBoolean(query, "preview")
}

func canonicalizeQuery(url *url.URL, query url.Values) bool {
	newRawQuery := query.Encode()
	isCanon := url.RawQuery == newRawQuery
	url.RawQuery = newRawQuery

	return isCanon
}

func canonicalizeReaddir(url *url.URL) bool {
	canon := true
	query := url.Query()

	canon = canonicalizePath(query) && canon
	canon = canonicalizeQuery(url, query) && canon

	return canon
}

func canonicalizeRead(url *url.URL) bool {
	canon := true
	query := url.Query()

	canon = canonicalizePath(query) && canon
	canon = canonicalizePreview(query) && canon
	canon = canonicalizeRetina(query) && canon
	canon = canonicalizeQuery(url, query) && canon

	return canon
}

func isModified(fileInfo os.FileInfo, header http.Header) bool {
	if _, present := header["If-Modified-Since"]; present {
		lastModified := header.Get("If-Modified-Since")
		lmTime, err := time.Parse(time.RFC1123, lastModified)

		if err != nil {
			log.Printf("Failed to parse if-modified-since header: %s - %s", lastModified, err.Error())
		} else if !lmTime.Before(fileInfo.ModTime()) {
			return false
		}
	}

	return true
}

func setCacheHeaders(fileInfo os.FileInfo, header *http.Header) {
	header.Set("Last-Modified", fileInfo.ModTime().Format(time.RFC1123))
	header.Set("Cache-Control", "private, max-age=0, no-cache")
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

	if header := r.Header; !isModified(fileInfo, header) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Access-Control-Allow-Origin", "*")
	setCacheHeaders(fileInfo, &header)

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
	}

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
	if header := r.Header; !isModified(fileInfo, header) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	header := w.Header()
	setCacheHeaders(fileInfo, &header)
	header.Set("Access-Control-Allow-Origin", "*")
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

func makeThumb(r *http.Request) (string, os.FileInfo, error) {
	thumbPath, retina := getThumbPathFromRequest(r)
	fileInfo, err := os.Stat(thumbPath)

	if err != nil {
		if os.IsNotExist(err) {
			thumbDir := filepath.Dir(thumbPath)
			if err := os.MkdirAll(thumbDir, 0755); err != nil {
				return thumbPath, nil, err
			}

			var dimension int
			if retina {
				dimension = 400
			} else {
				dimension = 200
			}

			fullPath := getFullPathFromRequest(r)
			if err := convert.MakeThumbnail(fullPath, thumbPath, dimension); err != nil {
				log.Print("Unable to create thumbnail", err)
				return thumbPath, nil, err
			}
		} else {
			log.Print("Unable to stat thumbnail", err)
			return thumbPath, nil, err
		}
	}

	return thumbPath, fileInfo, nil
}

func redirect(w http.ResponseWriter, r *http.Request) {
	urlStr := r.URL.RequestURI()
	log.Print("Redirect:" + urlStr)

	header := w.Header()
	header.Set("Access-Control-Allow-Origin", "*")

	http.Redirect(w, r, urlStr, http.StatusMovedPermanently)
}

func handleReaddir(w http.ResponseWriter, r *http.Request) {
	url := r.URL
	canon := canonicalizeReaddir(url)
	if !canon {
		redirect(w, r)
		return
	}

	fullPath := getFullPathFromRequest(r)

	serveDirectoryAtPath(fullPath, w, r)
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	url := r.URL
	canon := canonicalizeRead(url)
	if !canon {
		redirect(w, r)
		return
	}

	var fileInfoPtr *os.FileInfo
	var fullPath string
	if hasPreview(r) {
		thumbPath, fileInfo, err := makeThumb(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		fullPath = thumbPath
		if fileInfo == nil {
			fileInfoPtr = nil
		} else {
			fileInfoPtr = &fileInfo
		}

	} else {
		fullPath = getFullPathFromRequest(r)
		fileInfoPtr = nil
	}

	serveFileAtPath(fullPath, fileInfoPtr, w, r)
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
