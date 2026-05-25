package spinner

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Start begins a spinner on stderr with the given message.
// It returns a stop function that clears the spinner line.
// If stderr is not a terminal, no spinner is shown.
func Start(message string) func() {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return func() {}
	}

	var once sync.Once
	done := make(chan struct{})

	go func() {
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			default:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], message)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()

	return func() {
		once.Do(func() { close(done) })
		// Brief pause to let goroutine clear the line.
		time.Sleep(100 * time.Millisecond)
	}
}
