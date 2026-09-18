package command

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/hashicorp/go-multierror"
	"github.com/kballard/go-shellquote"
	"github.com/urfave/cli/v2"

	"github.com/peak/s5cmd/v2/parallel"
)

var runHelpTemplate = `Name:
	{{.HelpName}} - {{.Usage}}

Usage:
	{{.HelpName}} [file]

Options:
	{{range .VisibleFlags}}{{.}}
	{{end}}
Examples:
	1. Run the commands declared in "commands.txt" file in parallel
		 > s5cmd {{.HelpName}} commands.txt

	2. Read commands from standard input and execute in parallel.
		 > cat commands.txt | s5cmd {{.HelpName}}
`

func NewRunCommand() *cli.Command {
	return &cli.Command{
		Name:               "run",
		HelpName:           "run",
		Usage:              "run commands in batch",
		CustomHelpTemplate: runHelpTemplate,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "exit-on-error",
				Usage: "stop reading commands after the first one fails and cancel the ones still running",
			},
		},
		Before: func(c *cli.Context) error {
			err := validateRunCommand(c)
			if err != nil {
				printError(commandFromContext(c), c.Command.Name, err)
			}
			return err
		},
		Action: func(c *cli.Context) error {
			reader := os.Stdin
			if c.Args().Len() == 1 {
				f, err := os.Open(c.Args().First())
				if err != nil {
					printError(commandFromContext(c), c.Command.Name, err)
					return err
				}
				defer f.Close()

				reader = f
			}

			return NewRun(c, reader).Run(c.Context)
		},
	}
}

type Run struct {
	c      *cli.Context
	reader io.Reader

	// flags
	numWorkers  int
	exitOnError bool
}

func NewRun(c *cli.Context, r io.Reader) Run {
	return Run{
		c:           c,
		reader:      r,
		numWorkers:  c.Int("numworkers"),
		exitOnError: c.Bool("exit-on-error"),
	}
}

func (r Run) Run(ctx context.Context) error {
	pm := parallel.New(r.numWorkers)
	defer pm.Close()

	// With --exit-on-error the first failing command cancels this context:
	// the reader stops handing out lines and the commands still running
	// see the cancellation.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var stopped atomic.Bool
	waiter := parallel.NewWaiter(parallel.WithErrorHandler(func(error) {
		if r.exitOnError {
			stopped.Store(true)
			cancel()
		}
	}))

	reader := NewReader(ctx, r.reader)

	// A quoted argument may hold a newline: an object key or file name with
	// one in it is written by sync as 'first line
	// second line'. Like a shell, keep reading until the quote closes.
	// pending holds the lines read so far of such a command.
	var pending string
	var parseErr error

	lineno := -1
	for raw := range reader.Read() {
		lineno++

		line := strings.TrimSpace(pending + raw)
		if pending == "" {
			if line == "" {
				continue
			}

			// skip comment lines
			if strings.HasPrefix(line, "#") {
				continue
			}
		}

		fields, err := shellquote.Split(line)
		if isUnterminatedQuote(err) {
			pending += raw
			continue
		}
		pending = ""
		if err != nil {
			parseErr = fmt.Errorf("line %v: %w", lineno, err)
			break
		}

		if len(fields) == 0 {
			continue
		}

		if fields[0] == "run" {
			err := fmt.Errorf("%q command (line: %v) is not permitted in run-mode", "run", lineno)
			printError(commandFromContext(r.c), r.c.Command.Name, err)
			continue
		}

		fn := func() error {
			subcmd := fields[0]

			cmd := AppCommand(subcmd)
			if cmd == nil {
				err := fmt.Errorf("%q command (line: %v) not found", subcmd, lineno)
				printError(commandFromContext(r.c), r.c.Command.Name, err)
				return nil
			}

			flagset := flag.NewFlagSet(subcmd, flag.ExitOnError)
			if err := flagset.Parse(fields); err != nil {
				printError(commandFromContext(r.c), r.c.Command.Name, err)
				return nil
			}

			cliCtx := cli.NewContext(app, flagset, r.c)
			cliCtx.Context = ctx
			return cmd.Run(cliCtx)
		}

		pm.Run(fn, waiter)
	}

	if parseErr == nil && pending != "" {
		// the input ended inside a quoted argument
		_, err := shellquote.Split(strings.TrimSpace(pending))
		parseErr = fmt.Errorf("line %v: %w", lineno, err)
	}
	if parseErr != nil {
		// nothing after a line that cannot be parsed is run, whatever
		// --exit-on-error says: the input is not what it looks like.
		printError(commandFromContext(r.c), r.c.Command.Name, parseErr)
		stopped.Store(true)
		cancel()
	}

	merrorWaiter := waiter.Wait()

	readErr := reader.Err()
	if stopped.Load() && errors.Is(readErr, context.Canceled) {
		// we stopped the reader ourselves; the command that failed has
		// already been reported.
		readErr = nil
	}
	if readErr != nil {
		printError(commandFromContext(r.c), r.c.Command.Name, readErr)
	}

	return multierror.Append(merrorWaiter, readErr, parseErr).ErrorOrNil()
}

// isUnterminatedQuote reports whether shellquote.Split stopped at the end of
// a line inside a quoted or escaped argument, which the next line may finish.
func isUnterminatedQuote(err error) bool {
	return errors.Is(err, shellquote.UnterminatedSingleQuoteError) ||
		errors.Is(err, shellquote.UnterminatedDoubleQuoteError) ||
		errors.Is(err, shellquote.UnterminatedEscapeError)
}

// Reader is a cancelable reader.
type Reader struct {
	*bufio.Reader
	err    error
	linech chan string
	ctx    context.Context
}

// NewReader creates a new reader with cancellation.
func NewReader(ctx context.Context, r io.Reader) *Reader {
	reader := &Reader{
		ctx:    ctx,
		Reader: bufio.NewReader(r),
		linech: make(chan string),
	}

	go reader.read()
	return reader
}

// read reads lines from the underlying reader.
func (r *Reader) read() {
	defer close(r.linech)

	for {
		select {
		case <-r.ctx.Done():
			r.err = r.ctx.Err()
			return
		default:
			// If ReadString encounters an error before finding a delimiter,
			// it returns the data read before the error and the error itself (often io.EOF).
			line, err := r.ReadString('\n')
			if line != "" {
				select {
				case r.linech <- line:
				case <-r.ctx.Done():
					r.err = r.ctx.Err()
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					if errors.Is(r.ctx.Err(), context.Canceled) {
						r.err = r.ctx.Err()
					}
					return
				}
				r.err = multierror.Append(r.err, err)
			}
		}
	}
}

// Read returns read-only channel to consume lines.
func (r *Reader) Read() <-chan string {
	return r.linech
}

// Err returns encountered errors, if any.
func (r *Reader) Err() error {
	return r.err
}

func validateRunCommand(c *cli.Context) error {
	if c.Args().Len() > 1 {
		return fmt.Errorf("expected only 1 file")
	}
	return nil
}
