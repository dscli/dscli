// Package alltools to load all tools
package alltools

import (
	"gitcode.com/dscli/dscli/internal/toolcall"
	_ "gitcode.com/dscli/dscli/internal/toolcall/ask"
	_ "gitcode.com/dscli/dscli/internal/toolcall/code"
	_ "gitcode.com/dscli/dscli/internal/toolcall/issue"
)

var GetAllTools = toolcall.GetAllTools
