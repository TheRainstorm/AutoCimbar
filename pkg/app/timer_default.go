//go:build !windows

package app

func beginTimerResolution() func() {
	return func() {}
}
