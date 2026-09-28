package md5

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func Md5(ctx context.Context, str string, output string) (string, error) {
	h := md5.Sum([]byte(str))
	if output == "hex" {
		return hex.EncodeToString(h[:]), nil
	} else if output == "string" {
		return string(h[:]), nil
	}
	err := errors.New(fmt.Sprint("error - unknown output", output))
	logs.WithContext(ctx).Error(err.Error())
	return "", err
}
