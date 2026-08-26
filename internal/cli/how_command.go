package cli

import (
	"encoding/json"
	"fmt"
	"github.com/yuechen-li-dev/tspack/internal/how"
	"os"
	"strings"
)

func runHowCommand(args []string) {
	list := false
	jsonOutput := false
	positionals := []string{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--list":
			list = true
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "TSPACK_HOW_INVALID_ARGS: unknown flag %s\n", args[i])
				exit(1)
			}
			positionals = append(positionals, args[i])
		}
	}
	if list {
		items := how.List()
		if jsonOutput {
			type listEntry struct {
				Code  string `json:"code"`
				Title string `json:"title"`
			}
			resp := struct {
				Codes []listEntry `json:"codes"`
			}{Codes: make([]listEntry, 0, len(items))}
			for _, item := range items {
				resp.Codes = append(resp.Codes, listEntry{Code: item.Code, Title: item.Title})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(resp)
			return
		}
		fmt.Println("Known diagnostic help entries:")
		fmt.Println()
		for _, item := range items {
			fmt.Printf("  %-40s %s\n", item.Code, item.Title)
		}
		return
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_HOW_CODE_REQUIRED: diagnostic code is required (or use --list)")
		exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintln(os.Stderr, "TSPACK_HOW_INVALID_ARGS: expected exactly one diagnostic code")
		exit(1)
	}
	entry, ok := how.Lookup(positionals[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "TSPACK_HOW_CODE_NOT_FOUND: unknown diagnostic code %s (run: tspack how --list)\n", positionals[0])
		exit(1)
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(entry)
		return
	}
	fmt.Println(entry.Code)
	fmt.Println()
	fmt.Println(entry.Title)
	fmt.Println()
	fmt.Println("What it means:")
	fmt.Printf("  %s\n", entry.Summary)
	fmt.Println()
	fmt.Println("Why TSPack cares:")
	fmt.Printf("  %s\n", entry.Why)
	if len(entry.CommonCauses) > 0 {
		fmt.Println()
		fmt.Println("Common causes:")
		for _, cause := range entry.CommonCauses {
			fmt.Printf("  - %s\n", cause)
		}
	}
	if len(entry.Fixes) > 0 {
		fmt.Println()
		fmt.Println("How to fix:")
		for _, fix := range entry.Fixes {
			fmt.Printf("  - %s\n", fix)
		}
	}
	if len(entry.BadExamples) > 0 {
		fmt.Println()
		fmt.Println("Bad examples:")
		for _, e := range entry.BadExamples {
			fmt.Printf("  %s:\n%s\n", e.Label, e.Text)
		}
	}
	if len(entry.GoodExamples) > 0 {
		fmt.Println()
		fmt.Println("Good examples:")
		for _, e := range entry.GoodExamples {
			fmt.Printf("  %s:\n%s\n", e.Label, e.Text)
		}
	}
}
