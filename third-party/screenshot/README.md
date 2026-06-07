screenshot
==========

* 使用go 语言在win上截屏，可自动选择gdi和dxgi模式，也可以手动选择
* 可设置是否绘制鼠标指针
* 支持多显示器
* 使用SIMD做BGRA转RGBA，提升效率
* 分辨率不变化的情况不释放内存，以优化gdi的截图时间
* 这是为后面要实施的远程控制和投屏软件准备的基础库

example
=======

* sample program `main.go`

  ```go
  package main
  
  import (
  "fmt"
  "image"
  "image/png"
  "os"
  "runtime"
  "screenshot"
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
  ```
