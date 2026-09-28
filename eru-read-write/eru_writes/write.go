package eru_writes

import (
	"context"
	"errors"
	"fmt"
)

const (
	OutputTypeExcel = "excel"
	OutputTypeCsv   = "csv"
	OutputTypeHtml  = "html"
	OutputTypeImage = "image"
)

func IsColumnarOutput(outputType string) bool {
	switch outputType {
	case OutputTypeCsv, OutputTypeExcel, OutputTypeHtml, OutputTypeImage:
		return true
	default:
		return false
	}
}

type WriteData struct {
	ColumnarDataMap map[string][][]interface{}
	//ColumnarData               [][]interface{}
	//ColumnarDataHeader         []string
	//ColumnarDataHeaderFirstRow bool
	ColumnarSettings map[string]ColumnarSettings
	ExcludeColumns   []string
	FileName         string
}

type ColumnarSettings struct {
	HeaderFirstRow bool
	Headers        map[string]ColumnHeaders
}

type ColumnHeaders struct {
	HeaderName   string  `json:"header_name"`
	HeaderLabel  string  `json:"header_label"`
	DataType     string  `json:"data_type"`
	MaxWidth     float64 `json:"max_width"`
	SubTotal     bool    `json:"sub_total"`
	NumberFormat string  `json:"number_format,omitempty"`
}

type WriteI interface {
	WriteColumnar(ctx context.Context) (writeOutput []byte, err error)
	MapToCsv(ctx context.Context, mapObjs []map[string]interface{}, firstRowHeader bool) (writeOutput []byte, err error)
}

func (wd *WriteData) WriteColumnar() (writeOutput []byte, err error) {
	return nil, errors.New("WriteColumnar method not supported")
}

func extractHeaders(records []interface{}) (headers []string, validRecords []map[string]interface{}) {
	headerSet := make(map[string]struct{})
	for _, record := range records {
		if rec, recOk := record.(map[string]interface{}); recOk {
			validRecords = append(validRecords, rec)
			for key := range rec {
				headerSet[key] = struct{}{}
			}
		}
	}
	// Convert the set to a slice
	for key := range headerSet {
		headers = append(headers, key)
	}
	return
}

// ExcludedColumnIndices returns the set of column indices that should be skipped,
// matching excludeColumns against the header row. It returns nil when nothing is
// excluded so callers can take the unmodified fast path without an extra pass.
func ExcludedColumnIndices(headerRow []interface{}, excludeColumns []string) map[int]bool {
	if len(excludeColumns) == 0 || len(headerRow) == 0 {
		return nil
	}
	excludeSet := make(map[string]struct{}, len(excludeColumns))
	for _, c := range excludeColumns {
		excludeSet[c] = struct{}{}
	}
	excluded := make(map[int]bool)
	for i, h := range headerRow {
		if _, ok := excludeSet[fmt.Sprint(h)]; ok {
			excluded[i] = true
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	return excluded
}

// buildRow constructs a row for the given record based on the headers
func buildRow(record map[string]interface{}, headers []string) []string {
	row := make([]string, len(headers))
	for i, header := range headers {
		if value, exists := record[header]; exists {
			row[i] = fmt.Sprintf("%v", value) // Convert any type to a string
		} else {
			row[i] = "" // Leave blank if key is not present
		}
	}
	return row
}
