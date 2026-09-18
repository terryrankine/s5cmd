package e2e

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/igungor/gofakes3"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
	"gotest.tools/v3/icmd"
)

// sync -n s3://bucket/object file
func TestSyncFailForNonsharedFlagsFromCopyCommand(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)
	const (
		filename = "source.go"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, filename, "content")

	srcpath := fmt.Sprintf("s3://%s/%s", bucket, filename)

	cmd := s5cmd("sync", "-n", srcpath, ".")
	result := icmd.RunCmd(cmd)
	result.Assert(t, icmd.Expected{ExitCode: 1})

	// usage errors go to stderr; stdout must stay clean.
	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals("Incorrect Usage: flag provided but not defined: -n"),
		1: equals("See 's5cmd sync --help' for usage"),
	})
}

// sync folder/ folder2/
func TestSyncLocalToLocal(t *testing.T) {
	t.Parallel()

	_, s5cmd := setup(t)

	sourceWorkDir := fs.NewDir(t, "source")
	destWorkDir := fs.NewDir(t, "dest")

	srcpath := filepath.ToSlash(sourceWorkDir.Path())
	destpath := filepath.ToSlash(destWorkDir.Path())

	cmd := s5cmd("sync", srcpath, destpath)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %s %s": local->local copy operations are not permitted`, srcpath, destpath),
	})
}

// sync s3://bucket/source.go .
func TestSyncSingleS3ObjectToLocalTwice(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		filename = "source.go"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, filename, "content")

	srcpath := fmt.Sprintf("s3://%s/%s", bucket, filename)

	cmd := s5cmd("sync", srcpath, ".")
	result := icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v`, srcpath, filename),
	})

	// rerunning same command should not download object, empty result expected
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
}

// sync s3://bucket/dir/source.go .
func TestSyncSinglePrefixedS3ObjectToCurrentDirectory(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		dirname  = "dir"
		filename = "source.go"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, fmt.Sprintf("%s/%s", dirname, filename), "content")

	srcpath := fmt.Sprintf("s3://%s/%s/%s", bucket, dirname, filename)

	cmd := s5cmd("sync", srcpath, ".")
	result := icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v`, srcpath, filename),
	})

	// rerunning same command should not download object, empty result expected
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync s3://bucket/prefix/source.go dir/
func TestSyncPrefixedSingleS3ObjectToLocalDirectory(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		dirname  = "dir"
		filename = "source.go"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, fmt.Sprintf("%s/%s", dirname, filename), "content")

	srcpath := fmt.Sprintf("s3://%s/%s/%s", bucket, dirname, filename)
	dstpath := "folder"

	cmd := s5cmd("sync", srcpath, fmt.Sprintf("%v/", dstpath))
	result := icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v/%v`, srcpath, dstpath, filename),
	})

	// rerunning same command should not download object, empty result expected
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync s3://bucket/source.go dir/
func TestSyncSingleS3ObjectToLocalDirectory(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		filename = "source.go"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, filename, "content")

	srcpath := fmt.Sprintf("s3://%s/%s", bucket, filename)
	dstpath := "folder"

	cmd := s5cmd("sync", srcpath, fmt.Sprintf("%v/", dstpath))
	result := icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v/%v`, srcpath, dstpath, filename),
	})

	// rerunning same command should not download object, empty result expected
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync file s3://bucket
func TestSyncLocalFileToS3Twice(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		filename = "testfile1.txt"
		content  = "this is the content"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// the file to be uploaded is modified
	workdir := fs.NewDir(t, t.Name(), fs.WithFile(filename, content))
	defer workdir.Remove()

	dstpath := fmt.Sprintf("s3://%v", bucket)

	cmd := s5cmd("sync", filename, dstpath)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v/%v`, filename, dstpath, filename),
	})

	// rerunning same command should not upload files, empty result expected
	result = icmd.RunCmd(cmd, withWorkingDir(workdir))
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync file s3://bucket/prefix/
func TestSyncLocalFileToS3Prefix(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		filename = "testfile1.txt"
		content  = "this is the content"
		dirname  = "dir"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, t.Name(), fs.WithFile(filename, content))
	defer workdir.Remove()

	dstpath := fmt.Sprintf("s3://%v/%v", bucket, dirname)

	cmd := s5cmd("sync", filename, fmt.Sprintf("%v/", dstpath))
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v %v/%v`, filename, dstpath, filename),
	})

	// rerunning same command should not upload files, empty result expected
	result = icmd.RunCmd(cmd, withWorkingDir(workdir))
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync dir/file s3://bucket
func TestSyncLocalFileInDirectoryToS3(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		dirname  = "dir"
		filename = "testfile1.txt"
		content  = "this is the content"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, t.Name(), fs.WithDir(dirname, fs.WithFile(filename, content)))
	defer workdir.Remove()

	srcpath := fmt.Sprintf("%v/%v", dirname, filename)
	dstpath := fmt.Sprintf("s3://%v", bucket)

	cmd := s5cmd("sync", srcpath, dstpath)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/%v %v/%v`, dirname, filename, dstpath, filename),
	})

	// rerunning same command should not upload files, empty result expected
	result = icmd.RunCmd(cmd, withWorkingDir(workdir))
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync dir/file s3://bucket/prefix/
func TestSyncLocalFileInDirectoryToS3Prefix(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	const (
		dirname  = "dir"
		filename = "testfile1.txt"
		content  = "this is the content"
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, t.Name(), fs.WithDir(dirname, fs.WithFile(filename, content)))
	defer workdir.Remove()

	srcpath := fmt.Sprintf("%v/%v", dirname, filename)
	dstpath := fmt.Sprintf("s3://%v/%v", bucket, dirname)

	cmd := s5cmd("sync", srcpath, fmt.Sprintf("%v/", dstpath))
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/%v %v/%v`, dirname, filename, dstpath, filename),
	})

	// rerunning same command should not upload files, empty result expected
	result = icmd.RunCmd(cmd, withWorkingDir(workdir))
	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync --raw object* s3://bucket/prefix/
func TestCopyLocalFilestoS3WithRawFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	files := []fs.PathOp{
		fs.WithFile("file*.txt", "content"),
		fs.WithFile("file*1.txt", "content"),
		fs.WithFile("file*file.txt", "content"),
		fs.WithFile("file*2.txt", "content"),
	}

	expectedFiles := []string{"file*.txt"}
	nonExpectedFiles := []string{"file*1.txt", "file*file.txt", "file*2.txt"}

	// the file to be uploaded is modified
	workdir := fs.NewDir(t, t.Name(), files...)
	defer workdir.Remove()

	dstpath := fmt.Sprintf("s3://%v/prefix/", bucket)

	cmd := s5cmd("sync", "--raw", "file*.txt", dstpath)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp file*.txt %vfile*.txt`, dstpath),
	})

	result = icmd.RunCmd(cmd, withWorkingDir(workdir))

	// second run should not upload files, empty result expected
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})

	for _, obj := range expectedFiles {
		err := ensureS3Object(s3client, bucket, "prefix/"+obj, "content")
		if err != nil {
			t.Fatalf("%s is not exist in s3\n", obj)
		}
	}

	for _, obj := range nonExpectedFiles {
		err := ensureS3Object(s3client, bucket, "prefix/"+obj, "content")
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync folder/ s3://bucket
func TestSyncLocalFolderToS3EmptyBucket(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile.txt", "S: this is a test file"),
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another txt file"),
		),
		fs.WithDir("b",
			fs.WithFile("filename-with-hypen.gz", "S: file has hyphen in its name"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %va/another_test_file.txt %va/another_test_file.txt`, src, dst),
		1: equals(`cp %vb/filename-with-hypen.gz %vb/filename-with-hypen.gz`, src, dst),
		2: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		3: equals(`cp %vtestfile.txt %vtestfile.txt`, src, dst),
	}, sortInput(true))

	// there should be no error, since "no object found" error for destination is ignored
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"testfile.txt":             "S: this is a test file",
		"readme.md":                "S: this is a readme file",
		"b/filename-with-hypen.gz": "S: file has hyphen in its name",
		"a/another_test_file.txt":  "S: yet another txt file",
	}

	// assert s3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// cp parent/*/name.txt s3://bucket/newfolder
func TestSyncMultipleFilesWithWildcardedDirectoryToS3Bucket(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithDir("parent", fs.WithDir(
			"child1",
			fs.WithFile("name.txt", "A file in parent/child1/"),
		),
			fs.WithDir(
				"child2",
				fs.WithFile("name.txt", "A file in parent/child2/"),
			),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	dstpath := fmt.Sprintf("s3://%v/newfolder/", bucket)
	srcpath := workdir.Path()
	srcpath = filepath.ToSlash(srcpath)
	defer workdir.Remove()

	cmd := s5cmd("sync", srcpath+"/parent/*/name.txt", dstpath)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)
	rs := result.Stdout()
	assertLines(t, rs, map[int]compareFunc{
		0: equals(`cp %v/parent/child1/name.txt %vchild1/name.txt`, srcpath, dstpath),
		1: equals(`cp %v/parent/child2/name.txt %vchild2/name.txt`, srcpath, dstpath),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
	expectedS3Content := map[string]string{
		"newfolder/child1/name.txt": "A file in parent/child1/",
		"newfolder/child2/name.txt": "A file in parent/child2/",
	}

	// assert s3
	for filename, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, filename, content))
	}
}

// sync  s3://bucket/* folder/
func TestSyncS3BucketToEmptyFolder(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
		"abc/def/test.py":         "S: file in nested folders",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%v/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/a/another_test_file.txt %va/another_test_file.txt`, bucketPath, dst),
		1: equals(`cp %v/abc/def/test.py %vabc/def/test.py`, bucketPath, dst),
		2: equals(`cp %v/readme.md %vreadme.md`, bucketPath, dst),
		3: equals(`cp %v/testfile.txt %vtestfile.txt`, bucketPath, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("testfile.txt", "S: this is a test file"),
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another txt file"),
		),
		fs.WithDir("abc",
			fs.WithDir("def",
				fs.WithFile("test.py", "S: file in nested folders"),
			),
		),
	}

	// assert local filesystem
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync  s3://bucket/* s3://destbucket/prefix/
func TestSyncS3BucketToEmptyS3Bucket(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	const (
		prefix = "prefix"
	)
	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	s3Content := map[string]string{
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
		"abc/def/test.py":         "S: file in nested folders",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%v/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/%v/", dstbucket, prefix)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/a/another_test_file.txt %va/another_test_file.txt`, bucketPath, dst),
		1: equals(`cp %v/abc/def/test.py %vabc/def/test.py`, bucketPath, dst),
		2: equals(`cp %v/readme.md %vreadme.md`, bucketPath, dst),
		3: equals(`cp %v/testfile.txt %vtestfile.txt`, bucketPath, dst),
	}, sortInput(true))

	// assert  s3 objects in source bucket.
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects in dest bucket
	for key, content := range s3Content {
		key = fmt.Sprintf("%s/%s", prefix, key) // add the prefix
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}
}

