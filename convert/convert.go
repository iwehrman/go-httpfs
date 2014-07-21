package convert

import (
	"log"
	"os/exec"
	"strconv"
	"sync"
)

type thumbInfo struct {
	fullPath  string
	thumbPath string
	dimension int
	notifier  chan error
	callers   int
}

const MAX_WORKING = 4

var mutex = sync.RWMutex{}
var queue = make([]string, 0)
var working = make(map[string]bool)
var waiting = make(map[string]*thumbInfo)

func processQueue() {

	mutex.Lock()
	log.Print("Processing queue...")
	key := queue[0]
	queue = queue[1:]
	working[key] = true
	mutex.Unlock()

	mutex.RLock()
	if len(working) < MAX_WORKING && len(queue) > 0 {
		go processQueue()
	}

	thumbInfo := waiting[key]
	mutex.RUnlock()

	dimAsStr := strconv.Itoa(thumbInfo.dimension)
	dimensions := dimAsStr + "x" + dimAsStr
	cmd := exec.Command("convert", "-thumbnail", dimensions, thumbInfo.fullPath, thumbInfo.thumbPath)
	result := cmd.Run()

	for i := 0; i < thumbInfo.callers; {
		thumbInfo.notifier <- result
	}

	mutex.Lock()
	delete(working, key)

	if len(queue) > 0 {
		go processQueue()
	}
	mutex.Unlock()

}

func enqueueThumbnailRequest(fullPath, thumbPath string, dimension int) <-chan error {

	mutex.RLock()

	var notifier chan error
	if info, present := waiting[thumbPath]; !present {
		mutex.RUnlock()
		mutex.Lock()

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
		mutex.Unlock()
		mutex.RLock()
	} else {
		log.Print("Updating: " + thumbPath)
		info.callers = info.callers + 1
		notifier = info.notifier
	}

	log.Print("Queue length: " + strconv.Itoa(len(queue)))

	if len(queue) == 1 {
		go processQueue()
	}
	mutex.RUnlock()

	return notifier
}

func MakeThumbnail(fullPath, thumbPath string, dimension int) error {
	notifier := enqueueThumbnailRequest(fullPath, thumbPath, dimension)
	response := <-notifier
	return response
}
