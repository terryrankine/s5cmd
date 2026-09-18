package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/lanrat/extsort"
	"github.com/urfave/cli/v2"

	errorpkg "github.com/peak/s5cmd/v2/error"
	"github.com/peak/s5cmd/v2/log/stat"
	"github.com/peak/s5cmd/v2/storage"
	"github.com/peak/s5cmd/v2/storage/url"
)

const (
	extsortChannelBufferSize = 1_000
	extsortChunkSize         = 100_000
)

var syncHelpTemplate = `Name:
	{{.HelpName}} - {{.Usage}}

Usage:
	{{.HelpName}} [options] source destination

Options:
	{{range .VisibleFlags}}{{.}}
	{{end}}
Examples:
	01. Sync local folder to s3 bucket
		 > s5cmd {{.HelpName}} folder/ s3://bucket/

	02. Sync S3 bucket to local folder
		 > s5cmd {{.HelpName}} "s3://bucket/*" folder/

	03. Sync S3 bucket objects under prefix to S3 bucket.
		 > s5cmd {{.HelpName}} "s3://sourcebucket/prefix/*" s3://destbucket/

	04. Sync local folder to S3 but delete the files that S3 bucket has but local does not have.
		 > s5cmd {{.HelpName}} --delete folder/ s3://bucket/

	05. Sync S3 bucket to local folder but use size as only comparison criteria.
		 > s5cmd {{.HelpName}} --size-only "s3://bucket/*" folder/

	06. Sync a file to S3 bucket
		 > s5cmd {{.HelpName}} myfile.gz s3://bucket/

	07. Sync matching S3 objects to another bucket
		 > s5cmd {{.HelpName}} "s3://bucket/*.gz" s3://target-bucket/prefix/

	08. Perform KMS Server Side Encryption of the object(s) at the destination
		 > s5cmd {{.HelpName}} --sse aws:kms s3://bucket/object s3://target-bucket/prefix/object

	09. Perform KMS-SSE of the object(s) at the destination using customer managed Customer Master Key (CMK) key id
		 > s5cmd {{.HelpName}} --sse aws:kms --sse-kms-key-id <your-kms-key-id> s3://bucket/object s3://target-bucket/prefix/object

	10. Sync all files to S3 bucket but exclude the ones with txt and gz extension
		 > s5cmd {{.HelpName}} --exclude "*.txt" --exclude "*.gz" dir/ s3://bucket

	11. Sync all files to S3 bucket but include only the ones with txt and gz extension
		 > s5cmd {{.HelpName}} --include "*.txt" --include "*.gz" dir/ s3://bucket
`

func NewSyncCommandFlags() []cli.Flag {
	syncFlags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "delete",
			Usage: "delete objects in destination but not in source",
		},
		&cli.BoolFlag{
			Name:  "size-only",
			Usage: "make size of object only criteria to decide whether an object should be synced",
		},
		&cli.BoolFlag{
			Name:  "exit-on-error",
			Usage: "stop after the first cp or rm fails instead of continuing with the remaining objects (an error while listing always stops the sync)",
		},
	}
	sharedFlags := NewSharedFlags()
	return append(syncFlags, sharedFlags...)
}

func NewSyncCommand() *cli.Command {
	cmd := &cli.Command{
		Name:               "sync",
		HelpName:           "sync",
		Usage:              "sync objects",
		Flags:              NewSyncCommandFlags(),
		CustomHelpTemplate: syncHelpTemplate,
		Before: func(c *cli.Context) error {
			// sync command share same validation method as copy command
			err := validateCopyCommand(c)
			if err != nil {
				printError(commandFromContext(c), c.Command.Name, err)
			}
			return err
		},
		Action: func(c *cli.Context) (err error) {
			defer stat.Collect(c.Command.FullName(), &err)()

			s, err := NewSync(c)
			if err != nil {
				return err
			}
			return s.Run(c)
		},
	}

	cmd.BashComplete = getBashCompleteFn(cmd, false, false)
	return cmd
}

type ObjectPair struct {
	src, dst *storage.Object
}

// Sync holds sync operation flags and states.
type Sync struct {
	src         string
	dst         string
	op          string
	fullCommand string

	// flags
	delete   bool
	sizeOnly bool
	exclude  []string
	include  []string

	// patterns
	excludePatterns []*regexp.Regexp
	includePatterns []*regexp.Regexp

	// s3 options
	storageOpts storage.Options

	followSymlinks        bool
	storageClass          storage.StorageClass
	raw                   bool
	forceGlacierTransfer  bool
	ignoreGlacierWarnings bool

	srcRegion string
	dstRegion string

	// errs collects the errors reported while listing and planning. Run
	// sets it and folds it into its result.
	errs *syncErrors
}

