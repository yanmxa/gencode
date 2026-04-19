package core

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	coreDebugFile *os.File
	coreDebugOnce sync.Once
)

func debugLog(format string, args ...any) {
	coreDebugOnce.Do(func() {
		f, err := os.OpenFile("gen_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		coreDebugFile = f
	})
	if coreDebugFile == nil {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(coreDebugFile, "%s  %s\n", ts, msg)
}
