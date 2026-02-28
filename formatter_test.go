package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestTableFormatter(t *testing.T) {
	headers := []string{"Name", "Age", "City"}
	rowFunc := func(data interface{}) []string {
		switch p := data.(type) {
		case Person:
			return []string{p.Name, fmt.Sprintf("%d", p.Age), p.City}
		default:
			return []string{"", "", ""}
		}
	}

	formatter := NewTableFormatter(headers, rowFunc)

	// 测试单个对象
	person := Person{Name: "Alice", Age: 30, City: "New York"}

	// 重定向输出
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	formatter.Format(person)

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)
	output := buf.String()

	// tabwriter 使用空格对齐，而不是制表符
	expectedLines := []string{
		"Name    Age   City",
		"Alice   30    New York",
	}

	for _, line := range expectedLines {
		if !strings.Contains(output, line) {
			t.Errorf("TableFormatter 输出不包含预期行: %q\n完整输出: %q", line, output)
		}
	}
}

func TestJSONFormatter(t *testing.T) {
	formatter := &JSONFormatter{}

	person := Person{Name: "Bob", Age: 25, City: "London"}

	output, err := formatter.Format(person)
	if err != nil {
		t.Fatalf("JSONFormatter.Format 失败: %v", err)
	}

	// 验证JSON输出
	var decoded Person
	err = json.Unmarshal([]byte(output), &decoded)
	if err != nil {
		t.Fatalf("JSONFormatter 输出不是有效的JSON: %v\noutput: %s", err, output)
	}

	if decoded != person {
		t.Errorf("JSONFormatter 输出不匹配\ngot: %+v\nwant: %+v", decoded, person)
	}
}

func TestFormatOutput(t *testing.T) {
	headers := []string{"Name", "Age"}
	rowFunc := func(data interface{}) []string {
		switch p := data.(type) {
		case Person:
			return []string{p.Name, fmt.Sprintf("%d", p.Age)}
		default:
			return []string{"", ""}
		}
	}

	person := Person{Name: "Charlie", Age: 35}

	// 测试表格格式
	err := FormatOutput(person, "table", headers, rowFunc)
	if err != nil {
		t.Errorf("FormatOutput 表格格式失败: %v", err)
	}

	// 测试JSON格式
	err = FormatOutput(person, "json", headers, rowFunc)
	if err != nil {
		t.Errorf("FormatOutput JSON格式失败: %v", err)
	}

	// 测试默认格式（应该是表格）
	err = FormatOutput(person, "", headers, rowFunc)
	if err != nil {
		t.Errorf("FormatOutput 默认格式失败: %v", err)
	}
}

// Person 测试用结构体
type Person struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
	City string `json:"city"`
}
