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
	defer os.Remove(path)
	if err = Edit(ctx, path); err != nil {
		return content, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return content, err
	}
	content = strings.TrimSpace(string(b))

	// Display result to user.
	outfmt.Printf("File: %s\n", path)
	if content == "" {
		outfmt.Println("(empty file)")
	} else {
		// Escape any triple backticks in content to prevent breaking the
		// markdown code block fence used for display.
		safe := strings.ReplaceAll(content, "```", "'''")
		outfmt.Printf("```\n%s\n```\n", safe)
	}

	return content, err
}

// findEditorBinary splits an EDITOR/VISUAL value into binary and arguments.
// Uses strings.Cut on the first space so that values like "emacsclient -c"
// are correctly parsed as binary="emcsclient", args=["-c"]. Compared to
// strings.Fields, this avoids panicking when the value is empty or
// whitespace-only (Fields returns an empty slice). Note: paths containing
// spaces in the binary name itself are not supported.
func findEditorBinary(editor string) (name string, args []string) {
	name, rest, _ := strings.Cut(editor, " ")
	if rest != "" {
		args = strings.Fields(rest)
	}
	return name, args
}

func withTermEnv() []string {
	// Some editors (emacsclient) fail with "Unknown terminal type" when
	// TERM is set to "dumb" (as Emacs daemon does for subprocesses).
	// Fix: override TERM if it's invalid for interactive use.
	term := os.Getenv("TERM")
	if term != "" && term != "dumb" {
		return nil // existing TERM is fine, inherit normally
	}
	// Replace TERM with a valid value for the editor subprocess.
	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, "TERM=") {
			env[i] = "TERM=xterm-256color"
			return env
		}
	}
	return append(env, "TERM=xterm-256color")
}

func Edit(ctx context.Context, filename string) (err error) {
	editor := getEditor()
	if editor == "" {
		err = fmt.Errorf("no editor specified")
		return err
	}
	name, args := findEditorBinary(editor)
	if name == "" {
		return fmt.Errorf("cannot determine editor binary from: %s", editor)
	}
	args = append(args, filename)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if env := withTermEnv(); env != nil {
		cmd.Env = env
	}
	outfmt.Println(cmd.String())
	if err = cmd.Run(); err != nil {
		return err
	}
	return err
}
