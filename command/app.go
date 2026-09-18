package command

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/peak/s5cmd/v2/log"
	"github.com/peak/s5cmd/v2/log/stat"
	"github.com/peak/s5cmd/v2/parallel"
	"github.com/peak/s5cmd/v2/storage"
)

const (
	defaultWorkerCount = 256
	defaultRetryCount  = 10

	appName = "s5cmd"
)

var app = &cli.App{
	Name:                 appName,
	Usage:                "Blazing fast S3 and local filesystem execution tool",
	EnableBashCompletion: true,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "enable JSON formatted output",
		},
		&cli.IntFlag{
			Name:  "numworkers",
			Value: defaultWorkerCount,
			Usage: "number of workers execute operation on each object",
		},
		&cli.IntFlag{
			Name:    "retry-count",
			Aliases: []string{"r"},
			Value:   defaultRetryCount,
			Usage:   "number of times that a request will be retried for failures",
		},
		&cli.StringFlag{
			Name:    "endpoint-url",
			Usage:   "override default S3 host for custom services",
			EnvVars: []string{"S3_ENDPOINT_URL"},
		},
		&cli.BoolFlag{
			Name:  "no-verify-ssl",
			Usage: "disable SSL certificate verification",
		},
		&cli.GenericFlag{
			Name: "log",
			Value: &EnumValue{
				Enum:    []string{"trace", "debug", "info", "error"},
				Default: "info",
			},
			Usage: "log level: (trace, debug, info, error)",
		},
		&cli.StringFlag{
			Name:  "log-file",
			Usage: "write logs to the specified file instead of stdout/stderr",
		},
		&cli.BoolFlag{
			Name:  "install-completion",
			Usage: "get completion installation instructions for your shell (only available for bash, pwsh, and zsh)",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "fake run; show what commands will be executed without actually executing them",
		},
		&cli.BoolFlag{
			Name:  "stat",
			Usage: "collect statistics of program execution and display it at the end",
		},
		&cli.BoolFlag{
			Name:  "no-sign-request",
			Usage: "do not sign requests: credentials will not be loaded if --no-sign-request is provided",
		},
		&cli.BoolFlag{
			Name:  "use-list-objects-v1",
			Usage: "use ListObjectsV1 API for services that don't support ListObjectsV2",
		},
		&cli.StringFlag{
			Name:  "request-payer",
			Usage: "who pays for request (access requester pays buckets)",
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "use the specified profile from the credentials file",
		},
		&cli.StringFlag{
			Name:  "credentials-file",
			Usage: "use the specified credentials file instead of the default credentials file",
		},
		&cli.GenericFlag{
			Name:  "addressing-style",
			Usage: "use virtual host style or path style endpoint: (path, virtual)",
			Value: &EnumValue{
				Enum:    []string{"path", "virtual"},
				Default: "path",
			},
			EnvVars: []string{"S3_ADDRESSING_STYLE"},
		},
	},
	Before: func(c *cli.Context) error {
		retryCount := c.Int("retry-count")
		workerCount := c.Int("numworkers")
		printJSON := c.Bool("json")
		logLevel := c.String("log")
		logFile := c.String("log-file")
		isStat := c.Bool("stat")
		endpointURL := c.String("endpoint-url")

		if err := log.Init(logLevel, printJSON, log.WithLogFile(logFile)); err != nil {
			return err
		}
		parallel.Init(workerCount)

		if retryCount < 0 {
			err := fmt.Errorf("retry count cannot be a negative value")
			printError(commandFromContext(c), c.Command.Name, err)
			return err
		}
		if c.Bool("no-sign-request") && c.String("profile") != "" {
			err := fmt.Errorf(`"no-sign-request" and "profile" flags cannot be used together`)
			printError(commandFromContext(c), c.Command.Name, err)
			return err
		}
		if c.Bool("no-sign-request") && c.String("credentials-file") != "" {
			err := fmt.Errorf(`"no-sign-request" and "credentials-file" flags cannot be used together`)
			printError(commandFromContext(c), c.Command.Name, err)
			return err
		}

		if isStat {
			stat.InitStat()
		}

		if endpointURL != "" {
			if !strings.HasPrefix(endpointURL, "http") {
				err := fmt.Errorf(`bad value for --endpoint-url %v: scheme is missing. Must be of the form http://<hostname>/ or https://<hostname>/`, endpointURL)
				printError(commandFromContext(c), c.Command.Name, err)
				return err
			}
		}

		return nil
	},
	CommandNotFound: func(c *cli.Context, command string) {
		msg := log.ErrorMessage{
			Command: command,
			Err:     "command not found",
		}
		log.Error(msg)

		// After callback is not called if app exists with cli.Exit.
		parallel.Close()
		log.Close()
	},
	OnUsageError: onUsageError,
	Action: func(c *cli.Context) error {
		if c.Bool("install-completion") {
			printAutocompletionInstructions(os.Getenv("SHELL"))
			return nil
		}
		args := c.Args()
		if args.Present() {
			cli.ShowCommandHelp(c, args.First())
			return cli.Exit("", 1)
		}

		return cli.ShowAppHelp(c)
	},
	After: func(c *cli.Context) error {
		if c.Bool("stat") && len(stat.Statistics()) > 0 {
			log.Stat(stat.Statistics())
		}

		parallel.Close()
		log.Close()
		return nil
	},
}

