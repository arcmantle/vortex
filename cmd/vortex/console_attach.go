package main

import "strings"

func shouldAttachConsoleForCLI(args []string) bool {
	if len(args) == 0 {
		return true
	}

	for _, arg := range args {
		switch arg {
		case "-h", "--help", "help", "-v", "--version", "version":
			return true
		}
	}

	for _, arg := range args {
		if arg == "--" {
			return true
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg != "run"
	}

	return true
}