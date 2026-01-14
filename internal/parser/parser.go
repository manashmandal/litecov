package parser

import (
	"io"

	"github.com/litecov/litecov/internal/coverage"
)

// Parser parses coverage reports into a standard format
type Parser interface {
	Parse(r io.Reader) (*coverage.Report, error)
}
