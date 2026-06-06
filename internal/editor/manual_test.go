package editor

import (
    "context"
    "os"
    "testing"
)

func TestManualOpenEditor(t *testing.T) {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "emacsclient"
    }
    t.Logf("Using EDITOR=%s", editor)
    os.Setenv("EDITOR", editor)

    result, err := OpenEditor(context.Background(), "测试内容\n\n请输入您的回复：")
    if err != nil {
        t.Fatalf("OpenEditor failed: %v", err)
    }
    t.Logf("Result: %s", result)
    if result == "" {
        t.Fatal("empty result")
    }
}
