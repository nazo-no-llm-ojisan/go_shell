package main

func buildPwshCommandLine(cmd string, args []resolvedArg, mapped bool) string {
	line := ""
	if mapped {
		line = cmd
	} else {
		line = "& " + pwshQuote(cmd)
	}
	for _, arg := range args {
		if arg.Raw {
			line += " " + arg.Value
		} else {
			line += " " + pwshQuote(arg.Value)
		}
	}
	return line
}

func buildShCommandLine(cmd string, args []resolvedArg, mapped bool) string {
	line := ""
	if mapped {
		line = cmd
	} else {
		line = shQuote(cmd)
	}
	for _, arg := range args {
		line += " " + shQuote(arg.Value)
	}
	return line
}
