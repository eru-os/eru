package eru_writes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	DefaultHtmlColumnWidthPx = 7.5
	DefaultHtmlHeaderCharPx  = 8
	DefaultHtmlCellPaddingPx = 6
	DefaultHtmlPaddingPx     = 8
	DefaultDateFormat        = "02-Jan-2006"
	DefaultDateTimeFormat    = "02-Jan-2006 15:04:05"
	DefaultTimeFormat        = "15:04:05"
)

type HtmlWriteData struct {
	WriteData
	CellFormat map[string]CellFormatter
}

type ImageConfig struct {
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	Scale           float64 `json:"scale,omitempty"`
	Format          string  `json:"format,omitempty"`
	Quality         int     `json:"quality,omitempty"`
	Selector        string  `json:"selector,omitempty"`
	BackgroundColor string  `json:"background_color,omitempty"`
	TimeoutSeconds  int     `json:"timeout_seconds,omitempty"`
}

type htmlColumn struct {
	Label    string
	DataType string
	Css      string
	WidthPx  int
}

func (hwd *HtmlWriteData) WriteColumnar(ctx context.Context) (writeOutput []byte, err error) {
	logs.WithContext(ctx).Debug("WriteColumnar - Start")

	if hwd.ColumnarDataMap == nil {
		return nil, errors.New("html data not found")
	}

	sheetNames := make([]string, 0, len(hwd.ColumnarDataMap))
	for k := range hwd.ColumnarDataMap {
		sheetNames = append(sheetNames, k)
	}
	sort.Strings(sheetNames)

	var doc strings.Builder
	doc.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>`)
	doc.WriteString(html.EscapeString(hwd.FileName))
	doc.WriteString(`</title><style>`)
	doc.WriteString(htmlBaseCss())
	doc.WriteString(`</style></head><body><div class="eru-doc">`)

	for _, sheetName := range sheetNames {
		v := hwd.ColumnarDataMap[sheetName]
		if len(v) == 0 {
			continue
		}

		sheetSettings := ColumnarSettings{
			HeaderFirstRow: true,
			Headers:        make(map[string]ColumnHeaders),
		}
		if hwd.ColumnarSettings != nil {
			if settings, exists := hwd.ColumnarSettings[sheetName]; exists {
				sheetSettings = settings
			}
		}

		if hwd.CellFormat == nil {
			hwd.CellFormat = make(map[string]CellFormatter)
		}
		if cellFormatter, cellFormatterOk := hwd.CellFormat[sheetName]; cellFormatterOk {
			hwd.CellFormat[sheetName] = *mergeCellFormatters(&cellFormatter, DefaultCellFormatter())
		} else {
			hwd.CellFormat[sheetName] = *DefaultCellFormatter()
		}

		var excludedCols map[int]bool
		var headerRow []interface{}
		if sheetSettings.HeaderFirstRow {
			excludedCols = ExcludedColumnIndices(v[0], hwd.ExcludeColumns)
			headerRow = v[0]
		}

		cols := make([]htmlColumn, len(v[0]))
		for cNo := range v[0] {
			colKey := fmt.Sprint(cNo)
			if headerRow != nil {
				colKey = fmt.Sprint(headerRow[cNo])
			}
			colHeader := sheetSettings.Headers[colKey]
			label := colHeader.HeaderLabel
			if label == "" {
				label = colKey
			}
			maxWidth := colHeader.MaxWidth
			if maxWidth == 0 {
				maxWidth = DefaultMaxColumnWidth
			}
			finalWidth := calculateOptimalColumnWidth(v, cNo)
			if finalWidth > maxWidth {
				finalWidth = maxWidth
			}
			widthPx := columnWidthToPx(finalWidth)
			headerPx := len(label)*DefaultHtmlHeaderCharPx + 2*DefaultHtmlCellPaddingPx + 2
			if maxWidthPx := columnWidthToPx(maxWidth); headerPx > maxWidthPx {
				headerPx = maxWidthPx
			}
			if headerPx > widthPx {
				widthPx = headerPx
			}
			dataStyle := mergeCellStyles(hwd.CellFormat[sheetName].DataStyle, CellStyle{
				Alignment: &AlignmentStyle{
					Horizontal: getDefaultHorizontalAlignment(colHeader.DataType),
				},
			})
			cols[cNo] = htmlColumn{
				Label:    label,
				DataType: colHeader.DataType,
				Css:      cellStyleToCss(dataStyle),
				WidthPx:  widthPx,
			}
		}

		headerStyle := mergeCellStyles(hwd.CellFormat[sheetName].HeaderStyle, CellStyle{
			Alignment: &AlignmentStyle{
				Horizontal: "center",
			},
		})
		headerCss := cellStyleToCss(headerStyle)

		doc.WriteString(`<table class="eru-table">`)
		if len(sheetNames) > 1 {
			doc.WriteString(`<caption>`)
			doc.WriteString(html.EscapeString(sheetName))
			doc.WriteString(`</caption>`)
		}

		doc.WriteString(`<colgroup>`)
		for cNo := range cols {
			if excludedCols[cNo] {
				continue
			}
			doc.WriteString(fmt.Sprintf(`<col style="width:%dpx">`, cols[cNo].WidthPx))
		}
		doc.WriteString(`</colgroup>`)

		tbodyOpen := false
		for rNo, row := range v {
			isHeader := sheetSettings.HeaderFirstRow && rNo == 0
			if isHeader {
				doc.WriteString(`<thead><tr>`)
			} else if !tbodyOpen {
				tbodyOpen = true
				doc.WriteString(`<tbody><tr>`)
			} else {
				doc.WriteString(`<tr>`)
			}
			for cNo, colV := range row {
				if excludedCols[cNo] {
					continue
				}
				if isHeader {
					doc.WriteString(fmt.Sprintf(`<th style="%s">%s</th>`, headerCss, html.EscapeString(cols[cNo].Label)))
					continue
				}
				dt := cols[cNo].DataType
				doc.WriteString(fmt.Sprintf(`<td style="%s">%s</td>`, cols[cNo].Css, html.EscapeString(formatHtmlCellValue(colV, dt))))
			}
			if isHeader {
				doc.WriteString(`</tr></thead>`)
			} else {
				doc.WriteString(`</tr>`)
			}
		}
		if tbodyOpen {
			doc.WriteString(`</tbody>`)
		}
		doc.WriteString(`</table>`)
	}

	doc.WriteString(`</div></body></html>`)
	return []byte(doc.String()), nil
}

func columnWidthToPx(width float64) int {
	return int(math.Round(width*DefaultHtmlColumnWidthPx)) + 2*DefaultHtmlCellPaddingPx + 2
}

func htmlBaseCss() string {
	return fmt.Sprint(`html{background:#ffffff}`,
		`body{margin:0;padding:`, DefaultHtmlPaddingPx, `px;display:inline-block;background:#ffffff;`,
		`font-family:Arial,Helvetica,sans-serif;-webkit-font-smoothing:antialiased}`,
		`.eru-doc{display:inline-block}`,
		`.eru-table{border-collapse:collapse;table-layout:fixed;margin-bottom:`, DefaultHtmlPaddingPx*2, `px}`,
		`.eru-table:last-child{margin-bottom:0}`,
		`.eru-table caption{text-align:left;font-weight:700;padding-bottom:4px}`,
		`.eru-table th,.eru-table td{border:1px solid #d0d0d0;padding:3px `, DefaultHtmlCellPaddingPx, `px;`,
		`overflow-wrap:break-word;word-break:break-word}`)
}

