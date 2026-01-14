package parser

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/manashmandal/litecov/internal/coverage"
)

type LCOVParser struct{}

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
			current = &coverage.FileCoverage{
				Path: strings.TrimPrefix(line, "SF:"),
			}

		case strings.HasPrefix(line, "DA:"):
			if current == nil {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) >= 2 {
				lineNum, _ := strconv.Atoi(parts[0])
				hits, _ := strconv.Atoi(parts[1])
				current.LinesTotal++
				if hits > 0 {
					current.LinesCovered++
				} else {
					current.UncoveredLines = append(current.UncoveredLines, lineNum)
				}
			}

		case strings.HasPrefix(line, "LF:"):
			if current != nil {
				lf, _ := strconv.Atoi(strings.TrimPrefix(line, "LF:"))
				if lf > 0 && current.LinesTotal != lf {
					current.LinesTotal = lf
				}
			}

		case strings.HasPrefix(line, "LH:"):
			if current != nil {
				lh, _ := strconv.Atoi(strings.TrimPrefix(line, "LH:"))
				if lh > 0 && current.LinesCovered != lh {
					current.LinesCovered = lh
				}
			}

		case line == "end_of_record":
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
