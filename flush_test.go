package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFlushOutput(t *testing.T) {
	// 测试markdown2org转换器的flush逻辑
	t.Run("TestConvertStreamFlush", func(t *testing.T) {
		input := strings.NewReader("# Title\n\nThis is **bold** text.\n\n```go\nfmt.Println(\"Hello\")\n```\n\nNormal text.")
		var output bytes.Buffer

		converter := NewMarkdownToOrgConverter()
		err := converter.ConvertStream(input, &output)
		if err != nil {
			t.Fatalf("ConvertStream failed: %v", err)
		}

		result := output.String()
		expected := "* Title\n\nThis is \u200b*bold*\u200b text.\n\n#+begin_src go\nfmt.Println(\"Hello\")\n#+end_src\n\nNormal text.\n"

		if result != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, result)
		}
	})

	// 测试formatter的flush逻辑
	t.Run("TestTableFormatterFlush", func(t *testing.T) {
		var buf bytes.Buffer
		formatter := NewTableFormatter([]string{"ID", "Name"}, func(data any) []string {
			if m, ok := data.(map[string]string); ok {
				return []string{m["id"], m["name"]}
			}
			return []string{"", ""}
		}).WithWriter(&buf)

		data := []map[string]string{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		}

		_, err := formatter.Format(data)
		if err != nil {
			t.Fatalf("Format failed: %v", err)
		}

		result := buf.String()
		// tabwriter使用空格对齐，具体空格数量可能因环境而异
		// 我们只检查关键内容
		if !strings.Contains(result, "ID") || !strings.Contains(result, "Name") ||
			!strings.Contains(result, "1") || !strings.Contains(result, "Alice") ||
			!strings.Contains(result, "2") || !strings.Contains(result, "Bob") {
			t.Errorf("Table output missing expected content. Got:\n%s", result)
		}
	})
}

func TestMainFlushLogic(t *testing.T) {
	// 测试main.go中的flush逻辑
	t.Run("TestOutputPipeFlush", func(t *testing.T) {
		// 模拟main.go中的管道逻辑
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("Failed to create pipe: %v", err)
		}
		defer r.Close()
		defer w.Close()

		// 启动goroutine模拟转换器
		var output bytes.Buffer
		done := make(chan error, 1)
		go func() {
			converter := NewMarkdownToOrgConverter()
			done <- converter.ConvertStream(r, &output)
		}()

		// 写入数据
		testData := "# Test\n\nSome content.\n"
		_, err = w.Write([]byte(testData))
		if err != nil {
			t.Fatalf("Failed to write to pipe: %v", err)
		}

		// 模拟flush output逻辑 - 写入一个换行符然后关闭
		_, err = w.Write([]byte("\n"))
		if err != nil {
			t.Fatalf("Failed to write newline for flush: %v", err)
		}

		w.Close()

		// 等待转换完成
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ConvertStream failed: %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("ConvertStream timeout")
		}

		// 检查输出
		result := output.String()
		if !strings.Contains(result, "* Test") {
			t.Errorf("Expected output to contain '* Test', got: %s", result)
		}
	})
}
