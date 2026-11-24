package outbox

import (
	"errors"
	"strings"
)

// The README markers around the generated state diagram. gendoc rewrites what
// lies between them; a test compares it to Machine, so the drawing in the docs
// is always the table the worker enforces.
const (
	readmeBegin = "<!-- BEGIN GENERATED: cluster outbox row -->"
	readmeEnd   = "<!-- END GENERATED: cluster outbox row -->"
)

// readmeDiagram is the README block for the machine.
func readmeDiagram() string {
	return readmeBegin + "\n\n```mermaid\n" + Machine.Mermaid() + "```\n\n" + readmeEnd
}

// ReplaceReadmeDiagram returns the README with the generated block written in,
// or an error when the markers are missing or out of order.
func ReplaceReadmeDiagram(readme string) (string, error) {
	start := strings.Index(readme, readmeBegin)
	if start < 0 {
		return "", errors.New("README is missing the start marker of the outbox diagram")
	}
	// Only the text after the start marker can close the block: an end marker
	// earlier in the file belongs to another block, or is a stray one.
	rest := readme[start+len(readmeBegin):]
	end := strings.Index(rest, readmeEnd)
	if end < 0 {
		return "", errors.New("README is missing the end marker of the outbox diagram")
	}
	end += start + len(readmeBegin) + len(readmeEnd)
	return readme[:start] + readmeDiagram() + readme[end:], nil
}
