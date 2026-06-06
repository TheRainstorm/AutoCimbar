//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("AutoCimBar Wails GUI is currently built for Windows. Use encoder/decoder CLI on this platform.")
}