// syncErrors records the errors sync reports while it lists objects and
// plans commands. Those steps run in goroutines that print as they go and
// cannot return an error, so without this record a printed ERROR could
// still end in exit code 0.
type syncErrors struct {
	mu    sync.Mutex
	count int
	first error
}

func (e *syncErrors) add(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.first == nil {
		e.first = err
	}
	e.count++
}

// err summarises the recorded errors, or returns nil if there were none.
func (e *syncErrors) err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.count {
	case 0:
		return nil
	case 1:
		return e.first
	default:
		return fmt.Errorf("%w (and %d more errors)", e.first, e.count-1)
	}
}

// NewSync creates Sync from cli.Context
func NewSync(c *cli.Context) (Sync, error) {
	fullCommand := commandFromContext(c)

	exclude, err := patternsFromContext(c, "exclude")
	if err != nil {
		printError(fullCommand, c.Command.Name, err)
		return Sync{}, err
	}

	include, err := patternsFromContext(c, "include")
	if err != nil {
		printError(fullCommand, c.Command.Name, err)
		return Sync{}, err
	}

	return Sync{
		src:         c.Args().Get(0),
		dst:         c.Args().Get(1),
		op:          c.Command.Name,
		fullCommand: fullCommand,

		// flags
		delete:   c.Bool("delete"),
		sizeOnly: c.Bool("size-only"),
		exclude:  exclude,
		include:  include,

		// flags
		followSymlinks:        !c.Bool("no-follow-symlinks"),
		storageClass:          storage.StorageClass(c.String("storage-class")),
		raw:                   c.Bool("raw"),
		forceGlacierTransfer:  c.Bool("force-glacier-transfer"),
		ignoreGlacierWarnings: c.Bool("ignore-glacier-warnings"),
		// region settings
		srcRegion:   c.String("source-region"),
		dstRegion:   c.String("destination-region"),
		storageOpts: NewStorageOpts(c),
	}, nil
}

// Run compares files, plans necessary s5cmd commands to execute
// and executes them in order to sync source to destination.
func (s Sync) Run(c *cli.Context) error {
	srcurl, err := url.New(s.src, url.WithRaw(s.raw))
	if err != nil {
		return err
	}

	dsturl, err := url.New(s.dst, url.WithRaw(s.raw))
	if err != nil {
		return err
	}

	s.excludePatterns, err = createRegexFromWildcard(s.exclude)
	if err != nil {
		printError(s.fullCommand, s.op, err)
		return err
	}

	s.includePatterns, err = createRegexFromWildcard(s.include)
	if err != nil {
		printError(s.fullCommand, s.op, err)
		return err
	}

	ctx, cancel := context.WithCancel(c.Context)
	defer cancel()

	s.errs = &syncErrors{}

	sourceObjects, destObjects, err := s.getSourceAndDestinationObjects(ctx, cancel, srcurl, dsturl)
	if err != nil {
		printError(s.fullCommand, s.op, err)
		return err
	}

	isBatch := srcurl.IsWildcard()
	if !isBatch && !srcurl.IsRemote() {
		sourceClient, err := storage.NewClient(ctx, srcurl, s.storageOpts)
		if err != nil {
			return err
		}

		obj, err := sourceClient.Stat(ctx, srcurl)
		if err != nil {
			return err
		}

		isBatch = obj != nil && obj.Type.IsDir()
	}

	onlySource, onlyDest, commonObjects := compareObjects(sourceObjects, destObjects, isBatch)

	sourceObjects = nil
	destObjects = nil

	strategy := NewStrategy(s.sizeOnly) // create comparison strategy.
	pipeReader, pipeWriter := io.Pipe() // create a reader, writer pipe to pass commands to run

	// Create commands in background.
	go s.planRun(ctx, c, onlySource, onlyDest, commonObjects, dsturl, strategy, pipeWriter, isBatch)

	// The generated cp/rm commands report their own failures through Run's
	// result. Add the errors the listing and planning goroutines printed;
	// unless the run was cancelled, they all finished before the command
	// pipe was closed.
	err = NewRun(c, pipeReader).Run(ctx)
	return multierror.Append(err, s.errs.err()).ErrorOrNil()
}

// reportError prints err and records it so that Run exits non-zero.
func (s Sync) reportError(err error) {
	// printError does not print cancelation errors; do not record them either.
	if errorpkg.IsCancelation(err) {
		return
	}
	printError(s.fullCommand, s.op, err)
	s.errs.add(err)
}

