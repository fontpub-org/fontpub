package cli

func commandPath(args []string) []string {
	if len(args) >= 2 && args[0] == "package" {
		return args[:2]
	}
	if len(args) > 0 {
		return args[:1]
	}
	return nil
}
