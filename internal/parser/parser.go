package parser

import (
	"io"

	"github.com/litecov/litecov/internal/coverage"
)

type Parser interface {
	Parse(r io.Reader) (*coverage.Report, error)
}
