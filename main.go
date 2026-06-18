package main

import (
	"fmt"
	"os"

	"github.com/nanjj/clog"
)

func init() {
	tracer, err := clog.NewTracerWithOptions("Dscli")
	if err != nil {
		panic(err)
	}
	clog.SetGlobalTracer(tracer)
}

func main() {
	defer clog.CloseTracer(clog.GlobalTracer())
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
