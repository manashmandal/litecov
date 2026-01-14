package parser

import (
	"encoding/xml"
	"io"

	"github.com/litecov/litecov/internal/coverage"
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

	report := &coverage.Report{}

	for _, pkg := range cov.Packages {
		for _, class := range pkg.Classes {
			fc := coverage.FileCoverage{
				Path:       class.Filename,
				LinesTotal: len(class.Lines),
			}
			for _, line := range class.Lines {
				if line.Hits > 0 {
					fc.LinesCovered++
				}
			}
			report.Files = append(report.Files, fc)
		}
	}

	report.Calculate()
	return report, nil
}
