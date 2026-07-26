package cli

import (
	"bufio"
	"io"
	"strings"
)

// forEachLine feeds r to fn one line at a time with no upper bound on line
// length, and never stops reading before EOF.
//
// Both runners used bufio.Scanner here. A single line past the scanner's cap
// (64KB default; a coding agent echoing a minified bundle or a stream-json
// event carrying a whole file clears it easily) made Scan return false
// mid-stream with an error nobody checked: the pipe stopped being drained,
// the child blocked writing to it and never exited, and the run hung until
// the task timeout killed it — reporting truncated output as success, with
// the usage block lost in the unread tail.
func forEachLine(r io.Reader, fn func(line string)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			fn(strings.TrimRight(line, "\r\n"))
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// Whatever went wrong, keep the pipe moving so the child can
			// finish and cmd.Wait() can return.
			_, _ = io.Copy(io.Discard, br)
			return err
		}
	}
}