// sync folder/ s3://bucket (source older, same objects)
func TestSyncLocalFolderToS3BucketSameObjectsSourceOlder(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// local files are 1 minute older than the remotes
	timestamp := fs.WithTimestamps(
		now.Add(-time.Minute), // access time
		now.Add(-time.Minute), // mod time
	)

	folderLayout := []fs.PathOp{
		fs.WithFile("main.py", "S: this is a python file", timestamp),
		fs.WithFile("testfile.txt", "S: this is a test file", timestamp),
		fs.WithFile("readme.md", "S: this is a readme file", timestamp),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another txt file", timestamp),
			timestamp,
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"main.py":                 "D: this is a python file",
		"testfile.txt":            "D: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %va/another_test_file.txt %va/another_test_file.txt": object is newer or same age and object size matches`, src, dst),
		1: equals(`DEBUG "sync %vmain.py %vmain.py": object is newer or same age and object size matches`, src, dst),
		2: equals(`DEBUG "sync %vreadme.md %vreadme.md": object is newer or same age and object size matches`, src, dst),
		3: equals(`DEBUG "sync %vtestfile.txt %vtestfile.txt": object is newer or same age and object size matches`, src, dst),
	}, sortInput(true))

	// expected folder structure
	expectedFiles := []fs.PathOp{
		fs.WithFile("main.py", "S: this is a python file"),
		fs.WithFile("testfile.txt", "S: this is a test file"),
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another txt file"),
		),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync folder/ s3://bucket (source newer)
func TestSyncLocalFolderToS3BucketSourceNewer(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// local files are 1 minute newer than the remotes
	timestamp := fs.WithTimestamps(
		now.Add(time.Minute),
		now.Add(time.Minute),
	)

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile.txt", "S: this is an updated test file", timestamp),
		fs.WithFile("readme.md", "S: this is an updated readme file", timestamp),
		fs.WithDir("dir",
			fs.WithFile("main.py", "S: updated python file", timestamp),
			timestamp,
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"testfile.txt": "D: this is a test file ",
		"readme.md":    "D: this is a readme file",
		"dir/main.py":  "D: python file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vdir/main.py %vdir/main.py`, src, dst),
		1: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		2: equals(`cp %vtestfile.txt %vtestfile.txt`, src, dst),
	}, sortInput(true))

	// expected folder structure, without the timestamps.
	expectedFiles := []fs.PathOp{
		fs.WithFile("testfile.txt", "S: this is an updated test file"),
		fs.WithFile("readme.md", "S: this is an updated readme file"),
		fs.WithDir("dir",
			fs.WithFile("main.py", "S: updated python file"),
		),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// same as local source
	expectedS3Content := map[string]string{
		"testfile.txt": "S: this is an updated test file",
		"readme.md":    "S: this is an updated readme file",
		"dir/main.py":  "S: updated python file",
	}

	// assert s3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync folder/ s3://bucket with s5cmd running in a non-UTC zone.
//
// Same instant-versus-wall-clock check as
// TestCopyLocalToS3IfSourceNewerComparesInstantsAcrossTimezones, for the sync
// path: it lists both sides and round-trips every mtime through the external
// sort before comparing (upstream #845).
func TestSyncLocalFolderToS3BucketComparesInstantsAcrossTimezones(t *testing.T) {
	t.Parallel()

	zones := []string{"Australia/Perth", "America/New_York", "Europe/Copenhagen"}
	for _, zone := range zones {
		t.Run(zone, func(t *testing.T) {
			t.Parallel()

			now := time.Now()
			timeSource := newFixedTimeSource(now)
			s3client, s5cmd := setup(t, withTimeSource(timeSource))

			bucket := s3BucketFromTestName(t)
			createBucket(t, s3client, bucket)

			// 30 minutes either side of the S3 stamp: less than any zone's
			// offset. Sizes match so only the mtime decides.
			older := fs.WithTimestamps(now.Add(-30*time.Minute), now.Add(-30*time.Minute))
			newer := fs.WithTimestamps(now.Add(30*time.Minute), now.Add(30*time.Minute))

			workdir := fs.NewDir(t, "somedir",
				fs.WithFile("older.txt", "S: older", older),
				fs.WithFile("newer.txt", "S: newer", newer),
			)
			defer workdir.Remove()

			putFile(t, s3client, bucket, "older.txt", "D: older")
			putFile(t, s3client, bucket, "newer.txt", "D: newer")

			src := fmt.Sprintf("%v/", workdir.Path())
			src = filepath.ToSlash(src)
			dst := fmt.Sprintf("s3://%v/", bucket)

			cmd := s5cmd("--log", "debug", "sync", src, dst)
			result := icmd.RunCmd(cmd, withEnv("TZ", zone))

			result.Assert(t, icmd.Success)

			assertLines(t, result.Stdout(), map[int]compareFunc{
				0: equals(`DEBUG "sync %volder.txt %volder.txt": object is newer or same age and object size matches`, src, dst),
				1: equals(`cp %vnewer.txt %vnewer.txt`, src, dst),
			}, sortInput(true))

			assert.Assert(t, ensureS3Object(s3client, bucket, "older.txt", "D: older"))
			assert.Assert(t, ensureS3Object(s3client, bucket, "newer.txt", "S: newer"))
		})
	}
}

// sync s3://bucket/* folder/ (same objects, source older, destination newer)
func TestSyncS3BucketToLocalFolderSameObjectsSourceOlder(t *testing.T) {
	t.Parallel()

	newer := time.Now().Add(time.Minute)

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// local files are 1 minute newer than the remote ones
	timestamp := fs.WithTimestamps(
		newer,
		newer,
	)

	folderLayout := []fs.PathOp{
		fs.WithFile("main.py", "D: this is a python file", timestamp),
		fs.WithFile("testfile.txt", "D: this is a test file", timestamp),
		fs.WithFile("readme.md", "D: this is a readme file", timestamp),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "D: yet another txt file", timestamp),
			timestamp,
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"main.py":                 "S: this is a python file",
		"testfile.txt":            "S: this is a test file",   // content different from local
		"readme.md":               "S: this is a readme file", // content different from local
		"a/another_test_file.txt": "S: yet another txt file",  // content different from local
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %v/a/another_test_file.txt %va/another_test_file.txt": object is newer or same age and object size matches`, bucketPath, dst),
		1: equals(`DEBUG "sync %v/main.py %vmain.py": object is newer or same age and object size matches`, bucketPath, dst),
		2: equals(`DEBUG "sync %v/readme.md %vreadme.md": object is newer or same age and object size matches`, bucketPath, dst),
		3: equals(`DEBUG "sync %v/testfile.txt %vtestfile.txt": object is newer or same age and object size matches`, bucketPath, dst),
	}, sortInput(true))

	// expected folder structure without the timestamp.
	expectedFiles := []fs.PathOp{
		fs.WithFile("main.py", "D: this is a python file"),
		fs.WithFile("testfile.txt", "D: this is a test file"),
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "D: yet another txt file"),
		),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync s3://bucket/* folder/ (same objects, source newer)
func TestSyncS3BucketToLocalFolderSameObjectsSourceNewer(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// local files are 1 minute older, that makes remote files newer than them.
	timestamp := fs.WithTimestamps(
		now.Add(-time.Minute),
		now.Add(-time.Minute),
	)

	folderLayout := []fs.PathOp{
		fs.WithFile("main.py", "D: this is a python file", timestamp),
		fs.WithFile("testfile.txt", "D: this is a test file", timestamp),
		fs.WithFile("readme.md", "D: this is a readme file", timestamp),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "D: yet another txt file", timestamp),
			timestamp,
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"main.py":                 "S: this is a python file",
		"testfile.txt":            "S: this is an updated test file",
		"readme.md":               "S: this is an updated readme file",
		"a/another_test_file.txt": "S: yet another updated txt file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		1: equals(`cp %v/main.py %vmain.py`, bucketPath, dst),
		0: equals(`cp %v/a/another_test_file.txt %va/another_test_file.txt`, bucketPath, dst),
		2: equals(`cp %v/readme.md %vreadme.md`, bucketPath, dst),
		3: equals(`cp %v/testfile.txt %vtestfile.txt`, bucketPath, dst),
	}, sortInput(true))

	// expected folder structure without the timestamp.
	expectedFiles := []fs.PathOp{
		fs.WithFile("main.py", "S: this is a python file"),
		fs.WithFile("testfile.txt", "S: this is an updated test file"),
		fs.WithFile("readme.md", "S: this is an updated readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another updated txt file"),
		),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync s3://bucket/* s3://destbucket/ (source newer, same objects, different content, same sizes)
func TestSyncS3BucketToS3BucketSameSizesSourceNewer(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	sourceS3Content := map[string]string{
		"main.py":                 "S: this is a python file",
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
	}

	// the file sizes are same, with different contents.
	destS3Content := map[string]string{
		"main.py":                 "D: this is a python file",
		"testfile.txt":            "D: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	// make destination files 1 minute older
	timeSource.Advance(-time.Minute)
	for filename, content := range destS3Content {
		putFile(t, s3client, dstbucket, filename, content)
	}

	timeSource.Advance(time.Minute)
	for filename, content := range sourceS3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/a/another_test_file.txt %va/another_test_file.txt`, bucketPath, dst),
		1: equals(`cp %v/main.py %vmain.py`, bucketPath, dst),
		2: equals(`cp %v/readme.md %vreadme.md`, bucketPath, dst),
		3: equals(`cp %v/testfile.txt %vtestfile.txt`, bucketPath, dst),
	}, sortInput(true))

	// assert s3 objects in source
	for key, content := range sourceS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects in destination (should be same as source)
	for key, content := range sourceS3Content {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}
}

// sync s3://bucket/* s3://destbucket/ (source older, same objects, different content, same sizes)
func TestSyncS3BucketToS3BucketSameSizesSourceOlder(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	sourceS3Content := map[string]string{
		"main.py":                 "S: this is a python file",
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
	}

	// the file sizes are same, content different.
	destS3Content := map[string]string{
		"main.py":                 "D: this is a python file",
		"testfile.txt":            "D: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	// make source files 1 minute older
	timeSource.Advance(-time.Minute)
	for filename, content := range sourceS3Content {
		putFile(t, s3client, bucket, filename, content)
	}
	timeSource.Advance(time.Minute)

	for filename, content := range destS3Content {
		putFile(t, s3client, dstbucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %v/a/another_test_file.txt %va/another_test_file.txt": object is newer or same age and object size matches`, bucketPath, dst),
		1: equals(`DEBUG "sync %v/main.py %vmain.py": object is newer or same age and object size matches`, bucketPath, dst),
		2: equals(`DEBUG "sync %v/readme.md %vreadme.md": object is newer or same age and object size matches`, bucketPath, dst),
		3: equals(`DEBUG "sync %v/testfile.txt %vtestfile.txt": object is newer or same age and object size matches`, bucketPath, dst),
	}, sortInput(true))

	// assert s3 objects in source
	for key, content := range sourceS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects in destination (should not change).
	for key, content := range destS3Content {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}
}

// sync --size-only s3://bucket/* folder/
func TestSyncS3BucketToLocalFolderSameObjectsSizeOnly(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("test.py", "D: this is a python file"),
		fs.WithFile("testfile.txt", "D: this is a test file"),
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "D: yet another txt file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"test.py":                 "S: this is an updated python file", // content different from local, different size
		"testfile.txt":            "S: this is a test file",            // content different from local, same size
		"readme.md":               "S: this is a readme file",          // content different from local, same size
		"a/another_test_file.txt": "S: yet another txt file",           // content different from local, same size
		"abc/def/main.py":         "S: python file",                    // local does not have it.
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", "--size-only", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %v/a/another_test_file.txt %va/another_test_file.txt": object size matches`, bucketPath, dst),
		1: equals(`DEBUG "sync %v/readme.md %vreadme.md": object size matches`, bucketPath, dst),
		2: equals(`DEBUG "sync %v/testfile.txt %vtestfile.txt": object size matches`, bucketPath, dst),
		3: equals(`cp %v/abc/def/main.py %vabc/def/main.py`, bucketPath, dst),
		4: equals(`cp %v/test.py %vtest.py`, bucketPath, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("test.py", "S: this is an updated python file"),
		fs.WithFile("testfile.txt", "D: this is a test file"),
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "D: yet another txt file"),
		),
		fs.WithDir("abc",
			fs.WithDir("def",
				fs.WithFile("main.py", "S: python file"),
			),
		),
	}

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync s3://bucket/* s3://destbucket/ (same objects, same size, same content, different or same storage class)
func TestSyncS3BucketToS3BucketIsStorageClassChanging(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	srcbucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, srcbucket)
	createBucket(t, s3client, dstbucket)

	storageClassesAndFile := []struct {
		srcStorageClass string
		dstStorageClass string
		filename        string
		content         string
	}{
		{"STANDARD", "STANDARD", "testfile1.txt", "this is a test file"},
		{"STANDARD", "GLACIER", "testfile2.txt", "this is a test file"},
		{"GLACIER", "STANDARD", "testfile3.txt", "this is a test file"},
		{"GLACIER", "GLACIER", "testfile4.txt", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {

		putObject := s3.PutObjectInput{
			Bucket:       &srcbucket,
			Key:          &sc.filename,
			Body:         strings.NewReader(sc.content),
			StorageClass: &sc.srcStorageClass,
		}

		_, err := s3client.PutObject(&putObject)
		if err != nil {
			t.Fatalf("failed to put object in %v: %v", sc.srcStorageClass, err)
		}

		putObject = s3.PutObjectInput{
			Bucket:       &dstbucket,
			Key:          &sc.filename,
			Body:         strings.NewReader(sc.content),
			StorageClass: aws.String(sc.dstStorageClass),
		}

		_, err = s3client.PutObject(&putObject)
		if err != nil {
			t.Fatalf("failed to put object in %v: %v", sc.dstStorageClass, err)
		}

	}

	bucketPath := fmt.Sprintf("s3://%v", srcbucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	// there will be no stdout, since there are no changes; the Glacier
	// objects are skipped and reported, so the exit code is non-zero
	result.Assert(t, icmd.Expected{ExitCode: 1})
	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v %v": object '%v/testfile3.txt' is on Glacier storage`, src, dst, bucketPath),
		1: equals(`ERROR "sync %v %v": object '%v/testfile4.txt' is on Glacier storage`, src, dst, bucketPath),
	}, sortInput(true))

	// assert s3 objects in source
	for _, sc := range storageClassesAndFile {
		assert.Assert(t, ensureS3Object(s3client, srcbucket, sc.filename, sc.content, ensureStorageClass(sc.srcStorageClass)))
	}

	// assert s3 objects in destination
	for _, sc := range storageClassesAndFile {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, sc.filename, sc.content, ensureStorageClass(sc.dstStorageClass)))
	}
}