// compareObjects compares source and destination objects. It assumes that
// sourceObjects and destObjects channels are already sorted in ascending order.
// Returns objects those in only source, only destination
// and both.
func compareObjects(sourceObjects, destObjects chan *storage.Object, isSrcBatch bool) (chan *url.URL, chan *url.URL, chan *ObjectPair) {
	var (
		srcOnly   = make(chan *url.URL, extsortChannelBufferSize)
		dstOnly   = make(chan *url.URL, extsortChannelBufferSize)
		commonObj = make(chan *ObjectPair, extsortChannelBufferSize)
		srcName   string
		dstName   string
	)

	go func() {
		src, srcOk := <-sourceObjects
		dst, dstOk := <-destObjects

		defer close(srcOnly)
		defer close(dstOnly)
		defer close(commonObj)

		for {
			if srcOk {
				srcName = filepath.ToSlash(src.URL.Relative())
				if !isSrcBatch {
					srcName = src.URL.Base()
				}
			}
			if dstOk {
				dstName = filepath.ToSlash(dst.URL.Relative())
			}

			if srcOk && dstOk {
				if srcName < dstName {
					srcOnly <- src.URL
					src, srcOk = <-sourceObjects
				} else if srcName == dstName { // if there is a match.
					commonObj <- &ObjectPair{src: src, dst: dst}
					src, srcOk = <-sourceObjects
					dst, dstOk = <-destObjects
				} else {
					dstOnly <- dst.URL
					dst, dstOk = <-destObjects
				}
			} else if srcOk {
				srcOnly <- src.URL
				src, srcOk = <-sourceObjects
			} else if dstOk {
				dstOnly <- dst.URL
				dst, dstOk = <-destObjects
			} else /* if !srcOK && !dstOk */ {
				break
			}
		}
	}()

	return srcOnly, dstOnly, commonObj
}

