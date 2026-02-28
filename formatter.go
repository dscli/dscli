package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"text/template"
)

// Formatter 定义输出格式化接口
type Formatter interface {
	Format(data interface{}) (string, error)
}

// TableFormatter 表格格式化器
type TableFormatter struct {
	headers []string
	rowFunc func(interface{}) []string
	writer  io.Writer
}

// NewTableFormatter 创建表格格式化器
func NewTableFormatter(headers []string, rowFunc func(interface{}) []string) *TableFormatter {
	return &TableFormatter{
		headers: headers,
		rowFunc: rowFunc,
		writer:  os.Stdout,
	}
}

// WithWriter 设置输出写入器
func (f *TableFormatter) WithWriter(w io.Writer) *TableFormatter {
	f.writer = w
	return f
}

// Format 实现表格格式化
func (f *TableFormatter) Format(data interface{}) (string, error) {
	w := tabwriter.NewWriter(f.writer, 0, 0, 3, ' ', 0)

	// 写入表头
	for i, h := range f.headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)

	// 处理数据
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Slice {
		// 处理切片
		for i := 0; i < value.Len(); i++ {
			item := value.Index(i).Interface()
			row := f.rowFunc(item)
			for j, cell := range row {
				if j > 0 {
					fmt.Fprint(w, "\t")
				}
				fmt.Fprint(w, cell)
			}
			fmt.Fprintln(w)
		}
	} else {
		// 处理单个对象
		row := f.rowFunc(data)
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}

	w.Flush()
	return "", nil
}

// JSONFormatter JSON格式化器
type JSONFormatter struct{}

// Format 实现JSON格式化
func (f *JSONFormatter) Format(data interface{}) (string, error) {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// TemplateFormatter 模板格式化器
type TemplateFormatter struct {
	tmpl *template.Template
}

// NewTemplateFormatter 创建模板格式化器
func NewTemplateFormatter(tmplStr string) (*TemplateFormatter, error) {
	tmpl, err := template.New("output").Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	return &TemplateFormatter{tmpl: tmpl}, nil
}

// Format 实现模板格式化
func (f *TemplateFormatter) Format(data interface{}) (string, error) {
	var buf strings.Builder
	err := f.tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatOutput 根据格式类型格式化输出
func FormatOutput(data interface{}, format string, headers []string, rowFunc func(interface{}) []string) error {
	var formatter Formatter
	var err error

	switch format {
	case "json":
		formatter = &JSONFormatter{}
	case "table":
		formatter = NewTableFormatter(headers, rowFunc)
	default:
		// 默认使用表格格式
		formatter = NewTableFormatter(headers, rowFunc)
	}

	output, err := formatter.Format(data)
	if err != nil {
		return err
	}

	if output != "" {
		fmt.Println(output)
	}

	return nil
}

// FormatOutputToWriter 格式化输出到指定的写入器
func FormatOutputToWriter(w io.Writer, data interface{}, format string, headers []string, rowFunc func(interface{}) []string) error {
	var formatter Formatter

	switch format {
	case "json":
		formatter = &JSONFormatter{}
	case "table":
		formatter = NewTableFormatter(headers, rowFunc).WithWriter(w)
	default:
		formatter = NewTableFormatter(headers, rowFunc).WithWriter(w)
	}

	output, err := formatter.Format(data)
	if err != nil {
		return err
	}

	if output != "" {
		fmt.Fprintln(w, output)
	}

	return nil
}