func formatHtmlCellValue(value interface{}, dataType string) string {
	if value == nil {
		return ""
	}
	dt := dataType
	if dt == "" || dt == DataTypeString {
		dt = reflect.TypeOf(value).String()
	}
	switch dt {
	case DataTypeInteger, DataTypeBigInteger, DataTypeSmallInteger:
		if intVal, ok := safeInt(value); ok {
			return strconv.Itoa(intVal)
		}
	case DataTypeFloat, DataTypeDecimal:
		if floatVal, ok := safeFloat(value); ok {
			return strconv.FormatFloat(floatVal, 'f', DefaultFloatPrecision, DefaultFloatByteSize)
		}
	case DataTypeBoolean:
		if boolVal, ok := safeBool(value); ok {
			return strconv.FormatBool(boolVal)
		}
	case DataTypeDate:
		if dateVal, ok := safeDate(value); ok {
			return dateVal.Format(DefaultDateFormat)
		}
	case DataTypeTime, DataTypeTimeWithZone:
		if dateVal, ok := safeDate(value); ok {
			return dateVal.Format(DefaultTimeFormat)
		}
	case DataTypeDateTime, DataTypeDateTimeWithZone:
		if dateVal, ok := safeDate(value); ok {
			return dateVal.Format(DefaultDateTimeFormat)
		}
	default:
		if dateVal, ok := value.(time.Time); ok {
			return dateVal.Format(DefaultDateTimeFormat)
		}
	}
	return safeString(value)
}

