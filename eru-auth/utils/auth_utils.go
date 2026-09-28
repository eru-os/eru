package utils

import (
	"context"
	"encoding/json"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-templates/gotemplate"
)

func ExecuteTemplate(ctx context.Context, templateName string, templateString string, vars interface{}, outputType string) (output []byte, err error) {
	logs.WithContext(ctx).Debug("executeTemplate - Start")
	goTmpl := gotemplate.GoTemplate{Name: templateName, Template: templateString}
	outputObj, err := goTmpl.Execute(ctx, vars, outputType)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	} else if outputType == "string" {
		return []byte(outputObj.(string)), nil
	} else {
		output, err = json.Marshal(outputObj)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
	}
	return
}
