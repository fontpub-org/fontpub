package cli

import "strings"

func commandPath(args []string) []string {
	if len(args) >= 2 && args[0] == "package" {
		return args[:2]
	}
	if len(args) > 0 {
		return args[:1]
	}
	return nil
}

func extractStringFlag(args []string, name string) (string, []string, *CLIError) {
	rest := make([]string, 0, len(args))
	var value string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == name:
			if i+1 >= len(args) {
				return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "missing flag value", Details: map[string]any{"flag": name}}
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(arg, name+"="):
			value = strings.TrimPrefix(arg, name+"=")
		case strings.HasPrefix(arg, "--"):
			return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "unknown flag", Details: map[string]any{"flag": arg}}
		default:
			rest = append(rest, arg)
		}
	}
	return value, rest, nil
}

func extractBoolFlag(args []string, name string) (bool, []string, *CLIError) {
	rest := make([]string, 0, len(args))
	value := false
	for _, arg := range args {
		switch arg {
		case name:
			value = true
		default:
			rest = append(rest, arg)
		}
	}
	return value, rest, nil
}
