package parser

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// DetectFormat reads the first few lines to determine coverage format
func DetectFormat(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)

	// Read first 1KB to detect format
	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buf[:n])

	// Check for XML
	if strings.Contains(content, "<?xml") || strings.Contains(content, "<coverage") {
		return "cobertura", nil
	}

	// Check for LCOV markers
	if strings.Contains(content, "SF:") || strings.Contains(content, "end_of_record") {
		return "lcov", nil
	}

	return "", fmt.Errorf("unable to detect coverage format")
}

// GetParser returns the appropriate parser for the given format
func GetParser(format string) (Parser, error) {
	switch format {
	case "lcov":
		return &LCOVParser{}, nil
	case "cobertura", "xml":
		return &CoberturaParser{}, nil
	case "auto":
		// For auto, caller should use DetectFormat first
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}