// sync dir s3://destbucket/ (same objects, same size, same content, different or same storage class)
func TestSyncLocalFolderToS3BucketIsStorageClassChanging(t *testing.T) {
	t.Parallel()
	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	timestamp := fs.WithTimestamps(
		now.Add(-time.Minute),
		now.Add(-time.Minute),
	)

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is a test file", timestamp),
		fs.WithFile("testfile2.txt", "this is a test file", timestamp),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	storageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is a test file"},
		{"GLACIER", "testfile2.txt", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {

		putObject := s3.PutObjectInput{
			Bucket:       &bucket,
			Key:          &sc.filename,
			Body:         strings.NewReader(sc.content),
			StorageClass: aws.String(sc.storageClass),
		}

		_, err := s3client.PutObject(&putObject)
		if err != nil {
			t.Fatalf("failed to put object in %v: %v", sc.storageClass, err)
		}
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	// there will be no stdout
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})

	expectedFiles := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is a test file"),
		fs.WithFile("testfile2.txt", "this is a test file"),
	}

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3 objects in destination
	for _, sc := range storageClassesAndFile {
		assert.Assert(t, ensureS3Object(s3client, bucket, sc.filename, sc.content, ensureStorageClass(sc.storageClass)))
	}
}

// sync s3://srcbucket/ dir (same objects, same size, same content, different or same storage class)
func TestSyncS3BucketToLocalFolderIsStorageClassChanging(t *testing.T) {
	t.Parallel()
	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	// local files are 1 minute newer than the remotes
	timestamp := fs.WithTimestamps(
		now.Add(time.Minute),
		now.Add(time.Minute),
	)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	storageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is a test file"},
		{"GLACIER", "testfile2.txt", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {

		putObject := s3.PutObjectInput{
			Bucket:       &bucket,
			Key:          &sc.filename,
			Body:         strings.NewReader(sc.content),
			StorageClass: aws.String(sc.storageClass),
		}

		_, err := s3client.PutObject(&putObject)
		if err != nil {
			t.Fatalf("failed to put object in %v: %v", sc.storageClass, err)
		}
	}

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is a test file", timestamp),
		fs.WithFile("testfile2.txt", "this is a test file", timestamp),
	}

	// put objects in local folder

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	// there will be no stdout; the Glacier object is skipped and reported,
	// so the exit code is non-zero
	result.Assert(t, icmd.Expected{ExitCode: 1})
	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v %v": object '%v/testfile2.txt' is on Glacier storage`, src, dst, bucketPath),
	})

	expectedFiles := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is a test file"),
		fs.WithFile("testfile2.txt", "this is a test file"),
	}

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3 objects in destination
	for _, sc := range storageClassesAndFile {
		assert.Assert(t, ensureS3Object(s3client, bucket, sc.filename, sc.content, ensureStorageClass(sc.storageClass)))
	}
}

// sync s3://srcbucket/* s3://dstbucket/ (same objects, different size, different content, different or same storage class)
func TestSyncS3BucketToS3BucketIsStorageClassChangingWithDifferentSizeAndContent(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	srcbucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, srcbucket)
	createBucket(t, s3client, dstbucket)

	storageClassesAndFile := []struct {
		srcStorageClass string
		dstStorageClass string
		filename        string
		srcContent      string
		dstContent      string
	}{
		{"STANDARD", "STANDARD", "testfile1.txt", "this is an updated test file", "this is a test file"},
		{"STANDARD", "GLACIER", "testfile2.txt", "this is an updated test file", "this is a test file"},
		{"GLACIER", "STANDARD", "testfile3.txt", "this is an updated test file", "this is a test file"},
		{"GLACIER", "GLACIER", "testfile4.txt", "this is an updated test file", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {

		putFile(t, s3client, srcbucket, sc.filename, sc.srcContent, putStorageClass(sc.srcStorageClass))

		putObject := s3.PutObjectInput{
			Bucket:       &dstbucket,
			Key:          &sc.filename,
			Body:         strings.NewReader(sc.dstContent),
			StorageClass: aws.String(sc.dstStorageClass),
		}

		_, err := s3client.PutObject(&putObject)
		if err != nil {
			t.Fatalf("failed to put object in %v: %v", sc.dstStorageClass, err)
		}
	}

	bucketPath := fmt.Sprintf("s3://%v", srcbucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	cmd := s5cmd("sync", src, dst)

	result := icmd.RunCmd(cmd)
	// the Glacier objects are skipped and reported, so the exit code is non-zero
	result.Assert(t, icmd.Expected{ExitCode: 1})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v %v": object '%v/testfile3.txt' is on Glacier storage`, src, dst, bucketPath),
		1: equals(`ERROR "sync %v %v": object '%v/testfile4.txt' is on Glacier storage`, src, dst, bucketPath),
	}, sortInput(true))

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/testfile1.txt %vtestfile1.txt`, bucketPath, dst),
		1: equals(`cp %v/testfile2.txt %vtestfile2.txt`, bucketPath, dst),
	}, sortInput(true))

	// assert s3 objects in source
	for _, sc := range storageClassesAndFile {
		assert.Assert(t, ensureS3Object(s3client, srcbucket, sc.filename, sc.srcContent, ensureStorageClass(sc.srcStorageClass)))
	}

	// assert s3 objects in destination (file1 and file2 should be updated and file3 and file4 should be same as before)
	assert.Assert(t, ensureS3Object(s3client, dstbucket, "testfile1.txt", "this is an updated test file"), ensureStorageClass("STANDARD"))
	assert.Assert(t, ensureS3Object(s3client, dstbucket, "testfile2.txt", "this is an updated test file", ensureStorageClass("STANDARD")))
	assert.Assert(t, ensureS3Object(s3client, dstbucket, "testfile3.txt", "this is a test file", ensureStorageClass("STANDARD")))
	assert.Assert(t, ensureS3Object(s3client, dstbucket, "testfile4.txt", "this is a test file", ensureStorageClass("GLACIER")))
}

// sync dir s3://destbucket/ (same objects, different size, different content, different or same storage class)
func TestSyncLocalFolderToS3BucketIsStorageClassChangingWithDifferentSizeAndContent(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is an updated test file"),
		fs.WithFile("testfile2.txt", "this is an updated test file"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	storageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is a test file"},
		{"GLACIER", "testfile2.txt", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {
		putFile(t, s3client, bucket, sc.filename, sc.content, putStorageClass(sc.storageClass))
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vtestfile1.txt %vtestfile1.txt`, src, dst),
		1: equals(`cp %vtestfile2.txt %vtestfile2.txt`, src, dst),
	}, sortInput(true))

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3 objects in destination
	assert.Assert(t, ensureS3Object(s3client, bucket, "testfile1.txt", "this is an updated test file"), ensureStorageClass("STANDARD"))
	assert.Assert(t, ensureS3Object(s3client, bucket, "testfile2.txt", "this is an updated test file"), ensureStorageClass("STANDARD"))
}

