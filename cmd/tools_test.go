package cmd

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestShebang(t *testing.T) {
	tcs := []struct {
		script string
		name   string
		arg    []string
	}{
		{"", "/usr/bin/env", []string{"bash"}},
		{"#!/bin/bash\necho OK", "/bin/bash", []string{}},
		{"#!/bin/bash \necho OK", "/bin/bash", []string{}},
	}
	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			if name != tc.name || !reflect.DeepEqual(arg, tc.arg) {
				t.Fatal(name, arg, tc)
			}
		})
	}
}

func TestHandleBash(t *testing.T) {
	type Args struct {
		Script string `json:"script"`
	}

	jsonRawMessage := func(s string) (raw json.RawMessage) {
		v := &Args{
			Script: s,
		}
		b, err := json.Marshal(v)
		if err != nil {
			b = []byte{}
		}
		raw = b
		return
	}

	tcs := []struct {
		script string
		out    string
	}{
		{"echo -n hi", "hi"},
		{"", ""},
		{`#!/usr/bin/env bash
echo -n OK
`, "OK"},
		{`#!/usr/bin/env python
print("OK")`, "OK\n"},
		{`zzzzzzzz`, "执行失败: exit status 127\n输出:\nbash:行1: zzzzzzzz：未找到命令\n"},
	}

	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			raw := jsonRawMessage(tc.script)
			out, err := handleBash(".", raw)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.out {
				t.Fatal(out, tc.out)
			}
		})
	}
}
