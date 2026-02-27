package main

import "testing"

func TestPrintln(t *testing.T) {
	println := func(a ...any) (n int, err error) {
		return
	}
	origin := Println
	Println = println
	defer func() {
		Println = origin
	}()
}

func TestPrintf(t *testing.T) {
	printf := func(format string, a ...any) (n int, err error) {
		return
	}
	origin := Printf
	Printf = printf
	defer func() {
		Printf = origin
	}()
}
