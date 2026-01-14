package parser

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

var ErrUnknownFormat = errors.New("unable to detect coverage format")

func DetectFormat(r io.Reader) (string, error) {
	buf := make([]byte, 1024)
	n, err := bufio.NewReader(r).Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buf[:n])

	if strings.Contains(content, "<?xml") || strings.Contains(content, "<coverage") {
		return "cobertura", nil
	}

	if strings.Contains(content, "SF:") || strings.Contains(content, "end_of_record") {
		return "lcov", nil
	}

	return "", ErrUnknownFormat
}

func GetParser(format string) (Parser, error) {
	switch format {
	case "lcov":
		return &LCOVParser{}, nil
	case "cobertura", "xml":
		return &CoberturaParser{}, nil
	case "auto":
		return nil, nil
	default:
		return nil, errors.New("unknown format: " + format)
	}
}
