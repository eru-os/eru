package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
)

const sseMaxLineBytes = 8 * 1024 * 1024

func newJsonRequest(ctx context.Context, method string, url string, headers http.Header, body interface{}) (*http.Request, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")
	return req, nil
}

func streamSSE(ctx context.Context, req *http.Request, onEvent func(data []byte) (bool, error)) error {
	resp, err := utils.ExecuteHttp(ctx, req)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if resp == nil || resp.Body == nil {
		err = errors.New("no response body received from streaming endpoint")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		err = fmt.Errorf("streaming request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	var dataBuf bytes.Buffer

	dispatch := func() (bool, error) {
		if dataBuf.Len() == 0 {
			return false, nil
		}
		payload := bytes.TrimSpace(dataBuf.Bytes())
		dataBuf.Reset()
		if len(payload) == 0 {
			return false, nil
		}
		if string(payload) == "[DONE]" {
			return true, nil
		}
		return onEvent(payload)
	}

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			logs.WithContext(ctx).Error(ctxErr.Error())
			return ctxErr
		}

		line, readErr := readSSELine(reader)
		if len(line) > 0 || readErr == nil {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				stop, dispatchErr := dispatch()
				if dispatchErr != nil {
					return dispatchErr
				}
				if stop {
					return nil
				}
			case strings.HasPrefix(trimmed, ":"):
			case strings.HasPrefix(trimmed, "data:"):
				chunk := strings.TrimPrefix(trimmed, "data:")
				chunk = strings.TrimPrefix(chunk, " ")
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(chunk)
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if _, dispatchErr := dispatch(); dispatchErr != nil {
					return dispatchErr
				}
				return nil
			}
			logs.WithContext(ctx).Error(readErr.Error())
			return readErr
		}
	}
}

func readSSELine(reader *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		chunk, isPrefix, err := reader.ReadLine()
		builder.Write(chunk)
		if builder.Len() > sseMaxLineBytes {
			return builder.String(), errors.New("sse line exceeded maximum size")
		}
		if err != nil {
			return builder.String(), err
		}
		if !isPrefix {
			return builder.String(), nil
		}
	}
}
