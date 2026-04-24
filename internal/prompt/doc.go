// Package prompt provides two system prompts for `chat` and
// `reasoner` model.
//
// # Config directory: system and project

//	System config directory: CONFIG_DIR = ~/.dscli
//
//	Project config directory: PROJECT_CONFIG_DIR =
//	${PROJECT_ROOT}/.dscli
//
// # Prompt for chat model
//
//	chat.md should be in
//	 1. gitcode.com/dscli/dscli/internal/prompt/chat.md - source code, readonly
//	 2. ${CONFIG_DIR}/prompt/chat.md                    - system,      editable
//	 3. ${PROJECT_CONFIG_DIR}/prompt/chat.md            - project,     editable
//
//	If project chat.md exists, it will be respected. Or, if system
//	chat.md exists, it will be respected. And source code chat.md in
//	last priority but can not be edited.
//
// # Prompt for reasoner model
//
//	reasoner.md should be in
//	 1. gitcode.com/dscli/dscli/internal/prompt/reasoner.md - source code, readonly
//	 2. ${CONFIG_DIR}/prompt/reasoner.md                    - system,      editable
//	 3. ${PROJECT_CONFIG_DIR}/prompt/reasoner.md            - project,     editable
//
//	If project reasoner.md exists, it will be respected. Or, if system
//	reasoner.md exists, it will be respected. And source code
//	reasoner.md in last priority but can not be edited.

package prompt
