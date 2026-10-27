// Command gencatalog writes the fixed UI choices to the JSON file the web UI imports.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"slate/internal/dynamic"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gencatalog <output.json>")
		os.Exit(2)
	}
	b, err := json.MarshalIndent(dynamic.Catalog(), "", "  ")
	if err == nil {
		err = os.WriteFile(os.Args[1], append(b, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