// sync s3://destbucket/ dir (same objects, different size, different content, different or same storage class)
func TestSyncS3BucketToLocalFolderIsStorageClassChangingWithDifferentSizeAndContent(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	storageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is an updated test file"},
		{"GLACIER", "testfile2.txt", "this is an updated test file"},
	}

	for _, sc := range storageClassesAndFile {
		putFile(t, s3client, bucket, sc.filename, sc.content, putStorageClass(sc.storageClass))
	}

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is a test file"),
		fs.WithFile("testfile2.txt", "this is a test file"),
	}

	// put objects in local folder
	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", src, dst)

	result := icmd.RunCmd(cmd)

	// the Glacier object is skipped and reported, so the exit code is non-zero
	result.Assert(t, icmd.Expected{ExitCode: 1})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v %v": object '%v/testfile2.txt' is on Glacier storage`, src, dst, bucketPath),
	})

	// testfile1.txt should be updated and testfile2.txt shouldn't be updated because it is in glacier.
	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/testfile1.txt %vtestfile1.txt`, bucketPath, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("testfile1.txt", "this is an updated test file"),
		fs.WithFile("testfile2.txt", "this is a test file"),
	}

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --ignore-glacier-warnings s3://bucket/* dir/  (source has a Glacier object)
func TestSyncS3BucketToLocalFolderWithIgnoreGlacierWarnings(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	putFile(t, s3client, bucket, "testfile1.txt", "this is a test file", putStorageClass("STANDARD"))
	putFile(t, s3client, bucket, "testfile2.txt", "this is a test file", putStorageClass("GLACIER"))

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--ignore-glacier-warnings", src, dst)
	result := icmd.RunCmd(cmd)

	// the Glacier object is skipped silently
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/testfile1.txt %vtestfile1.txt`, bucketPath, dst),
	})
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t, fs.WithFile("testfile1.txt", "this is a test file"))
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --force-glacier-transfer s3://bucket/* dir/  (source has a Glacier object)
func TestSyncS3BucketToLocalFolderWithForceGlacierTransfer(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	putFile(t, s3client, bucket, "testfile1.txt", "this is a test file", putStorageClass("STANDARD"))
	putFile(t, s3client, bucket, "testfile2.txt", "this is a test file", putStorageClass("GLACIER"))

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--force-glacier-transfer", src, dst)
	result := icmd.RunCmd(cmd)

	// the Glacier object is synced like any other
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/testfile1.txt %vtestfile1.txt`, bucketPath, dst),
		1: equals(`cp %v/testfile2.txt %vtestfile2.txt`, bucketPath, dst),
	}, sortInput(true))
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithFile("testfile1.txt", "this is a test file"),
		fs.WithFile("testfile2.txt", "this is a test file"),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --delete s3://bucket/* s3://destbucket/ (storage class test)
func TestSyncS3BucketToS3BucketWithDeleteStorageClass(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	srcbucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, srcbucket)
	createBucket(t, s3client, dstbucket)

	dstStorageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is a test file"},
		{"GLACIER", "testfile2.txt", "this is a test file"},
	}

	for _, sc := range dstStorageClassesAndFile {
		putFile(t, s3client, dstbucket, sc.filename, sc.content, putStorageClass(sc.storageClass))
	}

	bucketPath := fmt.Sprintf("s3://%v", srcbucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	cmd := s5cmd("sync", "--delete", src, dst)

	result := icmd.RunCmd(cmd)

	// the source is empty: the deletes still run, but the "no object found"
	// error is reported and the exit code is non-zero
	result.Assert(t, icmd.Expected{ExitCode: 1})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync --delete=true %v %v": no object found`, src, dst),
	})

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`rm %vtestfile1.txt`, dst),
		1: equals(`rm %vtestfile2.txt`, dst),
	}, sortInput(true))

	// assert s3 objects in destination
	for _, sc := range dstStorageClassesAndFile {
		err := ensureS3Object(s3client, dstbucket, sc.filename, sc.content, ensureStorageClass(sc.storageClass))
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete dir s3://destbucket/ (storage class test)
func TestSyncLocalFolderToS3BucketWithDeleteStorageClass(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	storageClassesAndFile := []struct {
		storageClass string
		filename     string
		content      string
	}{
		{"STANDARD", "testfile1.txt", "this is a test file"},
		{"GLACIER", "testfile2.txt", "this is a test file"},
	}

	for _, sc := range storageClassesAndFile {
		putFile(t, s3client, bucket, sc.filename, sc.content, putStorageClass(sc.storageClass))
	}

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", src, dst)

	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`rm %vtestfile1.txt`, dst),
		1: equals(`rm %vtestfile2.txt`, dst),
	}, sortInput(true))

	// assert s3 objects in destination
	for _, sc := range storageClassesAndFile {
		err := ensureS3Object(s3client, bucket, sc.filename, sc.content, ensureStorageClass(sc.storageClass))
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --size-only folder/ s3://bucket/
func TestSyncLocalFolderToS3BucketSameObjectsSizeOnly(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("test.py", "S: this is a python file"),    // remote has it, different content, size same
		fs.WithFile("testfile.txt", "S: this is a test file"), // remote has it, but with different contents/size.
		fs.WithFile("readme.md", "S: this is a readme file"),  // remote has it, same object.
		fs.WithDir("a",
			fs.WithFile("another_test_file.txt", "S: yet another txt file"), // remote has it, different content, same size.
		),
		fs.WithDir("abc",
			fs.WithDir("def",
				fs.WithFile("main.py", "S: python file"), // remote does not have it
			),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"test.py":                 "D: this is a python file",
		"testfile.txt":            "D: this is an updated test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%s/", bucket)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", "--size-only", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %va/another_test_file.txt %va/another_test_file.txt": object size matches`, src, dst),
		1: equals(`DEBUG "sync %vreadme.md %vreadme.md": object size matches`, src, dst),
		2: equals(`DEBUG "sync %vtest.py %vtest.py": object size matches`, src, dst),
		3: equals(`cp %vabc/def/main.py %vabc/def/main.py`, src, dst),
		4: equals(`cp %vtestfile.txt %vtestfile.txt`, src, dst),
	}, sortInput(true))

	// expected folder structure without the timestamp.
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"test.py":                 "D: this is a python file",
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
		"abc/def/main.py":         "S: python file",
	}

	// assert s3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync --size-only s3://bucket/* s3://destbucket/
func TestSyncS3BucketToS3BucketSizeOnly(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")
	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	sourceS3Content := map[string]string{
		"main.py":                 "S: this is an updated python file",
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readve file",
		"a/another_test_file.txt": "S: yet another txt file",
	}

	destS3Content := map[string]string{
		"main.py":                 "D: this is a python file", // file size is smaller than source.
		"testfile.txt":            "D: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	// make source files older in bucket.
	// timestamps should be ignored with --size-only flag
	timeSource.Advance(-time.Minute)
	for filename, content := range destS3Content {
		putFile(t, s3client, dstbucket, filename, content)
	}
	timeSource.Advance(time.Minute)

	for filename, content := range sourceS3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%s/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	// log debug
	cmd := s5cmd("--log", "debug", "sync", "--size-only", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`DEBUG "sync %v/a/another_test_file.txt %va/another_test_file.txt": object size matches`, bucketPath, dst),
		1: equals(`DEBUG "sync %v/readme.md %vreadme.md": object size matches`, bucketPath, dst),
		2: equals(`DEBUG "sync %v/testfile.txt %vtestfile.txt": object size matches`, bucketPath, dst),
		3: equals(`cp %v/main.py %vmain.py`, bucketPath, dst),
	}, sortInput(true))

	// assert s3 objects in source
	for key, content := range sourceS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	expectedDestS3Content := map[string]string{
		"main.py":                 "S: this is an updated python file", // same as source.
		"testfile.txt":            "D: this is a test file",
		"readme.md":               "D: this is a readme file",
		"a/another_test_file.txt": "D: yet another txt file",
	}

	// assert s3 objects in destination
	for key, content := range expectedDestS3Content {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}
}

// sync --delete s3://bucket/* .
func TestSyncS3BucketToLocalWithDelete(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"contributing.md": "S: this is a readme file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile.txt", "D: this is a test file"),
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir("dir",
			fs.WithFile("main.py", "D: python file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--delete", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vcontributing.md %vcontributing.md`, src, dst),
		1: equals(`rm %vdir/main.py`, dst),
		2: equals(`rm %vreadme.md`, dst),
		3: equals(`rm %vtestfile.txt`, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithDir("dir"),
		fs.WithFile("contributing.md", "S: this is a readme file"),
	}

	// assert local filesystem
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync --delete s3://bucket/prefix/* dir/  (directory markers "prefix/",
// "prefix/sub/", "prefix/empty/")
//
// Markers are skipped like in cp: not downloaded, not an error. With --delete
// they take no part in the plan either: a local file that is not in the
// source is removed, and nothing else.
// See: https://github.com/peak/s5cmd/issues/517
func TestSyncS3ToLocalWithDirectoryMarkers(t *testing.T) {
	t.Parallel()

	var backend gofakes3.Backend
	s3client, s5cmd := setup(t, withBackend(&backend))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	putDirectoryMarker(t, s3client, backend, bucket, "p/")
	putDirectoryMarker(t, s3client, backend, bucket, "p/sub/")
	putDirectoryMarker(t, s3client, backend, bucket, "p/empty/")
	putFile(t, s3client, bucket, "p/a.txt", "A")
	putFile(t, s3client, bucket, "p/sub/b.txt", "BB")

	workdir := fs.NewDir(t, "somedir",
		fs.WithFile("stale.txt", "D: not in the source"),
	)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/p/", bucket)
	dst := filepath.ToSlash(workdir.Path()) + "/"

	cmd := s5cmd("sync", "--delete", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stderr(), map[int]compareFunc{})

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %va.txt %va.txt`, src, dst),
		1: equals(`cp %vsub/b.txt %vsub/b.txt`, src, dst),
		2: equals(`rm %vstale.txt`, dst),
	}, sortInput(true))

	expected := fs.Expected(t,
		fs.WithFile("a.txt", "A"),
		fs.WithDir("sub",
			fs.WithFile("b.txt", "BB"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync [--delete] dir/ s3://bucket/prefix/  (destination has directory
// markers "prefix/", "prefix/sub/", "prefix/old/")
//
// sync manages files, not folders: a marker in the destination is never
// deleted, with or without --delete, just as an empty local directory is
// never removed by "sync --delete s3://bucket/* dir/". "rm --raw" deletes a
// marker on purpose.
// See: https://github.com/peak/s5cmd/issues/517
func TestSyncLocalToS3WithDirectoryMarkersInDestination(t *testing.T) {
	t.Parallel()

	var backend gofakes3.Backend
	s3client, s5cmd := setup(t, withBackend(&backend))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	markers := []string{"p/", "p/sub/", "p/old/"}
	for _, marker := range markers {
		putDirectoryMarker(t, s3client, backend, bucket, marker)
	}
	putFile(t, s3client, bucket, "p/stale.txt", "D: not in the source")

	workdir := fs.NewDir(t, "somedir",
		fs.WithFile("a.txt", "A"),
		fs.WithDir("sub",
			fs.WithFile("b.txt", "BB"),
		),
	)
	defer workdir.Remove()

	src := filepath.ToSlash(workdir.Path()) + "/"
	dst := fmt.Sprintf("s3://%v/p/", bucket)

	// without --delete: files are uploaded, nothing is removed.
	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stderr(), map[int]compareFunc{})

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %va.txt %va.txt`, src, dst),
		1: equals(`cp %vsub/b.txt %vsub/b.txt`, src, dst),
	}, sortInput(true))

	assert.Assert(t, ensureS3Object(s3client, bucket, "p/stale.txt", "D: not in the source"))
	for _, marker := range markers {
		assert.Assert(t, s3ObjectExists(t, s3client, backend, bucket, marker), marker)
	}

	// with --delete: only the stale file goes; the markers stay.
	cmd = s5cmd("sync", "--delete", src, dst)
	result = icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stderr(), map[int]compareFunc{})

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`rm %vstale.txt`, dst),
	})

	assert.Assert(t, ensureS3Object(s3client, bucket, "p/a.txt", "A"))
	assert.Assert(t, ensureS3Object(s3client, bucket, "p/sub/b.txt", "BB"))
	assert.Assert(t, !s3ObjectExists(t, s3client, backend, bucket, "p/stale.txt"))
	for _, marker := range markers {
		assert.Assert(t, s3ObjectExists(t, s3client, backend, bucket, marker), marker)
	}
}

// sync --delete s3://bucket/* .
func TestSyncS3BucketToEmptyLocalWithDelete(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"contributing.md": "S: this is a readme file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--delete", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)
	stdout := result.Stdout()
	assertLines(t, stdout, map[int]compareFunc{
		0: equals(`cp %vcontributing.md %vcontributing.md`, src, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("contributing.md", "S: this is a readme file"),
	}

	// assert local filesystem
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync --delete folder/ s3://bucket/*
func TestSyncLocalToS3BucketWithDelete(t *testing.T) {
	t.Parallel()

	now := time.Now()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// ensure source is older.
	folderLayout := []fs.PathOp{
		fs.WithFile("contributing.md", "S: this is a readme file", fs.WithTimestamps(now.Add(-time.Minute), now.Add(-time.Minute))),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"readme.md":    "D: this is a readme file",
		"dir/main.py":  "D: this is a python file",
		"testfile.txt": "D: this is a test file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vcontributing.md %vcontributing.md`, src, dst),
		1: equals(`rm %vdir/main.py`, dst),
		2: equals(`rm %vreadme.md`, dst),
		3: equals(`rm %vtestfile.txt`, dst),
	}, sortInput(true))

	// assert local filesystem
	expectedFiles := []fs.PathOp{
		fs.WithFile("contributing.md", "S: this is a readme file"),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"contributing.md": "S: this is a readme file",
	}

	// assert s3 objects
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects should be deleted.
	for key, content := range s3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		if err == nil {
			t.Errorf("File %v is not deleted from remote : %v\n", key, err)
		}
	}
}

// sync --delete folder/ s3://bucket/*
func TestSyncLocalToEmptyS3BucketWithDelete(t *testing.T) {
	t.Parallel()

	now := time.Now()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("contributing.md", "S: this is a readme file", fs.WithTimestamps(now, now)),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vcontributing.md %vcontributing.md`, src, dst),
	}, sortInput(true))

	// assert local filesystem
	expectedFiles := []fs.PathOp{
		fs.WithFile("contributing.md", "S: this is a readme file"),
	}
	expected := fs.Expected(t, expectedFiles...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"contributing.md": "S: this is a readme file",
	}

	// assert s3 objects
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync --delete s3://bucket/* s3://destbucket/
func TestSyncS3BucketToS3BucketWithDelete(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")
	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	sourceS3Content := map[string]string{
		"readme.md":    "S: this is a readme file",
		"dir/main.py":  "S: this is a python file",
		"testfile.txt": "S: this is a test file",
	}

	destS3Content := map[string]string{
		"main.md":      "D: this is a readme file",
		"dir/test.py":  "D: this is a python file",
		"testfile.txt": "D: this is an updated test file", // different size from source
		"Makefile":     "D: this is a makefile",
	}

	for filename, content := range sourceS3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	for filename, content := range destS3Content {
		putFile(t, s3client, dstbucket, filename, content)
	}

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	cmd := s5cmd("sync", "--delete", "--size-only", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vdir/main.py %vdir/main.py`, src, dst),
		1: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		2: equals(`cp %vtestfile.txt %vtestfile.txt`, src, dst),
		3: equals(`rm %vMakefile`, dst),
		4: equals(`rm %vdir/test.py`, dst),
		5: equals(`rm %vmain.md`, dst),
	}, sortInput(true))

	expectedDestS3Content := map[string]string{
		"testfile.txt": "S: this is a test file", // same as source bucket.
		"readme.md":    "S: this is a readme file",
		"dir/main.py":  "S: this is a python file",
	}

	nonExpectedDestS3Content := map[string]string{
		"dir/test.py": "S: this is a python file",
		"main.md":     "D: this is a readme file",
		"Makefile":    "S: this is a makefile",
	}

	// assert s3 objects in source.
	for key, content := range sourceS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects in destination. (should be)
	for key, content := range expectedDestS3Content {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}

	// assert s3 objects should be deleted.
	for key, content := range nonExpectedDestS3Content {
		err := ensureS3Object(s3client, dstbucket, key, content)
		if err == nil {
			t.Errorf("File %v is not deleted in remote : %v\n", key, err)
		}
	}
}

