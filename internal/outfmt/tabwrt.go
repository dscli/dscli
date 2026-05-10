package outfmt

import (
	"strings"
	"text/tabwriter"
)

type tabwrt struct {
	*tabwriter.Writer
	buf *strings.Builder
}

func NewTabwrt() *tabwrt {
	buf := &strings.Builder{}
	return &tabwrt{
		Writer: tabwriter.NewWriter(buf, 0, 0, 2, ' ', tabwriter.TabIndent),
		buf:    buf,
	}
}

func (t *tabwrt) Println(a ...string) {
	b := []byte(strings.Join(a, "\t") + "\n")
	_, _ = t.Write(b)
}

// Flush flushes the tabwriter and outputs content through mode-aware Print.
func (t *tabwrt) Flush() error {
	err := t.Writer.Flush()
	if err != nil {
		return err
	}
	// Route through Print() to respect --mode flag (markdown/org conversion).
	Print(t.buf.String())
	return nil
}
