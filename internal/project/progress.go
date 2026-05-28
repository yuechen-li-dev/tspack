package project

import (
	"fmt"
	"io"
)

type Progress struct {
	Enabled bool
	Writer  io.Writer
}

func (p Progress) Step(format string, args ...any) {
	if !p.Enabled || p.Writer == nil {
		return
	}
	fmt.Fprintf(p.Writer, format+"\n", args...)
}
