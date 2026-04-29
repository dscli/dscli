// Package alltools to load all tools
package alltools

import (
	"gitcode.com/dscli/dscli/internal/toolcall"
	_ "gitcode.com/dscli/dscli/internal/toolcall/ask"
	_ "gitcode.com/dscli/dscli/internal/toolcall/code"
	_ "gitcode.com/dscli/dscli/internal/toolcall/file"
	_ "gitcode.com/dscli/dscli/internal/toolcall/issue"
	_ "gitcode.com/dscli/dscli/internal/toolcall/shell"
	_ "gitcode.com/dscli/dscli/internal/toolcall/skill"
	_ "gitcode.com/dscli/dscli/internal/toolcall/web"
)

var GetAllTools = toolcall.GetAllTools
