package main

import (
	"fmt"
	"github.com/ghp3000/screenshot"
	"image"
	"image/png"
	"os"
	"runtime"
	"strconv"
	"time"
)

// save *image.RGBA to filePath with PNG format.
func save(img *image.RGBA, filePath string) {
	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	png.Encode(file, img)
}

/*
	func main() {
		// Capture each displays.
		n := screenshot.NumActiveDisplays()
		if n <= 0 {
			panic("Active display not found")
		}

		var all image.Rectangle = image.Rect(0, 0, 0, 0)

		for i := 0; i < n; i++ {
			bounds := screenshot.GetDisplayBounds(i)
			all = bounds.Union(all)

			img, err := screenshot.CaptureRect(bounds)
			if err != nil {
				panic(err)
			}
			fileName := fmt.Sprintf("%d_%dx%d.png", i, bounds.Dx(), bounds.Dy())
			save(img, fileName)

			fmt.Printf("#%d : %v \"%s\"\n", i, bounds, fileName)
		}

		// Capture all desktop region into an image.
		fmt.Printf("%v\n", all)
		img, err := screenshot.Capture(all.Min.X, all.Min.Y, all.Dx(), all.Dy())
		if err != nil {
			panic(err)
		}
		save(img, "all.png")
	}
*/
func main() {
	runtime.LockOSThread()
	var err error
	shot := screenshot.NewScreenShot(1)
	if err != nil {
		fmt.Println(err)
		return
	}
	shot.DrawCursor(1)
	fmt.Println(shot.GetCaptureName())
	err = shot.Init(0)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer shot.Release()
	start := time.Now()
	for i := 0; i < 10; i++ {
		start = time.Now()
		img, err := shot.Capture()
		//_, err := screenshot.Capture(0, 0, 1920, 1080)
		if err != nil {
			fmt.Println(err)
			//return
		} else {
			fmt.Println(time.Since(start))
			save(img, strconv.FormatInt(time.Now().UnixNano(), 10)+".png")
		}

		//time.Sleep(time.Millisecond * 30)
	}
	fmt.Println(time.Since(start))
}
