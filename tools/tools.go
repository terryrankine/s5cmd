//go:build tools

// Package tools pins the developer tooling (linters, code generators) used by
// the Makefile. It is a separate Go module so those tools and their Go
// version requirement never leak into the main module or its vendor tree.
package tools

import (
	_ "go.uber.org/mock/mockgen"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "mvdan.cc/unparam"
)
