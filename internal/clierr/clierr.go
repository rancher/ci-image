// Package clierr defines the shape of errors and warnings the CLI prints, so
// every command reports problems the same way:
//
//	error: <headline>
//	    <key>: <value>
//	    ...
//
// A single problem is a *Detail: a headline plus aligned key/value fields. Several
// independent problems are a *Problems: a headline with a count, then one
// line per problem. Both implement error with their rendered text and no
// "error:"/"warning:" prefix; Render adds the prefix when printing.
package clierr

import (
	"fmt"
	"strings"
)

// Field is one labelled value of a Detail, e.g. {"pinned", "sha256:…"}.
type Field struct {
	Key, Value string
}

// Detail is a single problem: a headline sentence followed by the data it is
// about. Err, when set, is the underlying cause and is reachable via errors.Is
// and errors.As.
type Detail struct {
	Headline string
	Fields   []Field
	Err      error
}

func (d *Detail) Error() string {
	var b strings.Builder
	b.WriteString(d.Headline)
	width := 0
	for _, f := range d.Fields {
		width = max(width, len(f.Key))
	}
	for _, f := range d.Fields {
		fmt.Fprintf(&b, "\n    %-*s %s", width+1, f.Key+":", f.Value)
	}
	return b.String()
}

func (d *Detail) Unwrap() error { return d.Err }

// Problems collects independent problems under one headline.
type Problems struct {
	Headline string
	Items    []string
}

// Add records a problem; the text should be a single line starting with its
// subject, e.g. `tool "yq": missing checksum for platform linux/arm64`.
func (p *Problems) Add(format string, args ...any) {
	p.Items = append(p.Items, fmt.Sprintf(format, args...))
}

// Err returns p as an error, or nil when no problems were recorded.
func (p *Problems) Err() error {
	if len(p.Items) == 0 {
		return nil
	}
	return p
}

func (p *Problems) Error() string {
	noun := "problems"
	if len(p.Items) == 1 {
		noun = "problem"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d %s)", p.Headline, len(p.Items), noun)
	for _, it := range p.Items {
		b.WriteString("\n  " + it)
	}
	return b.String()
}

// Render formats err for the terminal under a level prefix ("error",
// "warning"). Only the first line carries the prefix; continuation lines keep
// their own indentation.
func Render(level string, err error) string {
	return level + ": " + err.Error()
}
