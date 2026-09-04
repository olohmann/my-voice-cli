package spinner

import (
	"fmt"
	"io"
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
	return start(os.Stderr, message, 80*time.Millisecond)
}

func start(output io.Writer, message string, interval time.Duration) func() {
	var once sync.Once
	done := make(chan struct{})
	cleared := make(chan struct{})

	go func() {
		defer close(cleared)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(output, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(output, "\r%s %s", frames[i%len(frames)], message)
				i++
			}
		}
	}()

	return func() {
		once.Do(func() { close(done) })
		<-cleared
	}
}
