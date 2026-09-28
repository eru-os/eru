package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type BooleanField struct {
	Field
	CheckValue bool `json:"check_value"`
	Value      bool `json:"value"`
}

func (f *BooleanField) Validate(ctx context.Context, v interface{}) (err error) {
	var errs []string
	defer func() {
		if len(errs) > 0 {
			err = errors.New(strings.Join(errs, " ; "))
			logs.WithContext(ctx).Error(fmt.Sprint(errs))
		} else {
			err = nil
		}
	}()

	if v == nil {
		if f.Required {
			errs = append(errs, fmt.Sprint("'", f.Label, "' cannot be blank"))
		}
		return
	}

	value, ok := v.(bool)
	if !ok {
		valueStr := fmt.Sprint(v)
		if valueStr == "Y" || valueStr == "1" {
			value = true
		} else if valueStr == "N" || valueStr == "0" {
			value = false
		} else {
			errs = append(errs, fmt.Sprint("'", f.Label, "' has to be a boolean"))
			return
		}
	}

	if f.CheckValue && f.Value != value {
		errs = append(errs, fmt.Sprint("invalid value for field '", f.Label, "'"))
	}

	return
}

func (f *BooleanField) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &f)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
