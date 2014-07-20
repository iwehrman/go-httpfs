package convert

import (
	"log"
	"os/exec"
	"strconv"
)

type thumbInfo struct {
	fullPath  string
	thumbPath string
	dimension int
	notifier  chan error
	callers   int
}

const MAX_WORKING = 4

var queue = make([]string, 0)
var working = make(map[string]bool)
var waiting = make(map[string]*thumbInfo)

func processQueue() {
	log.Print("Processing queue...")
	key := queue[0]
	queue = queue[1:]
	working[key] = true

	if len(working) < MAX_WORKING && len(queue) > 0 {
		go processQueue()
	}

	thumbInfo := waiting[key]
	dimAsStr := strconv.Itoa(thumbInfo.dimension)
	dimensions := dimAsStr + "x" + dimAsStr
	cmd := exec.Command("convert", "-thumbnail", dimensions, thumbInfo.fullPath, thumbInfo.thumbPath)
	result := cmd.Run()

	for i := 0; i < thumbInfo.callers; {
		thumbInfo.notifier <- result
	}

	delete(working, key)

	if len(queue) > 0 {
		go processQueue()
	}
}

func enqueueThumbnailRequest(fullPath, thumbPath string, dimension int) <-chan error {
	var notifier chan error
	if info, present := waiting[thumbPath]; !present {
		log.Print("Initializing: " + thumbPath)
		notifier = make(chan error, 1)
		waiting[thumbPath] = &thumbInfo{
			fullPath:  fullPath,
			thumbPath: thumbPath,
			dimension: dimension,
			notifier:  notifier,
			callers:   1,
		}

		queue = append(queue, thumbPath)
	} else {
		log.Print("Updating: " + thumbPath)
		info.callers = info.callers + 1
		notifier = info.notifier
	}

	log.Print("Queue length: " + strconv.Itoa(len(queue)))

	if len(queue) == 1 {
		go processQueue()
	}

	return notifier
}

func MakeThumbnail(fullPath, thumbPath string, dimension int) error {
	notifier := enqueueThumbnailRequest(fullPath, thumbPath, dimension)
	response := <-notifier
	return response
}
