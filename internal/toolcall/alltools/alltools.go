// Package alltools to load all tools
package alltools

import (
	"github.com/dscli/dscli/internal/toolcall"
	_ "github.com/dscli/dscli/internal/toolcall/ask"
	_ "github.com/dscli/dscli/internal/toolcall/code"
	_ "github.com/dscli/dscli/internal/toolcall/cwd"
	_ "github.com/dscli/dscli/internal/toolcall/file"
	_ "github.com/dscli/dscli/internal/toolcall/flycheck"
	_ "github.com/dscli/dscli/internal/toolcall/history"
	_ "github.com/dscli/dscli/internal/toolcall/mail"
	_ "github.com/dscli/dscli/internal/toolcall/memory"
	_ "github.com/dscli/dscli/internal/toolcall/shell"
	_ "github.com/dscli/dscli/internal/toolcall/skill"
	_ "github.com/dscli/dscli/internal/toolcall/sql"
	_ "github.com/dscli/dscli/internal/toolcall/web"
)

var GetAllTools = toolcall.GetAllTools
