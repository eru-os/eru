package validator

import (
	"context"
	"encoding/json"
	"errors"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	DataTypeString  = "string"
	DataTypeBoolean = "boolean"
	DataTypeNumber  = "number"
	DataTypeDate    = "date"
	DataTypeArray   = "array"
)

type FieldI interface {
	GetName() string
	GetDatatype() string
	Validate(ctx context.Context, v interface{}) error
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	ToEncode(ctx context.Context) bool
}

type Field struct {
	Name     string `json:"name" eru:"required"`
	Label    string `json:"label" eru:"required"`
	Required bool   `json:"required" eru:"required"`
	DataType string `json:"data_type" eru:"required"`
	Encode   bool   `json:"encode"`
}

func (f *Field) GetName() string {
	return f.Name
}

func (f *Field) GetDatatype() string {
	return f.DataType
}
func (f *Field) ToEncode(ctx context.Context) bool {
	return f.Encode
}

func GetField(dt string) FieldI {
	switch dt {
	case DataTypeString:
		return new(StringField)
	case DataTypeNumber:
		return new(NumberField)
	case DataTypeBoolean:
		return new(BooleanField)
	case DataTypeDate:
		return new(DateField)
	case DataTypeArray:
		return new(ArrayField)
	default:
		return new(Field)
	}
}

func (f *Field) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson method not implemented")
	return nil
}

func (f *Field) Validate(ctx context.Context, v interface{}) error {
	logs.WithContext(ctx).Debug("Validate method not implemented")
	return errors.New("Validate method not implemented")
}
