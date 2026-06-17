package main

import (
	"fmt"
	"os"

	"github.com/nanjj/clog"
)

func init() {
	tracer, err := clog.NewTracer("Dscli")
	if err != nil {
		panic(err)
	}
	clog.SetGlobalTracer(tracer)
}

func main() {
	defer func() {
		tracer := clog.GlobalTracer()
		if tracer != nil {
			tracer.Close()
		}
	}()
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
