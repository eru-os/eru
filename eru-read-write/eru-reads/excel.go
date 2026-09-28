package eru_reads

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-read-write/validator"
	"github.com/xuri/excelize/v2"
	"strconv"
	"strings"
	"time"
)

var dateFormats = []string{
	"01/02/2006", // Example: 12/31/1999
	"02-01-2006", // Example: 31-12-1999
	//"01-02-2006",      // Example: 12-31-1999
	"02.01.2006",      // Example: 31.12.1999
	"2006-01-02",      // Example: 1999-12-31
	"January 2, 2006", // Example: December 31, 1999
	"2 January 2006",  // Example: 31 December 1999
	"02-Jan-2006",     // Example: 31-Dec-1999
	"02-Jan-06",       // Example: 31-Dec-1999
	"2-Jan-06",
	"2006-01-02T15:04:05-07:00",
	"02-01-2006 15:04:05", //Example 31-12-1999  12:31:55
	"02-01-2006 15:04",
	"2006-01-02 15:04",
}

type ExcelReadData struct {
	ReadData
	Sheets map[string]FileReadData `json:"sheets"`
}

func (erd *ExcelReadData) ReadAsJson(ctx context.Context, readData []byte) (readOutput map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("WriteColumnar - Start")
	f, err := excelize.OpenReader(bytes.NewReader(readData), excelize.Options{
		RawCellValue: true,
	}, excelize.Options{
		LongDatePattern: "yyyy-mm-dd",
	}, excelize.Options{
		ShortDatePattern: "yyyy-mm-dd",
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
	}()

	if erd.Sheets == nil {
		erd.Sheets = make(map[string]FileReadData)
		for _, sn := range f.GetSheetList() {
			erd.Sheets[sn] = FileReadData{}
		}
	}
	schema := validator.Schema{}
	for sheetName, sheetObj := range erd.Sheets {
		if sheetName == "*" {
			for _, sn := range f.GetSheetList() {
				erd.Sheets[sn] = sheetObj
			}
			delete(erd.Sheets, "*")
			break
		}
	}
	for sheetName, sheetObj := range erd.Sheets {

		err = schema.SetFields(ctx, sheetObj.Fields)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}

		var sheetData []map[string]interface{}
		rows, rowsErr := f.GetRows(sheetName)
		if rowsErr != nil {
			err = rowsErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		_ = rows
		_ = sheetObj

		var cols []int
		var colHeaders []string

		if sheetObj.ColumnHeaders == nil {
			for _, cellHeader := range rows[sheetObj.HeaderRow] {
				colHeaders = append(colHeaders, cellHeader)
			}
		} else if len(sheetObj.ColumnHeaders) == 0 {
			for _, cellHeader := range rows[sheetObj.HeaderRow] {
				colHeaders = append(colHeaders, cellHeader)
			}
		} else {
			for chNo, ch := range sheetObj.ColumnHeaders {
				if ch == "" {
					if len(rows[sheetObj.HeaderRow]) > chNo {
						colHeaders = append(colHeaders, rows[sheetObj.HeaderRow][chNo])
					} else {
						colHeaders = append(colHeaders, "")
					}
				} else {
					colHeaders = append(colHeaders, ch)
				}
			}
		}

		if sheetObj.Columns == nil {
			for colNo, _ := range rows[0] {
				cols = append(cols, colNo+1)
			}
		} else if len(sheetObj.Columns) == 0 {
			for colNo, _ := range rows[0] {
				cols = append(cols, colNo+1)
			}
		} else {
			cols = sheetObj.Columns
		}
		//type colValue interface{}

		for rowNo, row := range rows {
			var errs []string
			if rowNo+1 >= sheetObj.DataStartRow {
				sheetRow := make(map[string]interface{})
				for _, colNo := range cols {

					isNum := false
					var rowValue interface{}
					var rowValueF float64
					field := schema.GetField(ctx, colHeaders[colNo-1])

					if len(row) > colNo-1 {
						if field.GetDatatype() == "date" {
							formatMatched := false
							for _, format := range dateFormats {
								if dateValue, err := time.Parse(format, row[colNo-1]); err == nil {
									rowValue = dateValue.Format("2006-01-02")
									formatMatched = true
									break
								} //else {
								//logs.WithContext(ctx).Error(err.Error())
								//}
							}
							if !formatMatched {
								rowValue = strings.TrimSpace(row[colNo-1])
							}
						} else if field.GetDatatype() == "string" {
							rowValue = row[colNo-1]
						} else {
							rowValueF, err = strconv.ParseFloat(row[colNo-1], 64)
							if err != nil {
								rowValue, err = strconv.ParseBool(row[colNo-1])
								if err != nil {
									rowValue = row[colNo-1]
								}
							} else {
								rowValue = rowValueF
								isNum = true
							}
						}
					} else {
						rowValue = nil
					}
					sheetRow[colHeaders[colNo-1]] = rowValue

					if field != nil {
						if field.GetDatatype() == "date" && isNum {
							var vTime time.Time
							vTime, err = excelize.ExcelDateToTime(rowValueF, false)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								errs = append(errs, err.Error())
							}
							sheetRow[colHeaders[colNo-1]] = vTime.Format("2006-01-02")
							rowValue = vTime.Format("2006-01-02")
						} else if field.GetDatatype() == "number" && !isNum && rowValue != nil {
							if strings.TrimSpace(rowValue.(string)) == "" {
								rowValue = nil
								sheetRow[colHeaders[colNo-1]] = rowValue
							}
						} else if field.GetDatatype() == "array" {
							var rowValueArray []interface{}
							if rowValue == nil {
								rowValueArray = make([]interface{}, 0)
							} else if arrayStr, arrayStrOk := rowValue.(string); arrayStrOk {
								if arrayStr == "" {
									rowValueArray = make([]interface{}, 0)
								} else {
									arrayVal := strings.Split(arrayStr, ",")
									ary := make([]float64, len(arrayVal))
									for i, sa := range arrayVal {
										ary[i], err = strconv.ParseFloat(sa, 64)
										if err != nil {
											rowValueArray = append(rowValueArray, sa)
										} else {
											rowValueArray = append(rowValueArray, ary[i])
										}
									}
								}
							} else if isNum {
								rowValueArray = append(rowValueArray, rowValue)
							}
							sheetRow[colHeaders[colNo-1]] = rowValueArray
							rowValue = rowValueArray
						}
						vErr := field.Validate(ctx, rowValue)
						if vErr != nil {
							logs.WithContext(ctx).Error(vErr.Error())
							errs = append(errs, vErr.Error())
						}
						if field.ToEncode(ctx) {
							if rowValue != nil {
								rowBytes := []byte("")
								rowBytes, err = json.Marshal(rowValue)
								if err != nil {
									errs = append(errs, err.Error())
								}
								rowStr := ""
								rowStr, err = strconv.Unquote(string(rowBytes))
								if err != nil {
									errs = append(errs, err.Error())
								}
								sheetRow[colHeaders[colNo-1]] = b64.StdEncoding.EncodeToString([]byte(rowStr))
							} else {
								sheetRow[colHeaders[colNo-1]] = ""
							}
						} else if field.GetDatatype() == "string" && isNum {
							//bigint := big.NewFloat(rowValueF)
							//rowValue = bigint.String()
							rowValue = fmt.Sprintf("%.f", rowValueF)
							sheetRow[colHeaders[colNo-1]] = rowValue
						}
					}
				}
				if len(errs) > 0 {
					sheetRow["error"] = strings.Join(errs, " , ")
				}
				sheetData = append(sheetData, sheetRow)
			}
		}
		if readOutput == nil {
			readOutput = make(map[string]interface{})
		}
		readOutput[sheetName] = sheetData
	}
	return readOutput, nil
}

// Function to convert column number to column name
func columnNumberToName(n int) string {
	columnName := ""
	for n > 0 {
		n--
		columnName = string(rune('A'+(n%26))) + columnName
		n /= 26
	}
	return columnName
}
