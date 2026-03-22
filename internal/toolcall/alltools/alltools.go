// Package alltools to load all tools
package alltools

import (
	"gitcode.com/dscli/dscli/internal/toolcall"
	_ "gitcode.com/dscli/dscli/internal/toolcall/code"
)

var GetAllTools = toolcall.GetAllTools