// onUsageError reports a usage error (unknown flag, bad flag value, ...) on
// stderr and points the user at the relevant help. Without it, urfave/cli
// prints "Incorrect Usage" and the full help text to stdout, which corrupts
// redirected output such as `s5cmd ls --json s3://bucket > out.json`.
func onUsageError(c *cli.Context, err error, _ bool) error {
	if err == nil {
		return nil
	}

	_, _ = fmt.Fprintf(c.App.ErrWriter, "Incorrect Usage: %v\n", err)
	_, _ = fmt.Fprintf(c.App.ErrWriter, "See '%s --help' for usage\n", helpCommandFromContext(c))
	return err
}

// helpCommandFromContext returns the "s5cmd <command>" path whose --help
// output applies to the given context, or just "s5cmd" at the top level.
//
// urfave/cli runs a command that has subcommands (e.g. select) as a nested
// App named "<parent app> <command>", so App.Name already carries the path up
// to the current command.
func helpCommandFromContext(c *cli.Context) string {
	name := c.App.Name
	if c.Command != nil && c.Command.Name != "" {
		name += " " + c.Command.Name
	}
	return name
}

// setUsageErrorHandler makes every command (and subcommand) that does not
// define its own OnUsageError report usage errors on stderr.
func setUsageErrorHandler(cmds []*cli.Command) {
	for _, cmd := range cmds {
		if cmd.OnUsageError == nil {
			cmd.OnUsageError = onUsageError
		}
		setUsageErrorHandler(cmd.Subcommands)
	}
}

// NewStorageOpts creates storage.Options object from the given context.
func NewStorageOpts(c *cli.Context) storage.Options {
	return storage.Options{
		DryRun:                 c.Bool("dry-run"),
		Endpoint:               c.String("endpoint-url"),
		MaxRetries:             c.Int("retry-count"),
		NoSignRequest:          c.Bool("no-sign-request"),
		NoVerifySSL:            c.Bool("no-verify-ssl"),
		RequestPayer:           c.String("request-payer"),
		UseListObjectsV1:       c.Bool("use-list-objects-v1"),
		Profile:                c.String("profile"),
		CredentialFile:         c.String("credentials-file"),
		LogLevel:               log.LevelFromString(c.String("log")),
		NoSuchUploadRetryCount: c.Int("no-such-upload-retry-count"),
		AddressingStyle:        c.String("addressing-style"),
	}
}

func Commands() []*cli.Command {
	return []*cli.Command{
		NewListCommand(),
		NewCopyCommand(),
		NewDeleteCommand(),
		NewMoveCommand(),
		NewMakeBucketCommand(),
		NewRemoveBucketCommand(),
		NewSelectCommand(),
		NewSizeCommand(),
		NewCatCommand(),
		NewPipeCommand(),
		NewRunCommand(),
		NewSyncCommand(),
		NewVersionCommand(),
		NewBucketVersionCommand(),
		NewPresignCommand(),
		NewHeadCommand(),
	}
}

func AppCommand(name string) *cli.Command {
	for _, c := range Commands() {
		if c.HasName(name) {
			return c
		}
	}

	return nil
}

// Main is the entrypoint function to run given commands.
func Main(ctx context.Context, args []string) error {
	app.Commands = Commands()
	setUsageErrorHandler(app.Commands)

	return app.RunContext(ctx, args)
}
