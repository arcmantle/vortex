package main

import (
	"fmt"
	"time"
)

func main() {
	spinnerFrames := []string{"-", "\\", "|", "/"}

	fmt.Println("terminal smoke starting")
	fmt.Println("checking carriage returns and line erase...")
	for index := 0; index < 24; index++ {
		frame := spinnerFrames[index%len(spinnerFrames)]
		fmt.Printf("\r\x1b[2K%s streaming %02d/24", frame, index+1)
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\x1b[2K\x1b[32m%s\x1b[0m carriage-return redraw complete\n", "OK")

	fmt.Println()
	fmt.Println("checking cursor-up compaction...")
	fmt.Println("phase: prepare")
	fmt.Println("status: warming cache")
	time.Sleep(300 * time.Millisecond)
	fmt.Print("\x1b[2A\r\x1b[2Kphase: compacted\n\r\x1b[2Kstatus: reused previous lines\n")
	time.Sleep(250 * time.Millisecond)

	fmt.Println()
	fmt.Println("checking colored blocks...")
	for step := 1; step <= 3; step++ {
		fmt.Printf("\x1b[3%dmstep %d active\x1b[0m\n", 1+step, step)
		time.Sleep(120 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("checking overwrite of a growing status line...")
	for percent := 0; percent <= 100; percent += 10 {
		fmt.Printf("\r\x1b[2Kdownload: %3d%% [%s%s]", percent, progressBar(percent/10), progressBarRemainder(10-percent/10))
		time.Sleep(70 * time.Millisecond)
	}
	fmt.Printf("\r\x1b[2K\x1b[36m%s\x1b[0m download complete\n", "DONE")

	fmt.Println()
	fmt.Println("terminal smoke finished")
}

func progressBar(count int) string {
	bar := make([]byte, count)
	for index := range bar {
		bar[index] = '#'
	}
	return string(bar)
}

func progressBarRemainder(count int) string {
	bar := make([]byte, count)
	for index := range bar {
		bar[index] = '-'
	}
	return string(bar)
}
