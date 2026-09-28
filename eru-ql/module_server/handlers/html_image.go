package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/ql"
	eru_writes "github.com/eru-os/eru/eru-read-write/eru_writes"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	ImageFormatPng      = "png"
	ImageFormatJpeg     = "jpeg"
	HtmlImageConvertUri = "/html-image/convert"
)

type htmlImageRequest struct {
	Html string `json:"html"`
	eru_writes.ImageConfig
}

func buildColumnarWriteData(ctx context.Context, res []map[string]interface{}, qobjs []ql.QueryObject, columns map[string]eru_writes.ColumnarSettings, excludeColumns []string, fileName string) (wd eru_writes.WriteData, err error) {
	logs.WithContext(ctx).Debug("buildColumnarWriteData - Start")
	wd = eru_writes.WriteData{
		ColumnarSettings: columns,
		ExcludeColumns:   excludeColumns,
		FileName:         fileName,
	}
	if wd.ColumnarSettings == nil {
		wd.ColumnarSettings = make(map[string]eru_writes.ColumnarSettings)
	}
	for vi, v := range res {
		for k, columnarData := range v {
			headers := make(map[string]eru_writes.ColumnHeaders)
			if _, exists := wd.ColumnarSettings[k]; exists {
				headers = wd.ColumnarSettings[k].Headers
			}
			for _, dt := range qobjs[vi].DataTypes {
				mw := eru_writes.DefaultMaxColumnWidth
				st := true
				hl := dt.ColName
				nf := ""
				if _, exists := headers[dt.ColName]; exists {
					mw = headers[dt.ColName].MaxWidth
					st = headers[dt.ColName].SubTotal
					nf = headers[dt.ColName].NumberFormat
					hl = headers[dt.ColName].HeaderLabel
					if hl == "" {
						hl = dt.ColName
					}
				}
				headers[dt.ColName] = eru_writes.ColumnHeaders{
					HeaderName:   dt.ColName,
					HeaderLabel:  hl,
					DataType:     dt.ColDatabaseTypeName,
					MaxWidth:     mw,
					SubTotal:     st,
					NumberFormat: nf,
				}
			}
			for hk := range headers {
				hkFound := false
				for _, dt := range qobjs[vi].DataTypes {
					if dt.ColName == hk {
						hkFound = true
						break
					}
				}
				if !hkFound {
					delete(headers, hk)
				}
			}

			wd.ColumnarSettings[k] = eru_writes.ColumnarSettings{
				HeaderFirstRow: true,
				Headers:        headers,
			}

			records, recordsOk := columnarData.([][]interface{})
			if !recordsOk {
				err = errors.New("incorrect columnar data format")
				logs.WithContext(ctx).Error(err.Error())
				return wd, err
			}
			if len(records) > 0 && len(records[0]) > 0 {
				if wd.ColumnarDataMap == nil {
					wd.ColumnarDataMap = make(map[string][][]interface{})
				}
				wd.ColumnarDataMap[k] = records
			}
		}
	}
	return wd, nil
}

func ImageContentType(format string) string {
	if strings.ToLower(format) == ImageFormatJpeg {
		return "image/jpeg"
	}
	return "image/png"
}

func HtmlToImage(ctx context.Context, htmlOutput []byte, imageConfig eru_writes.ImageConfig) (imageOutput []byte, err error) {
	logs.WithContext(ctx).Debug("HtmlToImage - Start")
	baseUrl := strings.TrimSuffix(os.Getenv("ERUHTMLIMAGE_BASEURL"), "/")
	if baseUrl == "" {
		err = errors.New("ERUHTMLIMAGE_BASEURL not set - image output not supported")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	reqBody, err := json.Marshal(htmlImageRequest{Html: string(htmlOutput), ImageConfig: imageConfig})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprint(baseUrl, HtmlImageConvertUri), bytes.NewReader(reqBody))
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := utils.ExecuteHttp(ctx, req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if resp.StatusCode >= 400 {
		err = errors.New(fmt.Sprint("error from eru-html-image : ", string(respBody)))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return respBody, nil
}
