package command

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/peak/s5cmd/v2/storage/url"
	"github.com/urfave/cli/v2"
)

func commandFromContext(c *cli.Context) string {
	cmd := c.Command.FullName()

	for _, f := range c.Command.Flags {
		flagname := f.Names()[0]
		for _, flagvalue := range contextValue(c, flagname) {
			cmd = fmt.Sprintf("%s --%s=%v", cmd, flagname, flagvalue)
		}
	}

	if c.Args().Len() > 0 {
		cmd = fmt.Sprintf("%v %v", cmd, strings.Join(c.Args().Slice(), " "))
	}

	return cmd
}

// contextValue traverses context and its ancestor contexts to find
// the flag value and returns string slice.
func contextValue(c *cli.Context, flagname string) []string {
	for _, c := range c.Lineage() {
		if !c.IsSet(flagname) {
			continue
		}

		val := c.Value(flagname)
		switch val.(type) {
		case cli.StringSlice:
			return c.StringSlice(flagname)
		case cli.Int64Slice, cli.IntSlice:
			values := c.Int64Slice(flagname)
			var result []string
			for _, v := range values {
				result = append(result, strconv.FormatInt(v, 10))
			}
			return result
		case string:
			return []string{c.String(flagname)}
		case bool:
			return []string{strconv.FormatBool(c.Bool(flagname))}
		case int, int64:
			return []string{strconv.FormatInt(c.Int64(flagname), 10)}
		default:
			return []string{fmt.Sprintf("%v", val)}
		}
	}

	return nil
}

// generateCommand generates command string from given context, app command, default flags and urls.
// A default flag with a nil value is omitted from the generated command, even
// if it is set in the given context. A default flag with a []string value is
// repeated once per element.
func generateCommand(c *cli.Context, cmd string, defaultFlags map[string]interface{}, urls ...*url.URL) (string, error) {
	command := AppCommand(cmd)
	flagset := flag.NewFlagSet(command.Name, flag.ContinueOnError)

	var args []string
	for _, url := range urls {
		args = append(args, quoteArg(url.String()))
	}

	flags := []string{}
	for flagname, flagvalue := range defaultFlags {
		switch v := flagvalue.(type) {
		case nil:
			continue
		case []string:
			for _, s := range v {
				flags = append(flags, fmt.Sprintf("--%s='%s'", flagname, s))
			}
		default:
			flags = append(flags, fmt.Sprintf("--%s='%v'", flagname, v))
		}
	}

	isDefaultFlag := func(flagname string) bool {
		_, ok := defaultFlags[flagname]
		return ok
	}

	for _, f := range command.Flags {
		flagname := f.Names()[0]
		if isDefaultFlag(flagname) || !c.IsSet(flagname) {
			continue
		}

		for _, flagvalue := range contextValue(c, flagname) {
			flags = append(flags, fmt.Sprintf("--%s='%s'", flagname, flagvalue))
		}
	}

	sort.Strings(flags)
	flags = append(flags, args...)
	flags = append([]string{command.Name}, flags...)

	err := flagset.Parse(flags)
	if err != nil {
		return "", err
	}

	cmdCtx := cli.NewContext(c.App, flagset, c)
	return strings.TrimSpace(commandFromContext(cmdCtx)), nil
}

// quoteArg quotes s for the command line that run parses with
// shellquote.Split. A plain key is left bare, anything else is single-quoted
// with embedded quotes escaped, so every byte of a key survives the trip:
// shellquote.Join leaves some whitespace (a vertical tab, say) unquoted that
// Split then splits on.
func quoteArg(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !isBareArgRune(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isBareArgRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("_-./:=+@%,", r)
}
