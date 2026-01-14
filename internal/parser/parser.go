package parser

import (
	"io"

	"github.com/manashmandal/litecov/internal/coverage"
)

type Parser interface {
	Parse(r io.Reader) (*coverage.Report, error)
}
