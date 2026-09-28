package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type NumberField struct {
	Field
	MinValue     float64 `json:"min_value"`
	MinCheck     bool    `json:"min_check"`
	MaxValue     float64 `json:"max_value"`
	MaxCheck     bool    `json:"max_check"`
	ValidateSign bool    `json:"validate_sign"`
	IsPositive   bool    `json:"is_positive"`
}

func (f *NumberField) Validate(ctx context.Context, v interface{}) (err error) {
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

	value, ok := v.(float64)
	if !ok {
		valueInt, valueIntok := v.(int)
		if !valueIntok {
			errs = append(errs, fmt.Sprint("'", f.Label, "' has to be a number"))
			return
		}
		value = float64(valueInt)
	}

	if f.MinCheck && f.MinValue > value {
		errs = append(errs, fmt.Sprint("minimum length of '", f.Label, "' has to be ", f.MinValue))
	}
	if f.MaxCheck && f.MaxValue < value {
		errs = append(errs, fmt.Sprint("maximum length of '", f.Label, "' has to be ", f.MaxValue))
	}

	if f.ValidateSign {
		if f.IsPositive && value < 0 {
			errs = append(errs, fmt.Sprint("positive value expected for '", f.Label, "'"))
		} else if !f.IsPositive && value > 0 {
			errs = append(errs, fmt.Sprint("negative value expected for '", f.Label, "'"))
		}
	}

	return
}
func (f *NumberField) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &f)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
