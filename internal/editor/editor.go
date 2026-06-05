package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
)

func getEditor() (editor string) {
	editor = os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor != "" {
		return editor
	}

	for _, p := range []string{"vi", "nano"} {
		_, err := exec.LookPath(p)
		if err == nil {
			editor = p
			break
		}
	}
	return editor
}

func getExt() (ext string) {
	mode := outfmt.GetOutputMode()
	if mode == "markdown" {
		ext = "md"
	} else {
		ext = "org"
	}
	return ext
}

func createTempfile(initialContent, ext string) (name string, err error) {
	tmpFile, err := os.CreateTemp("", "dscli_editor_*."+ext)
	if err != nil {
		return name, err
	}
	err = tmpFile.Close()
	if err != nil {
		return name, err
	}
	name = tmpFile.Name()
	err = os.WriteFile(name, []byte(initialContent), 0o655)
	if err != nil {
		return name, err
	}
	return name, err
}

func OpenEditor(ctx context.Context, initialContent string) (content string, err error) {
	ext := getExt()
	path, err := createTempfile(initialContent, ext)
	if err != nil {
		return content, err
	}
	defer os.RemoveAll(path)
	if err = Edit(ctx, path); err != nil {
		return content, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return content, err
	}
	content = strings.TrimSpace(string(b))
	return content, err
}

func Edit(ctx context.Context, filename string) (err error) {
	editor := getEditor()
	if editor == "" {
		err = fmt.Errorf("no editor specified")
		return err
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
		return err
	}
	return err
}
