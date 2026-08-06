package cmd

import (
	"fmt"
	"os"
	"time"
)

// spinnerFrames is the indeterminate cycle rendered while a slow store scan
// runs. The scan duration is not known upfront, so no determinate bar exists;
// this is the "infinity" progress indicator.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// withSpinner runs fn while animating an indeterminate progress indicator on
// stderr. It renders only when stderr is a terminal; pipes, redirected output,
// and scripted runs stay clean. fn's result is returned unchanged.
func withSpinner[T any](label string, fn func() T) T {
	if !stderrIsTerminal() {
		return fn()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			fmt.Fprintf(os.Stderr, "\r%s %s ", spinnerFrames[i%len(spinnerFrames)], label)
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}()
	res := fn()
	close(stop)
	<-done
	fmt.Fprint(os.Stderr, "\r\x1b[2K") // erase the spinner line
	return res
}

// stderrIsTerminal reports whether stderr is a character device (a terminal),
// which is the only case where an animated spinner is worth rendering.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
