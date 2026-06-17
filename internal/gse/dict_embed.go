//go:build go1.16 && !ne
// +build go1.16,!ne

package gse

import (
	_ "embed"
)

//go:embed data/dict/zh/s_1.txt
var zhS string

//go:embed data/dict/zh/stop_tokens.txt
var stopDict string
