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

var mutex = sync.Mutex{}
var queue = make([]string, 0)
var waiting = make(map[string]*thumbInfo)

var workTickets = make(chan bool, MAX_WORKING)

func produceWorkTickets() {
	for {
		select {
		case workTickets <- true:
			log.Print("Produced a work ticket")
		default:
			log.Print("Finished producing work tickets")
			return
		}
	}
}

func acquireWorkTicket() {
	<-workTickets
	log.Print("Acquired a work ticket")
	return
}

func releaseWorkTicket() {
	workTickets <- true
	log.Print("Returned a work ticket")
	return
}

func processEntry(key string) {
	log.Print("Processing entry:", key)

	thumbInfo := waiting[key]

	acquireWorkTicket()

	dimAsStr := strconv.Itoa(thumbInfo.dimension)
	dimensions := dimAsStr + "x" + dimAsStr
	cmd := exec.Command("convert", "-thumbnail", dimensions, thumbInfo.fullPath, thumbInfo.thumbPath)
	result := cmd.Run()

	releaseWorkTicket()

	mutex.Lock()
	for i := 0; i < thumbInfo.callers; i++ {
		thumbInfo.notifier <- result
	}
	delete(waiting, key)
	mutex.Unlock()
}

func enqueueThumbnailRequest(fullPath, thumbPath string, dimension int) <-chan error {
	var notifier chan error

	mutex.Lock()
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

		go processEntry(thumbPath)
	} else {
		log.Print("Updating: " + thumbPath)
		info.callers = info.callers + 1
		notifier = info.notifier
	}
	mutex.Unlock()

	return notifier
}

func MakeThumbnail(fullPath, thumbPath string, dimension int) error {
	notifier := enqueueThumbnailRequest(fullPath, thumbPath, dimension)
	response := <-notifier
	return response
}

func init() {
	go produceWorkTickets()
}
