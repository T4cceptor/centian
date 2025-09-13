package internal

import (
	"fmt"
	"os"
	"time"
)

// Default delay per character.
const defaultDelay = 50.0

// streamPrint prints a formatted string to the console one character at a time,
// with a delay between each character to simulate streaming output.
func StreamPrint(speed float64, format string, a ...any) {
	// First, we format the string with the given arguments.
	// This is important for handling format specifiers like %s, %d, etc.
	s := fmt.Sprintf(format, a...)

	// Now we iterate over the formatted string.
	for _, char := range s {
		fmt.Fprintf(os.Stdout, "%c", char)
		// Pause for the default delay duration.
		delay := time.Duration((defaultDelay / speed) * float64(time.Millisecond))
		time.Sleep(delay)
	}
}
