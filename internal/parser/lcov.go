package parser

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/litecov/litecov/internal/coverage"
)

// LCOVParser parses LCOV format coverage reports
type LCOVParser struct{}

// Parse reads LCOV format and returns a coverage report
func (p *LCOVParser) Parse(r io.Reader) (*coverage.Report, error) {
	report := &coverage.Report{}
	scanner := bufio.NewScanner(r)

	var current *coverage.FileCoverage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "SF:"):
			// Start of new file
			current = &coverage.FileCoverage{
				Path: strings.TrimPrefix(line, "SF:"),
			}

		case strings.HasPrefix(line, "DA:"):
			// Line data: DA:line_number,hit_count
			if current == nil {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) >= 2 {
				hits, _ := strconv.Atoi(parts[1])
				current.LinesTotal++
				if hits > 0 {
					current.LinesCovered++
				}
			}

		case strings.HasPrefix(line, "LF:"):
			// Lines found (total) - we calculate ourselves, but validate
			if current != nil {
				lf, _ := strconv.Atoi(strings.TrimPrefix(line, "LF:"))
				if lf > 0 && current.LinesTotal != lf {
					current.LinesTotal = lf
				}
			}

		case strings.HasPrefix(line, "LH:"):
			// Lines hit (covered) - we calculate ourselves, but validate
			if current != nil {
				lh, _ := strconv.Atoi(strings.TrimPrefix(line, "LH:"))
				if lh > 0 && current.LinesCovered != lh {
					current.LinesCovered = lh
				}
			}

		case line == "end_of_record":
			// End of file record
			if current != nil {
				report.Files = append(report.Files, *current)
				current = nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	report.Calculate()
	return report, nil
}