// getSourceAndDestinationObjects returns source and destination objects from
// given URLs. The returned channels gives objects sorted in ascending order
// with respect to their url.Relative path. See also storage.Less.
func (s Sync) getSourceAndDestinationObjects(ctx context.Context, cancel context.CancelFunc, srcurl, dsturl *url.URL) (chan *storage.Object, chan *storage.Object, error) {
	// Create source client with source region
	srcOpts := s.storageOpts
	if s.srcRegion != "" {
		srcOpts.SetRegion(s.srcRegion)
	}
	sourceClient, err := storage.NewClient(ctx, srcurl, srcOpts)
	if err != nil {
		return nil, nil, err
	}

	// Create destination client with destination region
	dstOpts := s.storageOpts
	if s.dstRegion != "" {
		dstOpts.SetRegion(s.dstRegion)
	}
	destClient, err := storage.NewClient(ctx, dsturl, dstOpts)
	if err != nil {
		return nil, nil, err
	}

	// add * to end of destination string, to get all objects recursively.
	var destinationURLPath string
	if strings.HasSuffix(s.dst, "/") {
		destinationURLPath = s.dst + "*"
	} else {
		destinationURLPath = s.dst + "/*"
	}

	destObjectsURL, err := url.New(destinationURLPath)
	if err != nil {
		return nil, nil, err
	}

	var (
		sourceObjects = make(chan *storage.Object, extsortChannelBufferSize)
		destObjects   = make(chan *storage.Object, extsortChannelBufferSize)
	)

	extsortDefaultConfig := extsort.DefaultConfig()
	extsortConfig := &extsort.Config{
		ChunkSize:          extsortChunkSize,
		NumWorkers:         extsortDefaultConfig.NumWorkers,
		ChanBuffSize:       extsortChannelBufferSize,
		SortedChanBuffSize: extsortChannelBufferSize,
	}
	extsortDefaultConfig = nil

	// get source objects.
	go func() {
		defer close(sourceObjects)
		unfilteredSrcObjectChannel := sourceClient.List(ctx, srcurl, s.followSymlinks)
		filteredSrcObjectChannel := make(chan storage.Object, extsortChannelBufferSize)

		go func() {
			defer close(filteredSrcObjectChannel)
			// filter and redirect objects
			for st := range unfilteredSrcObjectChannel {
				// An entry that cannot be read (a dangling symlink, a
				// directory without read permission) comes with its URL. If
				// the user excluded it, the listing lacks nothing the sync
				// would have used: skip it without a word.
				if st.Err != nil && st.URL != nil && s.isFilteredOut(st.URL.Path, srcurl.Prefix) {
					continue
				}
				if isListingError(st.Err) {
					// the source listing is incomplete, so no correct plan
					// can be made from it: report the error and stop.
					s.reportError(st.Err)
					cancel()
					continue
				}
				if s.shouldSkipSrcObject(st, true) {
					continue
				}
				// --exclude/--include are relative to the source prefix. An
				// object filtered out here is never copied; if it also exists
				// in the destination it becomes "only destination" and the
				// delete step applies the same filters, so it is kept.
				if s.isFilteredOut(st.URL.Path, srcurl.Prefix) {
					continue
				}
				filteredSrcObjectChannel <- *st
			}
		}()

		sorter, srcOutputChan, srcErrCh := extsort.Generic[storage.Object](
			filteredSrcObjectChannel, storage.FromBytes, storage.Object.ToBytes, storage.Compare, extsortConfig,
		)
		sorter.Sort(ctx)

		for srcObject := range srcOutputChan {
			sourceObjects <- &srcObject
		}

		// read and print the external sort errors
		// a sort error leaves the sorted source incomplete; like a listing
		// error it must stop the sync, or --delete would plan from it.
		for err := range srcErrCh {
			s.reportError(err)
			cancel()
		}
	}()

	// get destination objects.
	go func() {
		defer close(destObjects)
		unfilteredDestObjectsChannel := destClient.List(ctx, destObjectsURL, false)
		filteredDstObjectChannel := make(chan storage.Object, extsortChannelBufferSize)

		go func() {
			defer close(filteredDstObjectChannel)
			// filter and redirect objects
			for dt := range unfilteredDestObjectsChannel {
				if isListingError(dt.Err) {
					// the destination listing is incomplete. Going on would
					// treat the destination as (partly) empty: every source
					// object would be copied again and --delete would remove
					// nothing, or the wrong objects. Report the error and stop.
					s.reportError(dt.Err)
					cancel()
					continue
				}
				if s.shouldSkipDstObject(dt, false) {
					continue
				}
				filteredDstObjectChannel <- *dt
			}
		}()

		dstSorter, dstOutputChan, dstErrCh := extsort.Generic[storage.Object](
			filteredDstObjectChannel, storage.FromBytes, storage.Object.ToBytes, storage.Compare, extsortConfig,
		)
		dstSorter.Sort(ctx)

		for destObject := range dstOutputChan {
			destObjects <- &destObject
		}

		// read and print the external sort errors
		for err := range dstErrCh {
			s.reportError(err)
			cancel()
		}
	}()

	return sourceObjects, destObjects, nil
}