func cellStyleToCss(style CellStyle) string {
	var css []string
	if style.Font != nil {
		if style.Font.Bold {
			css = append(css, "font-weight:700")
		} else {
			css = append(css, "font-weight:400")
		}
		if style.Font.Italic {
			css = append(css, "font-style:italic")
		}
		if style.Font.Family != "" {
			css = append(css, fmt.Sprintf("font-family:%s", style.Font.Family))
		}
		if style.Font.Size > 0 {
			css = append(css, fmt.Sprintf("font-size:%dpt", style.Font.Size))
		}
		if style.Font.Color != "" {
			css = append(css, fmt.Sprintf("color:%s", normalizeHtmlColor(style.Font.Color)))
		}
	}

	if style.Fill != nil && len(style.Fill.Color) > 0 {
		if style.Fill.Type == "gradient" && len(style.Fill.Color) > 1 {
			css = append(css, fmt.Sprintf("background-image:linear-gradient(%s,%s)",
				normalizeHtmlColor(style.Fill.Color[0]), normalizeHtmlColor(style.Fill.Color[1])))
		} else {
			css = append(css, fmt.Sprintf("background-color:%s", normalizeHtmlColor(style.Fill.Color[0])))
		}
	}

	for _, border := range style.Border {
		if border == nil {
			continue
		}
		width, borderStyle := borderStyleToCss(border.Style)
		if borderStyle == "none" {
			continue
		}
		color := "#000000"
		if border.Color != "" {
			color = normalizeHtmlColor(border.Color)
		}
		value := fmt.Sprintf("%s %s %s", width, borderStyle, color)
		if border.Type == "" {
			css = append(css, fmt.Sprintf("border:%s", value))
		} else {
			switch border.Type {
			case "left", "right", "top", "bottom":
				css = append(css, fmt.Sprintf("border-%s:%s", border.Type, value))
			default:
				css = append(css, fmt.Sprintf("border:%s", value))
			}
		}
	}

	if style.Alignment != nil {
		if style.Alignment.Horizontal != "" {
			css = append(css, fmt.Sprintf("text-align:%s", horizontalAlignmentToCss(style.Alignment.Horizontal)))
		}
		if style.Alignment.Vertical != "" {
			css = append(css, fmt.Sprintf("vertical-align:%s", verticalAlignmentToCss(style.Alignment.Vertical)))
		}
	}

	return strings.Join(css, ";")
}

func horizontalAlignmentToCss(alignment string) string {
	switch alignment {
	case "centerContinuous", "distributed":
		return "center"
	case "fill", "justify":
		return "justify"
	default:
		return alignment
	}
}

func verticalAlignmentToCss(alignment string) string {
	switch alignment {
	case "center":
		return "middle"
	case "justify", "distributed":
		return "middle"
	default:
		return alignment
	}
}

func borderStyleToCss(style int) (width string, borderStyle string) {
	switch style {
	case 1, 7:
		return "1px", "solid"
	case 2:
		return "2px", "solid"
	case 3, 9:
		return "1px", "dashed"
	case 4, 11:
		return "1px", "dotted"
	case 5:
		return "3px", "solid"
	case 6:
		return "3px", "double"
	case 8, 10, 13:
		return "2px", "dashed"
	case 12:
		return "2px", "dotted"
	default:
		return "0", "none"
	}
}

func normalizeHtmlColor(color string) string {
	c := strings.TrimSpace(color)
	if c == "" {
		return c
	}
	if strings.HasPrefix(c, "#") {
		return c
	}
	if len(c) == 6 || len(c) == 8 {
		if _, err := strconv.ParseUint(c, 16, 64); err == nil {
			if len(c) == 8 {
				c = c[2:]
			}
			return fmt.Sprint("#", c)
		}
	}
	return c
}
