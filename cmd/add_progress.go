package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/adder"
	"github.com/charmbracelet/bubbles/progress"
)

// barProgress returns a ProgressFunc that renders a bubbles progress bar to
// stderr during downloads. It degrades to a byte counter when total is unknown.
func barProgress() adder.ProgressFunc {
	bar := progress.New(progress.WithDefaultGradient())
	// \x1b[K erases to end of line so a shorter status doesn't leave stale text.
	return func(downloaded, total int64) {
		if total <= 0 {
			fmt.Fprintf(os.Stderr, "\r\x1b[KDownloaded %d bytes", downloaded)
			return
		}
		pct := float64(downloaded) / float64(total)
		fmt.Fprintf(os.Stderr, "\r\x1b[K%s", bar.ViewAs(pct))
		if downloaded >= total {
			fmt.Fprintln(os.Stderr)
		}
	}
}
