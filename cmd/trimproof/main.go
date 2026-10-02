// Command trimproof is the operator CLI.
//
//	trimproof report [-policies policies.json] [-pairs-file eval-pairs.jsonl]
//	                 [-transitions-file transitions.jsonl] [-audit-file audit.jsonl] [-json]
//
// report summarizes each route from the gateway's files: its promotion
// state and transitions, the shadow evaluation evidence the promoter is
// deciding on, and the audit log. The flags default to the gateway's file
// names, so running it in the gateway's working directory needs none.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AkashAgarwalInd/trimproof/internal/report"
	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "report":
		os.Exit(runReport(os.Args[2:]))
	case "version", "-version", "--version":
		fmt.Println("trimproof", version)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: trimproof report [flags]   (trimproof report -h for flags)")
	os.Exit(2)
}

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	var f report.Files
	fs.StringVar(&f.Policies, "policies", "policies.json", "route policy file")
	fs.StringVar(&f.Pairs, "pairs-file", "eval-pairs.jsonl", "shadow evaluation samples (JSONL)")
	fs.StringVar(&f.Transitions, "transitions-file", "transitions.jsonl", "promotion transitions (JSONL)")
	fs.StringVar(&f.Audit, "audit-file", "audit.jsonl", "audit log (JSONL)")
	asJSON := fs.Bool("json", false, "write JSON instead of text")
	_ = fs.Parse(args)

	r, err := report.Build(f, eval.DefaultThresholds())
	if err != nil {
		fmt.Fprintln(os.Stderr, "trimproof report:", err)
		return 1
	}
	if *asJSON {
		err = r.WriteJSON(os.Stdout)
	} else {
		err = r.WriteText(os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "trimproof report:", err)
		return 1
	}
	return 0
}
