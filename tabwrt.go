package main

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

type tabwrt struct {
	*tabwriter.Writer
}

func NewTabwrt() *tabwrt {
	return &tabwrt{
		tabwriter.NewWriter(outputWriter, 0, 0, 2, ' ', tabwriter.TabIndent),
	}
}

func (t *tabwrt) Println(a ...string) {
	b := []byte(strings.Join(a, "\t") + "\n")
	_, _ = t.Write(b)
}

func (t *tabwrt) Flush() error {
	err := t.Writer.Flush()
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}
