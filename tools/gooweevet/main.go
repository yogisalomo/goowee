// Command gooweevet checks goowee's reactivity rules (#147):
//
//	go install github.com/yogisalomo/goowee/tools/gooweevet@latest
//	go vet -vettool=$(which gooweevet) ./...
//
// See package analyzer for the checks.
package main

import (
	"github.com/yogisalomo/goowee/tools/gooweevet/analyzer"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(analyzer.Analyzer) }
