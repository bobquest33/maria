
package main

import (
	"os/exec"
	"strings"
	"log"
	"regexp"
	"fmt"
)

func test1() {
	avprobe("/work/1/a.mp4")
}

func avprobe(path string) (err error, dur float32, w,h int) {
	out, err := exec.Command("ffprobe", path).CombinedOutput()
	if err != nil {
		return
	}
	for _, l := range strings.Split(string(out), "\n") {
		var re *regexp.Regexp
		var ma []string
		re, _ = regexp.Compile(`Duration: (.{11})`)
		ma = re.FindStringSubmatch(l)
		if len(ma) > 1 {
			var h,m,s,ms int
			fmt.Sscanf(ma[1], "%d:%d:%d.%d", &h, &m, &s, &ms)
			dur += float32(h)*3600
			dur += float32(m)*60
			dur += float32(s)
			dur += float32(ms)/100
			log.Printf("dur %v => %f", ma[1], dur)
		}
		re, _ = regexp.Compile(`Video: .* (\d+x\d+)`)
		ma = re.FindStringSubmatch(l)
		if len(ma) > 1 {
			fmt.Sscanf(ma[1], "%dx%d", &w, &h)
			log.Printf("wh %v => %dx%d", ma[1], w, h)
		}
	}
	return
}
