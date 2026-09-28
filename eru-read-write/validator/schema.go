package validator

import (
	"context"
	"encoding/json"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/tidwall/gjson"
)

type Schema struct {
	Fields []FieldI `json:"fields" eru:"required"`
}

func (s *Schema) SetFields(ctx context.Context, fieldsMapArray []*json.RawMessage) (err error) {
	for _, v := range fieldsMapArray {
		var fieldObj map[string]*json.RawMessage
		err = json.Unmarshal(*v, &fieldObj)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		var dataType string
		err = json.Unmarshal(*fieldObj["data_type"], &dataType)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		fieldI := GetField(dataType)
		err = fieldI.MakeFromJson(ctx, v)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		s.Fields = append(s.Fields, fieldI)
	}
	return
}

func (s *Schema) GetField(ctx context.Context, fieldName string) (filedI FieldI) {
	for _, f := range s.Fields {
		if f.GetName() == fieldName {
			return f
		}
	}
	return
}

func (s *Schema) Validate(ctx context.Context, json gjson.Result) (records []interface{}, errRecords []interface{}) {
	logs.WithContext(ctx).Debug("Validate - Start")
	for _, d := range json.Array() {
		dMap := make(map[string]interface{})
		var errStr []string
		for _, field := range s.Fields {
			fieldName := field.GetName()
			fieldValue := d.Get(fieldName).Value()
			err := field.Validate(ctx, fieldValue)
			if err != nil {
				errStr = append(errStr, err.Error())
			}
		}
		dMap = d.Value().(map[string]interface{})
		if len(errStr) > 0 {
			dMap["error"] = strings.Join(errStr, " , ")
			errRecords = append(errRecords, dMap)
		} else {
			records = append(records, dMap)
		}
	}
	return
}