// sync s3://bucket/*.txt folder/
func TestSyncS3toLocalWithWildcard(t *testing.T) {
	t.Parallel()
	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	// make local (destination) older.
	timestamp := fs.WithTimestamps(
		now.Add(-time.Minute), // access time
		now.Add(-time.Minute), // mod time
	)

	// even though test.py exists in the source, since '*.txt' wildcard
	// used, test.py will not be in the source, because all of the source
	// files will be with extension '*.txt' therefore test.py will be deleted.
	folderLayout := []fs.PathOp{
		fs.WithFile("test.py", "D: this is a python file", timestamp),
		fs.WithFile("test.txt", "D: this is a test file", timestamp),
	}

	s3Content := map[string]string{
		"test.txt":          "S: this is an updated test file",
		"readme.md":         "S: this is a readme file",
		"main.py":           "S: py file",
		"subfolder/sub.txt": "S: yet another txt",
		"test.py":           "S: this is a python file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("--log", "debug", "sync", "--delete", src+"*.txt", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		1: equals(`cp %vtest.txt %vtest.txt`, src, dst),
		0: equals(`cp %vsubfolder/sub.txt %vsubfolder/sub.txt`, src, dst),
		2: equals(`rm %vtest.py`, dst),
	}, sortInput(true))

	expectedLayout := []fs.PathOp{
		fs.WithFile("test.txt", "S: this is an updated test file"),
		fs.WithDir("subfolder",
			fs.WithFile("sub.txt", "S: yet another txt"),
		),
	}

	expected := fs.Expected(t, expectedLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --delete s3://bucket/* .
func TestSyncS3BucketToLocalWithDeleteFlag(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"test.txt": "S: this is a test file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t,
		"somedir",
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir(
			"subdir",
			fs.WithFile("main.py", "D: this is a python file")),
	)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("--log", "debug", "sync", "--delete", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vtest.txt %vtest.txt`, src, dst),
		1: equals(`rm %vreadme.md`, dst),
		2: equals(`rm %vsubdir/main.py`, dst),
	}, sortInput(true))

	expectedLayout := []fs.PathOp{
		fs.WithFile("test.txt", "S: this is a test file"),
		fs.WithDir("subdir"),
	}

	expected := fs.Expected(t, expectedLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync dir/ s3://bucket (symlink)
func TestSyncLocalFilesWithSymlinksToS3Bucket(t *testing.T) {
	t.Parallel()
	requireSymlinks(t)

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	fileContent := "CAFEBABE"
	folderLayout := []fs.PathOp{
		fs.WithDir(
			"a",
			fs.WithFile("file1.txt", fileContent),
			fs.WithFile("file2.txt", fileContent),
		),
		fs.WithDir("b"),
		fs.WithSymlink("b/link1", "a/file1.txt"),
		fs.WithSymlink("b/link2", "a/file2.txt"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("%v/b", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/b/link1 %vb/link1`, filepath.ToSlash(workdir.Path()), dst),
		1: equals(`cp %v/b/link2 %vb/link2`, filepath.ToSlash(workdir.Path()), dst),
	}, sortInput(true))
}