// planRun prepares the commands and writes them to writer 'w'.
func (s Sync) planRun(
	ctx context.Context,
	c *cli.Context,
	onlySource, onlyDest chan *url.URL,
	common chan *ObjectPair,
	dsturl *url.URL,
	strategy SyncStrategy,
	w io.WriteCloser,
	isBatch bool,
) {
	defer w.Close()

	// Always use raw mode since sync command generates commands
	// from raw S3 objects. Otherwise, generated copy command will
	// try to expand given source.
	//
	// --exclude and --include (and the patterns read from --exclude-from
	// and --include-from) are already applied to the source listing,
	// relative to the source prefix. Omit them from the generated cp
	// command: a raw URL has no prefix, so cp would match the patterns
	// against the full path instead.
	defaultFlags := map[string]interface{}{
		"raw":          true,
		"exclude":      nil,
		"include":      nil,
		"exclude-from": nil,
		"include-from": nil,
	}

	// it should wait until both of the child goroutines for onlySource and common channels
	// are completed before closing the WriteCloser w to ensure that all URLs are processed.
	var wg sync.WaitGroup

	// only in source
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case srcurl, ok := <-onlySource:
				if !ok {
					return
				}
				curDestURL, err := generateDestinationURL(srcurl, dsturl, isBatch)
				if err != nil {
					s.reportError(err)
					continue
				}
				command, err := generateCommand(c, "cp", defaultFlags, srcurl, curDestURL)
				if err != nil {
					printDebug(s.op, err, srcurl, curDestURL)
					continue
				}
				fmt.Fprintln(w, command)
			}
		}
	}()

	// both in source and destination
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case commonObject, ok := <-common:
				if !ok {
					return
				}
				sourceObject, destObject := commonObject.src, commonObject.dst
				curSourceURL, curDestURL := sourceObject.URL, destObject.URL
				err := strategy.ShouldSync(sourceObject, destObject) // check if object should be copied.
				if err != nil {
					printDebug(s.op, err, curSourceURL, curDestURL)
					continue
				}

				command, err := generateCommand(c, "cp", defaultFlags, curSourceURL, curDestURL)
				if err != nil {
					printDebug(s.op, err, curSourceURL, curDestURL)
					continue
				}
				fmt.Fprintln(w, command)
			}
		}
	}()

	// only in destination
	wg.Add(1)
	go func() {
		defer wg.Done()
		if s.delete {
			// unfortunately we need to read them all!
			// or rewrite generateCommand function?
			dstURLs := make([]*url.URL, 0, extsortChunkSize)

			for {
				done := false
				select {
				case <-ctx.Done():
					return
				case d, ok := <-onlyDest:
					if !ok {
						done = true
					} else {
						// objects filtered out by --exclude/--include are not part
						// of the sync, so they must not be deleted from the
						// destination either.
						if s.isFilteredOut(d.Path, dsturl.Prefix) {
							continue
						}
						dstURLs = append(dstURLs, d)
					}
				}
				if done {
					break
				}
			}

			// a listing error cancels the context. The objects seen until
			// then are only part of the picture, so nothing may be deleted
			// based on them.
			if ctx.Err() != nil || len(dstURLs) == 0 {
				return
			}

			// --exclude and --include are already applied above, relative to
			// the destination prefix. Omit them from the generated rm command,
			// which would match them against the full object key instead.
			rmFlags := map[string]interface{}{
				"raw":          true,
				"exclude":      nil,
				"include":      nil,
				"exclude-from": nil,
				"include-from": nil,
			}

			command, err := generateCommand(c, "rm", rmFlags, dstURLs...)
			if err != nil {
				printDebug(s.op, err, dstURLs...)
				return
			}
			fmt.Fprintln(w, command)
		} else {
			// we only need to consume them from the channel so that rest of the objects
			// can be sent to channel.
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-onlyDest:
					if !ok {
						return
					}
				}
			}
		}
	}()

	wg.Wait()
}

// generateDestinationURL generates destination url for given
// source url if it would have been in destination.
func generateDestinationURL(srcurl, dsturl *url.URL, isBatch bool) (*url.URL, error) {
	objname := srcurl.Base()
	if isBatch {
		objname = srcurl.Relative()
	}

	if dsturl.IsRemote() {
		if dsturl.IsPrefix() || dsturl.IsBucket() {
			return dsturl.Join(objname), nil
		}
		return dsturl.Clone(), nil

	}

	return dsturl.JoinInside(objname)
}

// isFilteredOut reports whether the object at path, taken relative to
// prefix, is left out of the sync by --exclude/--include: it is excluded
// when an exclude pattern matches, or when include patterns are given and
// none of them match.
func (s Sync) isFilteredOut(path, prefix string) bool {
	if len(s.excludePatterns) > 0 && isURLMatched(s.excludePatterns, path, prefix) {
		return true
	}
	if len(s.includePatterns) > 0 && !isURLMatched(s.includePatterns, path, prefix) {
		return true
	}
	return false
}

// shouldSkipObject checks is object should be skipped.
func (s Sync) shouldSkipSrcObject(object *storage.Object, verbose bool) bool {
	if object.Type.IsDir() || errorpkg.IsCancelation(object.Err) {
		return true
	}

	if err := object.Err; err != nil {
		if verbose {
			s.reportError(err)
		}
		return true
	}

	// Same rules as cp: Glacier objects are skipped and reported as errors
	// unless the caller forces the transfer or asks to ignore the warnings.
	if object.StorageClass.IsGlacier() && !s.forceGlacierTransfer {
		if verbose && !s.ignoreGlacierWarnings {
			err := fmt.Errorf("object '%v' is on Glacier storage", object)
			s.reportError(err)
		}
		return true
	}
	return false
}

func (s Sync) shouldSkipDstObject(object *storage.Object, verbose bool) bool {
	if object.Type.IsDir() || errorpkg.IsCancelation(object.Err) {
		return true
	}

	if err := object.Err; err != nil {
		if verbose {
			s.reportError(err)
		}
		return true
	}

	return false
}

// isListingError reports whether err, received while listing the source or
// the destination, leaves that listing incomplete. An empty listing (no
// object or no match found) and a cancellation are not errors of that kind.
// Any other error is, whatever its code: the sync must stop, or it would plan
// from a partial listing.
func isListingError(err error) bool {
	if err == nil || errors.Is(err, storage.ErrNoObjectFound) {
		return false
	}
	return !errorpkg.IsCancelation(err)
}
