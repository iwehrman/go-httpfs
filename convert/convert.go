package convert

import (
	"os/exec"
	"strconv"
)

var queue = make([]string, 1)
var waiting = make(map[string][]chan string)

func MakeThumbnail(fullPath, thumbPath string, dimension int) error {
	dimAsStr := strconv.Itoa(dimension)
	dimensions := dimAsStr + "x" + dimAsStr

	cmd := exec.Command("convert", "-thumbnail", dimensions, fullPath, thumbPath)
	return cmd.Run()
}