// sync --no-follow-symlinks * s3://bucket/prefix/
func TestSyncLocalFilesWithNoFollowSymlinksToS3Bucket(t *testing.T) {
	t.Parallel()
	requireSymlinks(t)

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	fileContent := "CAFEBABE"
	folderLayout := []fs.PathOp{
		fs.WithDir(
			"a",
			fs.WithFile("file1.txt", fileContent),
			fs.WithFile("file2.txt", fileContent),
		),
		fs.WithDir("b"),
		fs.WithSymlink("b/link1", "a/file1.txt"),
		fs.WithSymlink("b/link2", "a/file2.txt"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("%v/b", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--no-follow-symlinks", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	// do not follow symlinks in directory b (empty result)
	assertLines(t, result.Stdout(), map[int]compareFunc{})
}

// sync --exclude pattern s3://bucket/* s3://anotherbucket/prefix/
func TestSyncS3ObjectsIntoAnotherBucketWithExcludeFilters(t *testing.T) {
	t.Parallel()

	srcbucket := s3BucketFromTestNameWithPrefix(t, "src")
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	s3client, s5cmd := setup(t)

	createBucket(t, s3client, srcbucket)
	createBucket(t, s3client, dstbucket)

	srcFiles := []string{
		"file_already_exists_in_destination.txt",
		"file_not_exists_in_destination.txt",
		"main.py",
		"main.js",
		"readme.md",
		"main.pdf",
		"main/file.txt",
	}

	dstFiles := []string{
		"prefix/file_already_exists_in_destination.txt",
	}

	expectedFiles := []string{
		"prefix/file_not_exists_in_destination.txt",
		"prefix/file_already_exists_in_destination.txt",
	}

	excludedFiles := []string{
		"main.py",
		"main.js",
		"main.pdf",
		"main/file.txt",
		"readme.md",
	}

	const (
		content         = "this is a file content"
		excludePattern1 = "main*"
		excludePattern2 = "*.md"
	)

	for _, filename := range srcFiles {
		putFile(t, s3client, srcbucket, filename, content)
	}

	for _, filename := range dstFiles {
		putFile(t, s3client, dstbucket, filename, content)
	}

	src := fmt.Sprintf("s3://%v/*", srcbucket)
	dst := fmt.Sprintf("s3://%v/prefix/", dstbucket)

	cmd := s5cmd("sync", "--exclude", excludePattern1, "--exclude", excludePattern2, src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%s/file_not_exists_in_destination.txt s3://%s/prefix/file_not_exists_in_destination.txt`, srcbucket, dstbucket),
	}, sortInput(true))

	// assert s3 source objects
	for _, filename := range srcFiles {
		assert.Assert(t, ensureS3Object(s3client, srcbucket, filename, content))
	}

	// assert s3 destination objects
	for _, filename := range expectedFiles {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, filename, content))
	}

	// assert s3 destination objects which should not be in bucket.
	for _, filename := range excludedFiles {
		err := ensureS3Object(s3client, dstbucket, filename, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --exclude "*.gz" dir s3://bucket/
// sync --exclude "*.gz" dir/ s3://bucket/
// sync --exclude "*.gz" dir/* s3://bucket/
func TestSyncLocalDirectoryToS3WithExcludeFilter(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name            string
		directoryPrefix string
	}{
		{
			name:            "folder without /",
			directoryPrefix: "",
		},
		{
			name:            "folder with /",
			directoryPrefix: "/",
		},
		{
			name:            "folder with / and glob *",
			directoryPrefix: "/*",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bucket := s3BucketFromTestName(t)

			s3client, s5cmd := setup(t)

			createBucket(t, s3client, bucket)

			folderLayout := []fs.PathOp{
				fs.WithFile("testfile1.txt", "this is a test file 1"),
				fs.WithFile("readme.md", "this is a readme file"),
				fs.WithDir(
					"a",
					fs.WithFile("another_test_file.txt", "yet another txt file. yatf."),
				),
				fs.WithDir(
					"b",
					fs.WithFile("filename-with-hypen.gz", "file has hypen in its name"),
				),
			}

			workdir := fs.NewDir(t, "somedir", folderLayout...)
			defer workdir.Remove()

			const excludePattern = "*.gz"

			src := fmt.Sprintf("%v/", workdir.Path())
			src = src + tc.directoryPrefix
			dst := fmt.Sprintf("s3://%v/prefix/", bucket)

			src = filepath.ToSlash(src)
			cmd := s5cmd("sync", "--exclude", excludePattern, src, dst)
			result := icmd.RunCmd(cmd)

			result.Assert(t, icmd.Success)

			// assert local filesystem
			expected := fs.Expected(t, folderLayout...)
			assert.Assert(t, fs.Equal(workdir.Path(), expected))

			expectedS3Content := map[string]string{
				"prefix/testfile1.txt":           "this is a test file 1",
				"prefix/readme.md":               "this is a readme file",
				"prefix/a/another_test_file.txt": "yet another txt file. yatf.",
			}

			nonExpectedS3Content := map[string]string{
				"prefix/b/filename-with-hypen.gz": "file has hypen in its name",
			}

			// assert objects should be in S3
			for key, content := range expectedS3Content {
				assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
			}

			// assert objects should not be in S3.
			for key, content := range nonExpectedS3Content {
				err := ensureS3Object(s3client, bucket, key, content)
				assertError(t, err, errS3NoSuchKey)
			}
		})
	}
}

// sync --delete --exclude "sub/*" folder/ s3://bucket/prefix/
func TestSyncLocalToS3BucketWithDeleteAndExcludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"prefix/sub/keep.txt": "D: this is a text file",
		"prefix/old.log":      "D: this is a log file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	// pattern is relative to the destination prefix.
	const excludePattern = "sub/*"

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	cmd := s5cmd("sync", "--delete", "--exclude", excludePattern, src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`rm %vold.log`, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md": "S: this is a readme file",
		// excluded object exists only in destination and must not be deleted.
		"prefix/sub/keep.txt": "D: this is a text file",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/old.log": "D: this is a log file",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete --exclude-from patterns.txt folder/ s3://bucket/prefix/
func TestSyncLocalToS3BucketWithDeleteAndExcludeFromFile(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithFile("notes.tmp", "S: this is a temp file"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"prefix/sub/keep.txt": "D: this is a text file",
		"prefix/old.log":      "D: this is a log file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	// patterns are relative to the source and destination prefixes, like --exclude.
	const patternFileContent = "# skip temp files and the sub folder\n*.tmp\n\nsub/*\n"

	patterndir := fs.NewDir(t, "patterns", fs.WithFile("patterns.txt", patternFileContent))
	defer patterndir.Remove()

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	cmd := s5cmd("sync", "--delete", "--exclude-from", patterndir.Join("patterns.txt"), src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`rm %vold.log`, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md": "S: this is a readme file",
		// excluded object exists only in destination and must not be deleted.
		"prefix/sub/keep.txt": "D: this is a text file",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/old.log":   "D: this is a log file",
		"prefix/notes.tmp": "S: this is a temp file",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete --exclude "sub/*" s3://bucket/* folder/
func TestSyncS3BucketToLocalWithDeleteAndExcludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"readme.md": "S: this is a readme file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	folderLayout := []fs.PathOp{
		fs.WithFile("old.log", "D: this is a log file"),
		fs.WithDir("sub",
			fs.WithFile("keep.txt", "D: this is a text file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	// pattern is relative to the destination directory.
	const excludePattern = "sub/*"

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--delete", "--exclude", excludePattern, src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`rm %vold.log`, dst),
	}, sortInput(true))

	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		// excluded file exists only in destination and must not be deleted.
		fs.WithDir("sub",
			fs.WithFile("keep.txt", "D: this is a text file"),
		),
	}

	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync s3://bucket/* dir/  (object key contains "..")
func TestSyncS3ObjectsToLocalWithPathTraversalKey(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	putFile(t, s3client, bucket, "data/ok.txt", "ok")
	putFile(t, s3client, bucket, "data/../../escape.txt", "pwned")

	workdir := fs.NewDir(t, "somedir", fs.WithDir("dest"))
	defer workdir.Remove()

	cmd := s5cmd("sync", "s3://"+bucket+"/*", "dest/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%v/data/ok.txt dest/data/ok.txt`, bucket),
	})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`escapes destination`),
	})

	expected := fs.Expected(t,
		fs.WithDir("dest",
			fs.WithDir("data",
				fs.WithFile("ok.txt", "ok"),
			),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync s3://bucket/prefix/missing.txt newdir/  (object does not exist)
//
// The destination directory not existing is fine: the generated cp creates
// it. The error is that the source matched nothing, and it must be reflected
// in the exit code.
func TestSyncMissingS3ObjectToLocalDirectory(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, "prefix/file.txt", "content")

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/prefix/missing.txt", bucket)

	cmd := s5cmd("sync", src, "newdir/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v newdir/": no object found`, src),
	})

	// nothing was synced, so nothing was created
	expected := fs.Expected(t)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync s3://bucket/missing/* newdir/  (prefix matches nothing)
func TestSyncMissingS3PrefixToLocalDirectory(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)
	putFile(t, s3client, bucket, "prefix/file.txt", "content")

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/missing/*", bucket)

	cmd := s5cmd("sync", src, "newdir/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: equals(`ERROR "sync %v newdir/": no object found`, src),
	})
}

// sync s3://NotExistingBucket/* newdir/  (source bucket doesn't exist)
func TestSyncS3BucketThatDoesNotExistToLocal(t *testing.T) {
	t.Parallel()

	_, s5cmd := setup(t)

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := "s3://NotExistingBucket/*"

	cmd := s5cmd("sync", src, "newdir/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`status code: 404`),
	})
}

// sync --exit-on-error s3://bucket/* dest/  (dest exists and is empty)
//
// Upstream #810: the destination is listed as "dest/*" to find what is
// already there. An empty directory matches nothing, which is not an error:
// it only means everything must be copied.
func TestSyncS3ObjectsToEmptyLocalDirectoryWithExitOnErrorFlag(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"testfile.txt":   "S: this is a test file",
		"a/nested/b.txt": "S: nested file",
		"readme.md":      "S: this is a readme file",
	}
	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t, "somedir", fs.WithDir("dest"))
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/*", bucket)

	cmd := s5cmd("sync", "--exit-on-error", src, "dest/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%v/a/nested/b.txt dest/a/nested/b.txt`, bucket),
		1: equals(`cp s3://%v/readme.md dest/readme.md`, bucket),
		2: equals(`cp s3://%v/testfile.txt dest/testfile.txt`, bucket),
	}, sortInput(true))
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithDir("dest",
			fs.WithDir("a",
				fs.WithDir("nested",
					fs.WithFile("b.txt", "S: nested file"),
				),
			),
			fs.WithFile("readme.md", "S: this is a readme file"),
			fs.WithFile("testfile.txt", "S: this is a test file"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --exit-on-error s3://bucket/* newdir/  (dest does not exist yet)
func TestSyncS3ObjectsToMissingLocalDirectoryWithExitOnErrorFlag(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	putFile(t, s3client, bucket, "testfile.txt", "S: this is a test file")
	putFile(t, s3client, bucket, "a/b.txt", "S: nested file")

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/*", bucket)

	cmd := s5cmd("sync", "--exit-on-error", src, "newdir/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%v/a/b.txt newdir/a/b.txt`, bucket),
		1: equals(`cp s3://%v/testfile.txt newdir/testfile.txt`, bucket),
	}, sortInput(true))
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithDir("newdir",
			fs.WithDir("a",
				fs.WithFile("b.txt", "S: nested file"),
			),
			fs.WithFile("testfile.txt", "S: this is a test file"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync s3://bucket/* "data[2024]/"  (dest directory name contains glob characters)
//
// The destination is a path, never a pattern. "data[2024]/*" read as a glob
// matches nothing (it wants a single character out of 2, 0 or 4), so the
// files already in the directory were invisible and copied again on every
// run. They must be seen, and only the new object copied.
func TestSyncS3ObjectsToLocalDirectoryWithGlobCharactersInName(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	const dir = "data[2024]"

	// the destination copy is newer than the source, so it must be kept.
	now := time.Now()
	timestamp := fs.WithTimestamps(
		now.Add(time.Minute), // access time
		now.Add(time.Minute), // mod time
	)

	putFile(t, s3client, bucket, "testfile.txt", "S: this is a test file")
	putFile(t, s3client, bucket, "readme.md", "S: this is a readme file")

	workdir := fs.NewDir(t, "somedir",
		fs.WithDir(dir,
			fs.WithFile("testfile.txt", "D: this is a test file", timestamp),
		),
	)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/*", bucket)

	cmd := s5cmd("sync", src, dir+"/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%v/readme.md %v/readme.md`, bucket, dir),
	})
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithDir(dir,
			fs.WithFile("readme.md", "S: this is a readme file"),
			fs.WithFile("testfile.txt", "D: this is a test file"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --delete s3://bucket/* "logs[a-z]/"  (dest directory name contains glob characters)
//
// With --delete the invisible destination also meant nothing was ever removed.
func TestSyncS3ObjectsToLocalDirectoryWithGlobCharactersInNameWithDelete(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	const dir = "logs[a-z]"

	now := time.Now()
	timestamp := fs.WithTimestamps(
		now.Add(time.Minute), // access time
		now.Add(time.Minute), // mod time
	)

	putFile(t, s3client, bucket, "testfile.txt", "S: this is a test file")

	workdir := fs.NewDir(t, "somedir",
		fs.WithDir(dir,
			fs.WithFile("testfile.txt", "D: this is a test file", timestamp),
			fs.WithFile("stale.txt", "D: only in destination", timestamp),
		),
	)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/*", bucket)

	cmd := s5cmd("sync", "--delete", src, dir+"/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`rm %v/stale.txt`, dir),
	})
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithDir(dir,
			fs.WithFile("testfile.txt", "D: this is a test file"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync s3://bucket/* C:\somedir\dest[1]\  (native destination path)
//
// The destination is given as the OS writes it: on Windows with backslashes,
// which s5cmd turns into slashes.
func TestSyncS3ObjectsToNativeLocalDirectoryPath(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	const dir = "dest[1]"

	now := time.Now()
	timestamp := fs.WithTimestamps(
		now.Add(time.Minute), // access time
		now.Add(time.Minute), // mod time
	)

	putFile(t, s3client, bucket, "testfile.txt", "S: this is a test file")
	putFile(t, s3client, bucket, "sub/readme.md", "S: this is a readme file")

	workdir := fs.NewDir(t, "somedir",
		fs.WithDir(dir,
			fs.WithFile("testfile.txt", "D: this is a test file", timestamp),
		),
	)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/*", bucket)
	dst := workdir.Join(dir) + string(filepath.Separator)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%v/sub/readme.md %vsub/readme.md`, bucket, filepath.ToSlash(dst)),
	})
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := fs.Expected(t,
		fs.WithDir(dir,
			fs.WithDir("sub",
				fs.WithFile("readme.md", "S: this is a readme file"),
			),
			fs.WithFile("testfile.txt", "D: this is a test file"),
		),
	)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync "data[2024]/*" s3://bucket/  (source directory name contains glob characters)
func TestSyncLocalDirectoryWithGlobCharactersInNameToS3(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	const dir = "data[2024]"

	workdir := fs.NewDir(t, "somedir",
		fs.WithDir(dir,
			fs.WithFile("testfile.txt", "S: this is a test file"),
			fs.WithDir("sub",
				fs.WithFile("readme.md", "S: this is a readme file"),
			),
		),
	)
	defer workdir.Remove()

	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", dir+"/*", dst)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/sub/readme.md %vsub/readme.md`, dir, dst),
		1: equals(`cp %v/testfile.txt %vtestfile.txt`, dir, dst),
	}, sortInput(true))
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	assert.Assert(t, ensureS3Object(s3client, bucket, "testfile.txt", "S: this is a test file"))
	assert.Assert(t, ensureS3Object(s3client, bucket, "sub/readme.md", "S: this is a readme file"))
}

// sync --delete --include "*.md" --include "sub/*" folder/ s3://bucket/prefix/
func TestSyncLocalToS3BucketWithDeleteAndIncludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"prefix/keep.txt":    "D: this is a text file",
		"prefix/sub/old.log": "D: this is a log file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	// patterns are relative to the destination prefix.
	cmd := s5cmd("sync", "--delete", "--include", "*.md", "--include", "sub/*", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`rm %vsub/old.log`, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md": "S: this is a readme file",
		// object not matching --include exists only in destination and must not be deleted.
		"prefix/keep.txt": "D: this is a text file",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/sub/old.log": "D: this is a log file",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --exclude "sub/*" folder/ s3://bucket/prefix/
func TestSyncLocalToS3BucketWithExcludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithFile("testfile1.txt", "S: this is a test file 1"),
		fs.WithDir("sub",
			fs.WithFile("new.log", "S: this is a log file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	// pattern is relative to the source directory.
	const excludePattern = "sub/*"

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	cmd := s5cmd("sync", "--exclude", excludePattern, src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`cp %vtestfile1.txt %vtestfile1.txt`, src, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md":     "S: this is a readme file",
		"prefix/testfile1.txt": "S: this is a test file 1",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/sub/new.log": "S: this is a log file",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --exclude "sub/*" s3://bucket/prefix/* folder/
func TestSyncS3BucketToLocalWithExcludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"prefix/readme.md":     "S: this is a readme file",
		"prefix/testfile1.txt": "S: this is a test file 1",
		"prefix/sub/new.log":   "S: this is a log file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	workdir := fs.NewDir(t, "somedir")
	defer workdir.Remove()

	// pattern is relative to the source prefix, not the full object key.
	const excludePattern = "sub/*"

	src := fmt.Sprintf("s3://%v/prefix/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("sync", "--exclude", excludePattern, src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`cp %vtestfile1.txt %vtestfile1.txt`, src, dst),
	}, sortInput(true))

	// excluded object must not be downloaded, so no "sub" directory.
	expectedFolderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithFile("testfile1.txt", "S: this is a test file 1"),
	}

	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync --include "*.md" --include "sub/*" folder/ s3://bucket/prefix/
func TestSyncLocalToS3BucketWithIncludeFilter(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithFile("testfile1.txt", "S: this is a test file 1"),
		fs.WithDir("sub",
			fs.WithFile("new.log", "S: this is a log file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	// patterns are relative to the source directory.
	cmd := s5cmd("sync", "--include", "*.md", "--include", "sub/*", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`cp %vsub/new.log %vsub/new.log`, src, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md":   "S: this is a readme file",
		"prefix/sub/new.log": "S: this is a log file",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/testfile1.txt": "S: this is a test file 1",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete --exclude "sub/*" folder/ s3://bucket/prefix/
//
// An excluded object that exists on both sides with different content is
// outside the sync: it is neither copied nor deleted.
func TestSyncLocalToS3BucketWithDeleteAndExcludeFilterCommonObject(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("readme.md", "S: this is a readme file"),
		fs.WithDir("sub",
			fs.WithFile("x.txt", "S: this is a much longer text file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"prefix/sub/x.txt": "D: short",
		"prefix/old.log":   "D: this is a log file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	// pattern is relative to the source directory and destination prefix.
	const excludePattern = "sub/*"

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/prefix/", bucket)

	cmd := s5cmd("sync", "--delete", "--exclude", excludePattern, src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %vreadme.md %vreadme.md`, src, dst),
		1: equals(`rm %vold.log`, dst),
	}, sortInput(true))

	// assert local filesystem
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	expectedS3Content := map[string]string{
		"prefix/readme.md": "S: this is a readme file",
		// excluded object exists on both sides: neither overwritten nor deleted.
		"prefix/sub/x.txt": "D: short",
	}

	nonExpectedS3Content := map[string]string{
		"prefix/old.log": "D: this is a log file",
	}

	// assert objects should be in S3
	for key, content := range expectedS3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert objects should not be in S3.
	for key, content := range nonExpectedS3Content {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete somedir s3://bucket/ (removes 10k objects)
func TestIssue435(t *testing.T) {
	t.Parallel()

	// Skip this as it takes too long to complete with GCS.
	skipTestIfGCS(t, "takes too long to complete")

	bucket := s3BucketFromTestName(t)

	s3client, s5cmd := setup(t, withS3Backend("mem"))

	createBucket(t, s3client, bucket)

	// empty folder
	folderLayout := []fs.PathOp{}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	const filecount = 10_000

	filenameFunc := func(i int) string { return fmt.Sprintf("file_%06d", i) }
	contentFunc := func(i int) string { return fmt.Sprintf("file body %06d", i) }

	for i := 0; i < filecount; i++ {
		filename := filenameFunc(i)
		content := contentFunc(i)
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("--log", "debug", "sync", "--delete", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stderr(), map[int]compareFunc{})

	expected := make(map[int]compareFunc)
	for i := 0; i < filecount; i++ {
		expected[i] = contains("rm s3://%v/file_%06d", bucket, i)
	}

	assertLines(t, result.Stdout(), expected, sortInput(true))

	// assert s3 objects
	for i := 0; i < filecount; i++ {
		filename := filenameFunc(i)
		content := contentFunc(i)

		err := ensureS3Object(s3client, bucket, filename, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync s3://bucket/* s3://bucket/ (dest bucket is empty)
func TestSyncS3BucketToEmptyS3BucketWithExitOnErrorFlag(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	const (
		prefix = "prefix"
	)
	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	s3Content := map[string]string{
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
		"abc/def/test.py":         "S: file in nested folders",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	bucketPath := fmt.Sprintf("s3://%v", bucket)
	src := fmt.Sprintf("%v/*", bucketPath)
	dst := fmt.Sprintf("s3://%v/%v/", dstbucket, prefix)

	cmd := s5cmd("sync", "--exit-on-error", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/a/another_test_file.txt %va/another_test_file.txt`, bucketPath, dst),
		1: equals(`cp %v/abc/def/test.py %vabc/def/test.py`, bucketPath, dst),
		2: equals(`cp %v/readme.md %vreadme.md`, bucketPath, dst),
		3: equals(`cp %v/testfile.txt %vtestfile.txt`, bucketPath, dst),
	}, sortInput(true))

	// assert  s3 objects in source bucket.
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}

	// assert s3 objects in dest bucket
	for key, content := range s3Content {
		key = fmt.Sprintf("%s/%s", prefix, key) // add the prefix
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, content))
	}
}

// sync --exit-on-error s3://bucket/* s3://NotExistingBucket/ (dest bucket doesn't exist)
func TestSyncExitOnErrorS3BucketToS3BucketThatDoesNotExist(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	destbucket := "NotExistingBucket"

	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
		"abc/def/test.py":         "S: file in nested folders",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("s3://%v/*", bucket)
	dst := fmt.Sprintf("s3://%v/", destbucket)

	cmd := s5cmd("sync", "--exit-on-error", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`status code: 404`),
	}, strictLineCheck(false))
}

// sync s3://bucket/* s3://NotExistingBucket/ (dest bucket doesn't exist)
func TestSyncS3BucketToS3BucketThatDoesNotExist(t *testing.T) {
	t.Parallel()

	now := time.Now()
	timeSource := newFixedTimeSource(now)
	s3client, s5cmd := setup(t, withTimeSource(timeSource))

	bucket := s3BucketFromTestName(t)
	destbucket := "NotExistingBucket"

	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"testfile.txt":            "S: this is a test file",
		"readme.md":               "S: this is a readme file",
		"a/another_test_file.txt": "S: yet another txt file",
		"abc/def/test.py":         "S: file in nested folders",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	src := fmt.Sprintf("s3://%v/*", bucket)
	dst := fmt.Sprintf("s3://%v/", destbucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`status code: 404`),
	})
}

// If source path contains a special file it should not be synced
func TestSyncSocketDestinationEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	t.Parallel()

	s3client, s5cmd := setup(t)
	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, t.Name())
	defer workdir.Remove()

	sockaddr := workdir.Join("/s5cmd.sock")
	ln, err := net.Listen("unix", sockaddr)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ln.Close()
		os.Remove(sockaddr)
	})

	cmd := s5cmd("sync", ".", "s3://"+bucket+"/")
	result := icmd.RunCmd(cmd, withWorkingDir(workdir))

	// assert error message
	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`is not a regular file`),
	})

	// assert logs are empty (no sync)
	assertLines(t, result.Stdout(), nil)

	// assert exit code
	result.Assert(t, icmd.Expected{ExitCode: 1})
}

// sync --include pattern s3://bucket/* s3://anotherbucket/prefix/
func TestSyncS3ObjectsIntoAnotherBucketWithIncludeFilters(t *testing.T) {
	t.Parallel()

	srcbucket := s3BucketFromTestNameWithPrefix(t, "src")
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	s3client, s5cmd := setup(t)

	createBucket(t, s3client, srcbucket)
	createBucket(t, s3client, dstbucket)

	srcFiles := []string{
		"file_already_exists_in_destination.txt",
		"file_not_exists_in_destination.txt",
		"main.py",
		"main.js",
		"readme.md",
		"main.pdf",
		"main/file.txt",
	}

	dstFiles := []string{
		"prefix/file_already_exists_in_destination.txt",
	}

	excludedFiles := []string{
		"prefix/file_not_exists_in_destination.txt",
	}

	includedFiles := []string{
		"main.js",
		"main.pdf",
		"main.py",
		"main/file.txt",
		"readme.md",
	}

	const (
		content         = "this is a file content"
		includePattern1 = "main*"
		includePattern2 = "*.md"
	)

	for _, filename := range srcFiles {
		putFile(t, s3client, srcbucket, filename, content)
	}

	for _, filename := range dstFiles {
		putFile(t, s3client, dstbucket, filename, content)
	}

	src := fmt.Sprintf("s3://%v/*", srcbucket)
	dst := fmt.Sprintf("s3://%v/prefix/", dstbucket)

	cmd := s5cmd("sync", "--include", includePattern1, "--include", includePattern2, src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp s3://%s/%s s3://%s/prefix/%s`, srcbucket, includedFiles[0], dstbucket, includedFiles[0]),
		1: equals(`cp s3://%s/%s s3://%s/prefix/%s`, srcbucket, includedFiles[1], dstbucket, includedFiles[1]),
		2: equals(`cp s3://%s/%s s3://%s/prefix/%s`, srcbucket, includedFiles[2], dstbucket, includedFiles[2]),
		3: equals(`cp s3://%s/%s s3://%s/prefix/%s`, srcbucket, includedFiles[3], dstbucket, includedFiles[3]),
		4: equals(`cp s3://%s/%s s3://%s/prefix/%s`, srcbucket, includedFiles[4], dstbucket, includedFiles[4]),
	}, sortInput(true))

	// assert s3 source objects
	for _, filename := range srcFiles {
		assert.Assert(t, ensureS3Object(s3client, srcbucket, filename, content))
	}

	// assert s3 destination objects
	for _, filename := range includedFiles {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, "prefix/"+filename, content))
	}

	// assert s3 destination objects which should not be in bucket.
	for _, filename := range excludedFiles {
		err := ensureS3Object(s3client, dstbucket, filename, content)
		assertError(t, err, errS3NoSuchKey)
	}
}

// sync --delete ./dist s3://bucket/ (and the other ways to name the folder)
//
// Coverage for peak/s5cmd#852: a relative source folder, with or without a
// trailing slash, or an absolute one. Objects that are not in the source
// must be removed from the bucket in every case.
func TestSyncLocalFolderToS3BucketWithDeleteSourceForms(t *testing.T) {
	t.Parallel()

	sources := []struct {
		name string
		src  func(workdir *fs.Dir) string
		// prefix the objects get in the bucket: a folder given without a
		// trailing slash is uploaded as a folder, like cp does.
		prefix string
	}{
		{"relative", func(*fs.Dir) string { return "./dist" }, "dist/"},
		{"relative-trailing-slash", func(*fs.Dir) string { return "dist/" }, ""},
		{"absolute", func(w *fs.Dir) string { return filepath.ToSlash(w.Join("dist")) }, "dist/"},
	}

	for _, tc := range sources {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s3client, s5cmd := setup(t)

			bucket := s3BucketFromTestName(t)
			createBucket(t, s3client, bucket)

			workdir := fs.NewDir(t, "workdir", fs.WithDir("dist",
				fs.WithFile("index.html", "S: index"),
				fs.WithDir("assets", fs.WithFile("app.js", "S: app")),
			))
			defer workdir.Remove()

			// only in the bucket: must be deleted.
			stale := map[string]string{
				"stale.html":               "D: stale",
				tc.prefix + "old.html":     "D: old",
				tc.prefix + "assets/x.css": "D: x",
			}
			for key, content := range stale {
				putFile(t, s3client, bucket, key, content)
			}

			src := tc.src(workdir)
			dst := fmt.Sprintf("s3://%v/", bucket)

			cmd := s5cmd("sync", "--delete", src, dst)
			result := icmd.RunCmd(cmd, withWorkingDir(workdir))

			result.Assert(t, icmd.Success)

			assertLines(t, result.Stdout(), map[int]compareFunc{
				0: suffix(`dist/assets/app.js %v%vassets/app.js`, dst, tc.prefix),
				1: suffix(`dist/index.html %v%vindex.html`, dst, tc.prefix),
				2: equals(`rm %v%vassets/x.css`, dst, tc.prefix),
				3: equals(`rm %v%vold.html`, dst, tc.prefix),
				4: equals(`rm %vstale.html`, dst),
			}, sortInput(true))

			// assert s3 objects
			assert.Assert(t, ensureS3Object(s3client, bucket, tc.prefix+"index.html", "S: index"))
			assert.Assert(t, ensureS3Object(s3client, bucket, tc.prefix+"assets/app.js", "S: app"))
			for key, content := range stale {
				err := ensureS3Object(s3client, bucket, key, content)
				assertError(t, err, errS3NoSuchKey)
			}
		})
	}
}

// sync --delete --destination-region eu-west-1 ./dist s3://bucket/
//
// The bucket is in eu-west-1 but the environment says us-east-1, as in
// peak/s5cmd#852 (GitHub Actions). The generated rm command must remove the
// objects from the bucket's region.
func TestSyncLocalFolderToS3BucketInAnotherRegionWithDelete(t *testing.T) {
	t.Parallel()

	const bucketRegion = "eu-west-1"

	s3client, s5cmd := setup(t, withBucketRegion(bucketRegion))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, "workdir", fs.WithDir("dist",
		fs.WithFile("index.html", "S: index"),
	))
	defer workdir.Remove()

	putFile(t, s3client, bucket, "dist/stale.html", "D: stale")

	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", "--destination-region", bucketRegion, "./dist", dst)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir), withEnv("AWS_REGION", "us-east-1"))

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp dist/index.html %vdist/index.html`, dst),
		1: equals(`rm %vdist/stale.html`, dst),
	}, sortInput(true))

	assertLines(t, result.Stderr(), map[int]compareFunc{})

	// assert s3 objects
	assert.Assert(t, ensureS3Object(s3client, bucket, "dist/index.html", "S: index"))

	err := ensureS3Object(s3client, bucket, "dist/stale.html", "D: stale")
	assertError(t, err, errS3NoSuchKey)
}

// sync --delete ./dist s3://bucket/ (bucket in another region, no region flag)
//
// Listing the destination fails with a BucketRegionError. The sync must report
// that and stop: it must not treat the bucket as empty, re-upload everything
// and quietly skip the deletions, which is what peak/s5cmd#852 saw.
func TestSyncLocalFolderToS3BucketInAnotherRegionFails(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t, withBucketRegion("eu-west-1"))

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	workdir := fs.NewDir(t, "workdir", fs.WithDir("dist",
		fs.WithFile("index.html", "S: index"),
	))
	defer workdir.Remove()

	putFile(t, s3client, bucket, "dist/stale.html", "D: stale")

	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", "./dist", dst)
	result := icmd.RunCmd(cmd, withWorkingDir(workdir), withEnv("AWS_REGION", "us-east-1"))

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`ERROR "sync --delete=true ./dist %v": BucketRegionError: incorrect region`, dst),
	}, strictLineCheck(false))

	// nothing was deleted or uploaded.
	assert.Assert(t, ensureS3Object(s3client, bucket, "dist/stale.html", "D: stale"))

	err := ensureS3Object(s3client, bucket, "dist/index.html", "S: index")
	assertError(t, err, errS3NoSuchKey)
}

// sync --delete folder/ s3://bucket/ (listing the folder fails part way)
//
// A dangling symlink aborts the directory walk, so the files after it are
// never listed. The sync must stop instead of deleting their copies from the
// bucket.
func TestSyncLocalFolderWithDanglingSymlinkToS3BucketWithDelete(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on windows")
	}

	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithFile("a.txt", "S: a"),
		fs.WithSymlink("m.txt", "does-not-exist"),
		fs.WithFile("z.txt", "S: z"),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	s3Content := map[string]string{
		"a.txt": "S: a",
		"z.txt": "S: z",
	}
	for key, content := range s3Content {
		putFile(t, s3client, bucket, key, content)
	}

	src := fmt.Sprintf("%v/", workdir.Path())
	src = filepath.ToSlash(src)
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--delete", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`ERROR "sync --delete=true %v %v": `, src, dst),
	})

	// nothing was deleted.
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// --stat sync --delete s3://bucket/* folder/
//
// The rm generated by --delete removes every destination-only file with a
// single command; the stat table must count each file removed (upstream
// peak/s5cmd#649).
func TestSyncS3BucketToLocalWithDeleteAndStat(t *testing.T) {
	t.Parallel()
	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	s3Content := map[string]string{
		"contributing.md": "S: this is a readme file",
	}

	for filename, content := range s3Content {
		putFile(t, s3client, bucket, filename, content)
	}

	folderLayout := []fs.PathOp{
		fs.WithFile("testfile.txt", "D: this is a test file"),
		fs.WithFile("readme.md", "D: this is a readme file"),
		fs.WithDir("dir",
			fs.WithFile("main.py", "D: python file"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := fmt.Sprintf("s3://%v/", bucket)
	dst := fmt.Sprintf("%v/", workdir.Path())
	dst = filepath.ToSlash(dst)

	cmd := s5cmd("--stat", "sync", "--delete", src+"*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	output, stats := splitStatTable(t, result.Stdout())

	assertLines(t, output, map[int]compareFunc{
		0: equals(`cp %vcontributing.md %vcontributing.md`, src, dst),
		1: equals(`rm %vdir/main.py`, dst),
		2: equals(`rm %vreadme.md`, dst),
		3: equals(`rm %vtestfile.txt`, dst),
	}, sortInput(true))

	assert.DeepEqual(t, stats, map[string]string{
		"cp":   "1 0 1",
		"rm":   "3 0 3",
		"sync": "1 0 1",
	})

	expectedFolderLayout := []fs.PathOp{
		fs.WithDir("dir"),
		fs.WithFile("contributing.md", "S: this is a readme file"),
	}

	// assert local filesystem
	expected := fs.Expected(t, expectedFolderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))

	// assert s3
	for key, content := range s3Content {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
}

// sync dir/ s3://bucket/
//
// upstream peak/s5cmd#751: one file whose name is not valid UTF-8 (a Latin-1
// "é", as left behind by an old application) made url.New fail with "error
// parsing regexp: invalid UTF-8", which stopped the directory walk. sync
// then uploaded only the files seen so far. A name with a newline broke the
// generated cp line in two, which ended the run.
func TestSyncLocalFolderWithAwkwardFileNamesToS3(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "darwin" {
		// APFS refuses file names that are not valid UTF-8 ("illegal byte
		// sequence"), so the Latin-1 name below cannot exist there.
		t.Skip("macOS file systems reject non-UTF-8 file names")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names are UTF-16: they cannot hold invalid UTF-8 or a newline")
	}

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	content := map[string]string{
		"aaa first.txt":                     "first",
		"caf\xe9 samedi 31.07.flv":          "latin-1 e-acute",
		"new\nline.txt":                     "newline",
		"it's \"quoted\" $and\\ back`slash": "shell characters",
		"glob*star?mark.txt":                "glob characters",
		"plus+sign %20percent":              "url characters",
		"narrow no-break 日本語 é 🎉.png":       "unicode",
		"ctrl\x01\x7fchars":                 "control characters",
		"zzz last.txt":                      "last",
	}

	var layout []fs.PathOp
	for name, body := range content {
		layout = append(layout, fs.WithFile(name, body))
	}
	workdir := fs.NewDir(t, "somedir", layout...)
	defer workdir.Remove()

	src := workdir.Path() + "/"
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	for name, body := range content {
		assert.Assert(t, ensureS3Object(s3client, bucket, name, body), "key %q", name)
	}

	// A second sync finds nothing to do for the keys the fake can list:
	// its listing is plain XML, in which Go writes an invalid byte and a
	// control character as U+FFFD, and it ignores encoding-type=url. Real
	// S3 honours it; see TestS3ListURLEncodedKeys in the storage package.
	cmd = s5cmd("sync", src, dst)
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stderr(), map[int]compareFunc{})
	for name := range content {
		if name == "caf\xe9 samedi 31.07.flv" || name == "ctrl\x01\x7fchars" {
			continue
		}
		assert.Assert(t, !strings.Contains(result.Stdout(), name), "%q was copied again:\n%s", name, result.Stdout())
	}
}

// sync s3://bucket/* s3://dstbucket/
//
// The planned cp commands travel through the run command as shell-quoted
// text. Every key must come out the other end unchanged, including one that
// holds a newline: the shell-quoted form of that key spans two lines.
func TestSyncS3BucketToS3BucketKeysWithShellCharacters(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	dstbucket := s3BucketFromTestNameWithPrefix(t, "dst")

	createBucket(t, s3client, bucket)
	createBucket(t, s3client, dstbucket)

	content := map[string]string{
		"aaa first.txt":                  "first",
		"new\nline.txt":                  "newline",
		"dir/it's \"quoted\" $and `tick": "shell characters",
		"glob*star?mark.txt":             "glob characters",
		"plus+sign %20percent":           "url characters",
		"narrow no-break 日本語 é 🎉.png":    "unicode",
		" leading and trailing space ":   "spaces",
		"zzz last.txt":                   "last",
	}
	if runtime.GOOS != "windows" {
		// url.Join turns a backslash into a slash on Windows, meant for local
		// relative paths; it does so for remote keys too. That is a separate,
		// Windows-only bug.
		content["dir/back\\slash"] = "backslash"
	}

	for key, body := range content {
		putFile(t, s3client, bucket, key, body)
	}

	src := fmt.Sprintf("s3://%v/*", bucket)
	dst := fmt.Sprintf("s3://%v/", dstbucket)

	cmd := s5cmd("sync", src, dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)
	assertLines(t, result.Stderr(), map[int]compareFunc{})

	for key, body := range content {
		assert.Assert(t, ensureS3Object(s3client, dstbucket, key, body), "key %q", key)
	}
	assertS3Keys(t, s3client, dstbucket, content)

	cmd = s5cmd("sync", src, dst)
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{})
	assertLines(t, result.Stderr(), map[int]compareFunc{})
}

// --stat sync --include "*" --exclude "work/*" ... "dir/*" s3://bucket/
//
// The report behind peak/s5cmd#720: a backup of a whole tree with a handful
// of exclude patterns left out directories that no pattern names
// (Omics/ready/INFO.md was never uploaded), while the same command run one
// level down ("dir/Omics/*" s3://bucket/Omics/) uploaded them. Both forms must
// upload the same files, apply the patterns relative to the source prefix,
// and a second run must find nothing left to copy.
func TestSyncLocalTreeToS3BucketWithExcludesUploadsUnnamedDirectories(t *testing.T) {
	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithDir("storageA",
			fs.WithFile("README.md", "S: readme"),
			fs.WithDir("Omics",
				fs.WithDir("ready", fs.WithFile("INFO.md", "S: info")),
				fs.WithDir("raw", fs.WithFile("sample.pileup", "S: pileup")),
				fs.WithDir("screenshots", fs.WithFile("shot.png", "S: shot")),
			),
			// "work/*" is anchored at the source prefix: a nested "work"
			// directory is not excluded.
			fs.WithDir("Genomics",
				fs.WithDir("work", fs.WithFile("notes.txt", "S: notes")),
			),
			fs.WithDir("work", fs.WithFile("job.log", "S: job")),
			fs.WithDir("test", fs.WithFile("case.txt", "S: case")),
			fs.WithDir("Partial", fs.WithFile("part.bin", "S: part")),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	filters := []string{
		"--include", "*",
		"--exclude", "work/*",
		"--exclude", "test/*",
		"--exclude", "Partial/*",
		"--exclude", "*screen*",
		"--exclude", "*.pileup",
	}

	root := filepath.ToSlash(workdir.Join("storageA"))
	dst := fmt.Sprintf("s3://%v/", bucket)

	uploaded := map[string]string{
		"README.md":               "S: readme",
		"Omics/ready/INFO.md":     "S: info",
		"Genomics/work/notes.txt": "S: notes",
	}
	excluded := map[string]string{
		"Omics/raw/sample.pileup":    "S: pileup",
		"Omics/screenshots/shot.png": "S: shot",
		"work/job.log":               "S: job",
		"test/case.txt":              "S: case",
		"Partial/part.bin":           "S: part",
	}

	// 1. the whole tree, as reported.
	args := append([]string{"--stat", "sync"}, filters...)
	args = append(args, root+"/*", dst)
	cmd := s5cmd(args...)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	output, stats := splitStatTable(t, result.Stdout())
	assertLines(t, output, map[int]compareFunc{
		0: equals(`cp %v/Genomics/work/notes.txt %vGenomics/work/notes.txt`, root, dst),
		1: equals(`cp %v/Omics/ready/INFO.md %vOmics/ready/INFO.md`, root, dst),
		2: equals(`cp %v/README.md %vREADME.md`, root, dst),
	}, sortInput(true))
	assert.DeepEqual(t, stats, map[string]string{
		"cp":   "3 0 3",
		"sync": "1 0 1",
	})

	for key, content := range uploaded {
		assert.Assert(t, ensureS3Object(s3client, bucket, key, content))
	}
	for key, content := range excluded {
		err := ensureS3Object(s3client, bucket, key, content)
		assertError(t, err, errS3NoSuchKey)
	}

	// the reporter's check: the directory is listable in the bucket.
	cmd = s5cmd("ls", dst+"Omics/ready/")
	result = icmd.RunCmd(cmd)
	result.Assert(t, icmd.Success)
	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: suffix("INFO.md"),
	})

	// 2. the same command again: everything is in sync, nothing to copy.
	cmd = s5cmd(args...)
	result = icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	output, stats = splitStatTable(t, result.Stdout())
	assert.Equal(t, strings.TrimSpace(output), "")
	assert.DeepEqual(t, stats, map[string]string{
		"sync": "1 0 1",
	})

	// 3. one level down, into a fresh bucket: the same Omics files as 1.
	bucket2 := bucket + "-omics"
	createBucket(t, s3client, bucket2)
	dst2 := fmt.Sprintf("s3://%v/Omics/", bucket2)

	args = append([]string{"--stat", "sync"}, filters...)
	args = append(args, root+"/Omics/*", dst2)
	cmd = s5cmd(args...)
	result = icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	output, stats = splitStatTable(t, result.Stdout())
	assertLines(t, output, map[int]compareFunc{
		0: equals(`cp %v/Omics/ready/INFO.md %vready/INFO.md`, root, dst2),
	})
	assert.DeepEqual(t, stats, map[string]string{
		"cp":   "1 0 1",
		"sync": "1 0 1",
	})

	assert.Assert(t, ensureS3Object(s3client, bucket2, "Omics/ready/INFO.md", "S: info"))
	for _, key := range []string{"Omics/raw/sample.pileup", "Omics/screenshots/shot.png"} {
		err := ensureS3Object(s3client, bucket2, key, excluded[key])
		assertError(t, err, errS3NoSuchKey)
	}

	// the local tree is untouched.
	expected := fs.Expected(t, folderLayout...)
	assert.Assert(t, fs.Equal(workdir.Path(), expected))
}

// sync --exclude "work/*" --exclude "*screen*" "dir/*" s3://bucket/
// (excluded entries cannot be read)
//
// The likely shape of peak/s5cmd#720: the tree holds dangling symlinks
// (pipeline work directories are full of them) and the user excludes them.
// Following such a link fails, and that used to end the whole directory walk:
// every file after the link in the same top-level directory was silently
// left out. On this branch the walk error stops the sync instead. Neither is
// right when the entry is excluded: it is not part of the sync, so it must
// not fail it. Unreadable entries that are not excluded still stop the sync
// (see TestSyncLocalFolderWithDanglingSymlinkToS3BucketWithDelete).
func TestSyncLocalTreeToS3BucketSkipsUnreadableExcludedEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on windows")
	}

	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithDir("storageA",
			// a dangling link matched by the glob itself, excluded by *screen*
			fs.WithSymlink("Ascreen", "does-not-exist"),
			fs.WithDir("Omics",
				// a dangling link inside a walked directory, excluded by *screen*
				fs.WithSymlink("a-screen.png", "does-not-exist"),
				fs.WithDir("ready", fs.WithFile("INFO.md", "S: info")),
			),
			fs.WithDir("work",
				// a directory nobody can read, excluded by work/*
				fs.WithDir("locked", fs.WithFile("secret", "S: secret"), fs.WithMode(0)),
				fs.WithSymlink("stale", "does-not-exist"),
				fs.WithFile("job.log", "S: job"),
			),
			fs.WithFile("zz.txt", "S: zz"),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer func() {
		_ = os.Chmod(workdir.Join("storageA", "work", "locked"), 0o755)
		workdir.Remove()
	}()

	root := filepath.ToSlash(workdir.Join("storageA"))
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", "--exclude", "work/*", "--exclude", "*screen*", root+"/*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Success)

	assertLines(t, result.Stdout(), map[int]compareFunc{
		0: equals(`cp %v/Omics/ready/INFO.md %vOmics/ready/INFO.md`, root, dst),
		1: equals(`cp %v/zz.txt %vzz.txt`, root, dst),
	}, sortInput(true))

	assert.Assert(t, ensureS3Object(s3client, bucket, "Omics/ready/INFO.md", "S: info"))
	assert.Assert(t, ensureS3Object(s3client, bucket, "zz.txt", "S: zz"))

	err := ensureS3Object(s3client, bucket, "work/job.log", "S: job")
	assertError(t, err, errS3NoSuchKey)
}

// sync "dir/*" s3://bucket/ (an entry that is not excluded cannot be read)
//
// The walk must go on past the entry so that the error names it, and the
// sync must still stop: with an incomplete listing no correct plan exists.
func TestSyncLocalTreeToS3BucketReportsUnreadableEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on windows")
	}

	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithDir("Omics",
			fs.WithSymlink("a-link", "does-not-exist"),
			fs.WithDir("ready", fs.WithFile("INFO.md", "S: info")),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer workdir.Remove()

	src := filepath.ToSlash(workdir.Path())
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src+"/*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`ERROR "sync %v/* %v": given object %v/Omics/a-link not found`, src, dst, src),
	})
}

// sync "dir/*" s3://bucket/ (a directory that is not excluded cannot be read)
//
// Same rule as for an unreadable file: the walk goes on, the error names the
// directory, and the sync stops.
func TestSyncLocalTreeToS3BucketReportsUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not enforced on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}

	t.Parallel()

	s3client, s5cmd := setup(t)

	bucket := s3BucketFromTestName(t)
	createBucket(t, s3client, bucket)

	folderLayout := []fs.PathOp{
		fs.WithDir("Omics",
			fs.WithDir("locked", fs.WithFile("secret", "S: secret"), fs.WithMode(0)),
			fs.WithDir("ready", fs.WithFile("INFO.md", "S: info")),
		),
	}

	workdir := fs.NewDir(t, "somedir", folderLayout...)
	defer func() {
		_ = os.Chmod(workdir.Join("Omics", "locked"), 0o755)
		workdir.Remove()
	}()

	src := filepath.ToSlash(workdir.Path())
	dst := fmt.Sprintf("s3://%v/", bucket)

	cmd := s5cmd("sync", src+"/*", dst)
	result := icmd.RunCmd(cmd)

	result.Assert(t, icmd.Expected{ExitCode: 1})

	assertLines(t, result.Stderr(), map[int]compareFunc{
		0: contains(`ERROR "sync %v/* %v": open %v/Omics/locked: permission denied`, src, dst, src),
	})
}
