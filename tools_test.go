package main

import (
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

func TestRunScriptShebang(t *testing.T) {
	tcs := []struct {
		script   string
		out      string
		checkErr func(t *testing.T, err error)
	}{
		{"echo -n hi", "hi", nil},
		{"", "", nil},
		{`#!/usr/bin/env bash
echo -n OK
`, "OK", nil},
		{`#!/usr/bin/env python
print("OK")`, "OK\n", nil},
		{`zzzzzzzz`, "bash:行1: zzzzzzzz：未找到命令\n", func(t *testing.T, err error) {
			if err == nil {
				t.Fatal(err)
			}
			if err.Error() != "exit status 127" {
				t.Fatal(err)
			}
		}},
	}

	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			out, err := runScriptShebang(tc.script, name, arg)

			if tc.checkErr == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				tc.checkErr(t, err)
			}

			if out != tc.out {
				t.Fatal(out, tc.out)
			}
		})
	}
}
