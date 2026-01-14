package parser

import (
	"encoding/xml"
	"io"

	"github.com/manashmandal/litecov/internal/coverage"
)

type CoberturaParser struct{}

type coberturaXML struct {
	XMLName  xml.Name           `xml:"coverage"`
	Packages []coberturaPackage `xml:"packages>package"`
}

type coberturaPackage struct {
	Name    string           `xml:"name,attr"`
	Classes []coberturaClass `xml:"classes>class"`
}

type coberturaClass struct {
	Name     string          `xml:"name,attr"`
	Filename string          `xml:"filename,attr"`
	Lines    []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number int `xml:"number,attr"`
	Hits   int `xml:"hits,attr"`
}

func (p *CoberturaParser) Parse(r io.Reader) (*coverage.Report, error) {
	var cov coberturaXML
	if err := xml.NewDecoder(r).Decode(&cov); err != nil {
		return nil, err
	}

	fileMap := make(map[string]*coverage.FileCoverage)
	linesSeen := make(map[string]map[int]bool)

	for _, pkg := range cov.Packages {
		for _, class := range pkg.Classes {
			fc, exists := fileMap[class.Filename]
			if !exists {
				fc = &coverage.FileCoverage{Path: class.Filename}
				fileMap[class.Filename] = fc
				linesSeen[class.Filename] = make(map[int]bool)
			}
			for _, line := range class.Lines {
				if linesSeen[class.Filename][line.Number] {
					continue
				}
				linesSeen[class.Filename][line.Number] = true
				fc.LinesTotal++
				if line.Hits > 0 {
					fc.LinesCovered++
				} else {
					fc.UncoveredLines = append(fc.UncoveredLines, line.Number)
				}
			}
		}
	}

	report := &coverage.Report{}
	for _, fc := range fileMap {
		report.Files = append(report.Files, *fc)
	}

	report.Calculate()
	return report, nil
}
