package spinner

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStopClearsWithoutFixedDelay(t *testing.T) {
	var output bytes.Buffer
	stop := start(&output, "Thinking...", time.Hour)

	started := time.Now()
	stop()
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("stop took %s, expected synchronized immediate cleanup", elapsed)
	}
	if !strings.Contains(output.String(), "\033[K") {
		t.Fatalf("spinner did not clear its line: %q", output.String())
	}
}
