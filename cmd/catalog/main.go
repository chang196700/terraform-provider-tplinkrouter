// Command catalog generates local reference documentation or performs an explicit
// read-only audit. The default command does not connect to a router.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
)

func main() {
	command, args := "generate", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	var err error
	switch command {
	case "generate":
		err = generateCommand(args)
	case "audit":
		err = auditCommand(args)
	case "help":
		fmt.Println("Usage: go run ./cmd/catalog [generate|audit] [options]\nDefault: generate local reference documentation without router access.\nUse -h after a command to list its options.")
	default:
		err = errors.New("unknown command; use generate or audit")
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func specsSorted() ([]catalog.Spec, error) {
	specs := catalog.All()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	seen := map[string]bool{}
	for _, spec := range specs {
		if spec.Name == "" || strings.ContainsAny(spec.Name, "/\\.") || seen[spec.Name] {
			return nil, errors.New("catalog contains an empty, unsafe, or duplicate spec name")
		}
		seen[spec.Name] = true
	}
	return specs, nil
}

