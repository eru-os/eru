package eru_writes

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type CsvWriteData struct {
	WriteData
}

func (cwd *WriteData) MapToCsv(ctx context.Context, mapObjs []interface{}, firstRowHeader bool) (writeOutput []byte, err error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	// Extract and write headers (unique keys from all records)
	headers, validRecords := extractHeaders(mapObjs)
	if err = writer.Write(headers); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("Error writing CSV headers:", err))
		return
	}

	// Write CSV rows
	for _, record := range validRecords {
		row := buildRow(record, headers)
		if err = writer.Write(row); err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("Error writing CSV row:", err))
			return
		}
	}
	// Flush the writer
	writer.Flush()
	if err = writer.Error(); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("Error flushing data to buffer:", err.Error()))
		fmt.Println()
		return
	}
	return buffer.Bytes(), nil
}
