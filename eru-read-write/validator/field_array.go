package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type ArrayField struct {
	Field
	Values []interface{} `json:"values"`
}

func (f *ArrayField) Validate(ctx context.Context, v interface{}) (err error) {
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

	value, ok := v.([]interface{})
	if !ok {
		errs = append(errs, fmt.Sprint("'", f.Label, "' has to be an array"))
		return
	}

	if len(value) == 0 {
		if f.Required {
			errs = append(errs, fmt.Sprint("'", f.Label, "' cannot be blank"))
		}
		return
	}
	var vFoundArray []interface{}
	if len(f.Values) > 0 {
		for _, rval := range value {
			vFound := false
			for _, val := range f.Values {
				if rval == val {
					vFound = true
					break
				}
			}
			if !vFound {
				vFoundArray = append(vFoundArray, rval)
			}
		}
		if len(vFoundArray) > 0 {
			errs = append(errs, fmt.Sprint("invalid value for field '", f.Label, "' : (", vFoundArray, ")"))
			return
		}
	}

	return
}

func (f *ArrayField) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &f)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
