package main

import (
	"context"
	"reflect"
	"testing"
)

func TestShebang(t *testing.T) {
	tcs := []struct {
		script string
		name   string
		arg    []string
	}{
		{"echo hi", "/usr/bin/env", []string{"bash"}},
		{"#!/usr/bin/env bash\necho hi", "/usr/bin/env", []string{"bash"}},
		{"#!/usr/bin/env python\nprint('hi')", "/usr/bin/env", []string{"python"}},
		{"#!/bin/bash\necho hi", "/bin/bash", []string{}},
		{"# comment\necho hi", "/usr/bin/env", []string{"bash"}},
	}
	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			if name != tc.name {
				t.Errorf("name mismatch: want %s, got %s", tc.name, name)
			}
			if !reflect.DeepEqual(arg, tc.arg) {
				t.Errorf("arg mismatch: want %v, got %v", tc.arg, arg)
			}
		})
	}
}

func TestRunScriptShebang(t *testing.T) {
	tcs := []struct {
		script   string
		expected string
		checkErr func(error) bool
	}{
		{"echo -n hi", "hi", nil},
		{"echo -n 'hello world'", "hello world", nil},
		{`#!/usr/bin/env bash
echo -n test`, "test", nil},
		{`#!/usr/bin/env python
print("OK")`, "OK\n", nil},
	}
	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			// 创建包含ToolDisplayName的context
			ctx := context.WithValue(context.Background(), ToolDisplayName, "test-tool")
			out, err := runScript(ctx, tc.script, name, arg)

			if tc.checkErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if out != tc.expected {
					t.Errorf("output mismatch: want %q, got %q", tc.expected, out)
				}
			} else {
				if !tc.checkErr(err) {
					t.Errorf("error mismatch: %v", err)
				}
			}
		})
	}
}
