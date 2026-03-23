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

type lsOptions struct {
	PackageID     string
	ActivationDir string
}

func parseLSOptions(args []string) (lsOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, []string{"--activation-dir"})
	if err != nil {
		return lsOptions{}, err
	}
	if len(parsed.positionals) > 1 {
		return lsOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "ls accepts at most one package id", Details: map[string]any{}}
	}
	opts := lsOptions{ActivationDir: parsed.stringValue("--activation-dir")}
	if len(parsed.positionals) == 1 {
		opts.PackageID = normalizePackageID(parsed.positionals[0])
	}
	return opts, nil
}

type verifyOptions struct {
	PackageID     string
	ActivationDir string
}

func parseVerifyOptions(args []string) (verifyOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, []string{"--activation-dir"})
	if err != nil {
		return verifyOptions{}, err
	}
	if len(parsed.positionals) > 1 {
		return verifyOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "verify accepts at most one package id", Details: map[string]any{}}
	}
	opts := verifyOptions{ActivationDir: parsed.stringValue("--activation-dir")}
	if len(parsed.positionals) == 1 {
		opts.PackageID = normalizePackageID(parsed.positionals[0])
	}
	return opts, nil
}

type deactivateOptions struct {
	PackageID     string
	ActivationDir string
	DryRun        bool
}

func parseDeactivateOptions(args []string) (deactivateOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run"}, []string{"--activation-dir"})
	if err != nil {
		return deactivateOptions{}, err
	}
	if len(parsed.positionals) != 1 {
		return deactivateOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "deactivate requires <owner>/<repo>", Details: map[string]any{}}
	}
	return deactivateOptions{
		PackageID:     normalizePackageID(parsed.positionals[0]),
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
	}, nil
}

type repairOptions struct {
	PackageID     string
	ActivationDir string
	DryRun        bool
}

func parseRepairOptions(args []string) (repairOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--yes"}, []string{"--activation-dir"})
	if err != nil {
		return repairOptions{}, err
	}
	if len(parsed.positionals) > 1 {
		return repairOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "repair accepts at most one package id", Details: map[string]any{}}
	}
	opts := repairOptions{
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
	}
	if len(parsed.positionals) == 1 {
		opts.PackageID = normalizePackageID(parsed.positionals[0])
	}
	return opts, nil
}

type uninstallOptions struct {
	PackageID     string
	Version       string
	ActivationDir string
	DryRun        bool
	All           bool
}

func parseUninstallOptions(args []string) (uninstallOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--yes", "--all"}, []string{"--activation-dir", "--version"})
	if err != nil {
		return uninstallOptions{}, err
	}
	if len(parsed.positionals) != 1 {
		return uninstallOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "uninstall requires <owner>/<repo>", Details: map[string]any{}}
	}
	return uninstallOptions{
		PackageID:     normalizePackageID(parsed.positionals[0]),
		Version:       parsed.stringValue("--version"),
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
		All:           parsed.boolValue("--all"),
	}, nil
}

type updateOptions struct {
	PackageID     string
	ActivationDir string
	DryRun        bool
	Activate      bool
}

func parseUpdateOptions(args []string) (updateOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--activate"}, []string{"--activation-dir"})
	if err != nil {
		return updateOptions{}, err
	}
	if len(parsed.positionals) > 1 {
		return updateOptions{}, &CLIError{Code: "INPUT_REQUIRED", Message: "update accepts at most one package id", Details: map[string]any{}}
	}
	opts := updateOptions{
		ActivationDir: parsed.stringValue("--activation-dir"),
		DryRun:        parsed.boolValue("--dry-run"),
		Activate:      parsed.boolValue("--activate"),
	}
	if len(parsed.positionals) == 1 {
		opts.PackageID = normalizePackageID(parsed.positionals[0])
	}
	return opts, nil
}

type packageInitOptions struct {
	Root      string
	DryRun    bool
	WriteFile bool
	Yes       bool
}

func parsePackageInitOptions(args []string) (packageInitOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--write", "--yes"}, nil)
	if err != nil {
		return packageInitOptions{}, err
	}
	root, errObj := oneOptionalPath(parsed.positionals)
	if errObj != nil {
		return packageInitOptions{}, errObj
	}
	return packageInitOptions{
		Root:      root,
		DryRun:    parsed.boolValue("--dry-run"),
		WriteFile: parsed.boolValue("--write"),
		Yes:       parsed.boolValue("--yes"),
	}, nil
}

type packageValidateOptions struct {
	Root string
}

func parsePackageValidateOptions(args []string) (packageValidateOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, nil)
	if err != nil {
		return packageValidateOptions{}, err
	}
	root, errObj := oneOptionalPath(parsed.positionals)
	if errObj != nil {
		return packageValidateOptions{}, errObj
	}
	return packageValidateOptions{Root: root}, nil
}

type packagePreviewOptions struct {
	Root      string
	PackageID string
}

func parsePackagePreviewOptions(args []string) (packagePreviewOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, []string{"--package-id"})
	if err != nil {
		return packagePreviewOptions{}, err
	}
	root, errObj := oneOptionalPath(parsed.positionals)
	if errObj != nil {
		return packagePreviewOptions{}, errObj
	}
	return packagePreviewOptions{
		Root:      root,
		PackageID: parsed.stringValue("--package-id"),
	}, nil
}

type packageCheckOptions struct {
	Root string
	Tag  string
}

func parsePackageCheckOptions(args []string) (packageCheckOptions, *CLIError) {
	parsed, err := parseArgs(args, nil, []string{"--tag"})
	if err != nil {
		return packageCheckOptions{}, err
	}
	root, errObj := oneOptionalPath(parsed.positionals)
	if errObj != nil {
		return packageCheckOptions{}, errObj
	}
	return packageCheckOptions{
		Root: root,
		Tag:  parsed.stringValue("--tag"),
	}, nil
}

type workflowInitOptions struct {
	Root   string
	DryRun bool
	Yes    bool
}

func parseWorkflowInitOptions(args []string) (workflowInitOptions, *CLIError) {
	parsed, err := parseArgs(args, []string{"--dry-run", "--yes"}, nil)
	if err != nil {
		return workflowInitOptions{}, err
	}
	root, errObj := oneOptionalPath(parsed.positionals)
	if errObj != nil {
		return workflowInitOptions{}, errObj
	}
	return workflowInitOptions{
		Root:   root,
		DryRun: parsed.boolValue("--dry-run"),
		Yes:    parsed.boolValue("--yes"),
	}, nil
}
