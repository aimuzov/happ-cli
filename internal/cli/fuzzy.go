package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	gofzf "github.com/koki-develop/go-fzf"
)

// fuzzyBar is the left gutter drawn on every item in the embedded finder.
const fuzzyBar = "▌ "

// fuzzySelect shows labels in an fzf-style fuzzy finder and returns the chosen
// 0-based index. It prefers the real fzf binary when present and falls back to
// an embedded finder otherwise. errAborted is returned when the user cancels.
func fuzzySelect(prompt string, labels []string) (int, error) {
	if path, err := exec.LookPath("fzf"); err == nil {
		return fzfSelect(path, prompt, labels)
	}
	return goFuzzySelect(prompt, labels)
}

// fzfSelect drives the external fzf binary. Items are fed as "index\tlabel" so
// the original index survives the round-trip even when labels collide; fzf is
// told to hide and ignore the index column.
func fzfSelect(path, prompt string, labels []string) (int, error) {
	var in bytes.Buffer
	for i, l := range labels {
		fmt.Fprintf(&in, "%d\t%s\n", i, l)
	}
	// --with-nth hides the index column from both the display and the search,
	// while fzf still echoes the full original line (index included) on select.
	cmd := exec.Command(path,
		"--delimiter", "\t",
		"--with-nth", "2..",
		"--prompt", prompt+"> ",
		"--height", "40%",
		"--reverse",
		"--no-multi",
	)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// 130 = aborted (Esc/Ctrl-C), 1 = finished with no selection.
			if code := exit.ExitCode(); code == 130 || code == 1 {
				return 0, errAborted
			}
		}
		return 0, fmt.Errorf("fzf: %w", err)
	}
	return parseFzfSelection(string(out), len(labels))
}

// parseFzfSelection extracts the leading index column from an fzf-selected line.
func parseFzfSelection(out string, n int) (int, error) {
	line := strings.TrimRight(out, "\r\n")
	if line == "" {
		return 0, errAborted
	}
	field := line
	if tab := strings.IndexByte(line, '\t'); tab >= 0 {
		field = line[:tab]
	}
	idx, err := strconv.Atoi(field)
	if err != nil || idx < 0 || idx >= n {
		return 0, fmt.Errorf("fzf returned unexpected selection %q", line)
	}
	return idx, nil
}

// goFuzzySelect is the embedded fallback used when fzf is not installed.
// Styles use ANSI palette indices ("0".."15") and empty (terminal default)
// colors, so the finder follows the terminal's own color scheme instead of
// imposing fixed colors that clash with the theme.
func goFuzzySelect(prompt string, labels []string) (int, error) {
	f, err := gofzf.New(
		gofzf.WithPrompt(prompt+"> "),
		// Drop the "> " pointer column; the left bar is part of every item (see
		// itemFunc below) so a gutter is always present. The bar is neutral by
		// default and picks up the accent on the current row, which go-fzf
		// applies to the whole line.
		gofzf.WithCursor(""),
		gofzf.WithStyles(
			gofzf.WithStyleCursorLine(gofzf.Style{ForegroundColor: "4", Bold: true}),
			gofzf.WithStyleMatches(gofzf.Style{ForegroundColor: "4", Bold: true}),
		),
	)
	if err != nil {
		return 0, err
	}
	idxs, err := f.Find(labels, func(i int) string { return fuzzyBar + labels[i] })
	if err != nil {
		if errors.Is(err, gofzf.ErrAbort) {
			return 0, errAborted
		}
		return 0, err
	}
	if len(idxs) == 0 {
		return 0, errAborted
	}
	return idxs[0], nil
}
