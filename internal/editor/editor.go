package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func getEditor() (editor string) {
	editor = os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor != "" {
		return
	}

	for _, p := range []string{"vi", "nano"} {
		_, err := exec.LookPath(p)
		if err == nil {
			editor = p
			break
		}
	}
	return
}

func getExt() (ext string) {
	mode := outfmt.GetOutputMode()
	if mode == "markdown" {
		ext = "md"
	} else {
		ext = "org"
	}
	return
}

func createTempfile(initialContent string, ext string) (name string, err error) {
	tmpFile, err := os.CreateTemp("", "dscli_editor_*."+ext)
	if err != nil {
		return
	}
	err = tmpFile.Close()
	if err != nil {
		return
	}
	name = tmpFile.Name()
	err = os.WriteFile(name, []byte(initialContent), 0o655)
	if err != nil {
		return
	}
	return
}

func OpenEditor(ctx context.Context, initialContent string) (content string, err error) {

	ext := getExt()
	path, err := createTempfile(initialContent, ext)
	if err != nil {
		return
	}
	defer os.RemoveAll(path)
	if err = Edit(ctx, path); err != nil {
		return
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content = strings.TrimSpace(string(b))
	return
}

func Edit(ctx context.Context, filename string) (err error) {
	editor := getEditor()
	if editor == "" {
		err = fmt.Errorf("no editor specified")
		return
	}
	cmdParts := strings.Fields(editor)
	name := cmdParts[0]
	args := cmdParts[1:]
	args = append(args, filename)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	outfmt.Println(cmd.String())
	if err = cmd.Run(); err != nil {
		return
	}
	return
}
