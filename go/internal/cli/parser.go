package cli

import "strings"

type parsedArgs struct {
	bools       map[string]bool
	strings     map[string]string
	positionals []string
}

func parseArgs(args []string, boolFlags []string, stringFlags []string) (parsedArgs, *CLIError) {
	out := parsedArgs{
		bools:       map[string]bool{},
		strings:     map[string]string{},
		positionals: make([]string, 0, len(args)),
	}
	boolSet := make(map[string]struct{}, len(boolFlags))
	for _, name := range boolFlags {
		boolSet[name] = struct{}{}
	}
	stringSet := make(map[string]struct{}, len(stringFlags))
	for _, name := range stringFlags {
		stringSet[name] = struct{}{}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if _, ok := boolSet[arg]; ok {
			out.bools[arg] = true
			continue
		}
		if _, ok := stringSet[arg]; ok {
			if i+1 >= len(args) {
				return parsedArgs{}, &CLIError{Code: "INPUT_REQUIRED", Message: "missing flag value", Details: map[string]any{"flag": arg}}
			}
			out.strings[arg] = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--") {
			matched := false
			for name := range stringSet {
				if strings.HasPrefix(arg, name+"=") {
					out.strings[name] = strings.TrimPrefix(arg, name+"=")
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			return parsedArgs{}, &CLIError{Code: "INPUT_REQUIRED", Message: "unknown flag", Details: map[string]any{"flag": arg}}
		}
		out.positionals = append(out.positionals, arg)
	}
	return out, nil
}

func (p parsedArgs) boolValue(name string) bool {
	return p.bools[name]
}

func (p parsedArgs) stringValue(name string) string {
	return p.strings[name]
}

type showOptions struct {
	PackageID string
	Version   string
}

func parseShowOptions(args []string) (showOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, []string{"--version"})
	if err != nil {
		return showOptions{}, err
	}
	if len(parsed.positionals) != 1 {
		return showOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "show requires <owner>/<repo>", Details: map[string]any{}}
	}
	return showOptions{
		PackageID: normalizePackageID(parsed.positionals[0]),
		Version:   parsed.stringValue("--version"),
	}, nil
}

type installOptions struct {
	PackageID     string
	Version       string
	ActivationDir string
	DryRun        bool
	Activate      bool
}

func parseInstallOptions(args []string) (installOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--activate"}, []string{"--activation-dir", "--version"})
	if err != nil {
		return installOptions{}, err
	}
	if len(parsed.positionals) != 1 {
		return installOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "install requires <owner>/<repo>", Details: map[string]any{}}
	}
	return installOptions{
		PackageID:     normalizePackageID(parsed.positionals[0]),
		Version:       parsed.stringValue("--version"),
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
		Activate:      parsed.boolValue("--activate"),
	}, nil
}

type activateOptions struct {
	PackageID     string
	Version       string
	ActivationDir string
	DryRun        bool
}

func parseActivateOptions(args []string) (activateOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run"}, []string{"--version", "--activation-dir"})
	if err != nil {
		return activateOptions{}, err
	}
	if len(parsed.positionals) != 1 {
		return activateOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "activate requires <owner>/<repo>", Details: map[string]any{}}
	}
	return activateOptions{
		PackageID:     normalizePackageID(parsed.positionals[0]),
		Version:       parsed.stringValue("--version"),
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
	}, nil
}
