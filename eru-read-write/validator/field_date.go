package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type DateField struct {
	Field
	MinValue  string   `json:"min_value"`
	MinCheck  bool     `json:"min_check"`
	MaxValue  string   `json:"max_value"`
	MaxCheck  bool     `json:"max_check"`
	AllowDays []string `json:"allow_days"`
}

func (f *DateField) Validate(ctx context.Context, v interface{}) (err error) {
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
	if v.(string) == "" {
		if f.Required {
			errs = append(errs, fmt.Sprint("'", f.Label, "' cannot be blank"))
		}
		return
	}

	var value time.Time
	value, err = time.Parse("2006-01-02", v.(string))
	if err != nil {
		errs = append(errs, fmt.Sprint("'", f.Label, "' has to be a date"))
		return
	}

	valueDtMin, valueDtMinErr := time.Parse("2006-01-02", f.MinValue)

	if f.MinCheck {
		if valueDtMinErr != nil {
			errs = append(errs, fmt.Sprint("'incorrect minvalue for field ", f.Label))
			return
		}

		if valueDtMin.Sub(value) > 0 {
			errs = append(errs, fmt.Sprint("minimum date for '", f.Label, "' has to be ", f.MinValue))
		}
	}

	if f.MaxCheck {
		valueDtMax, valueDtMaxErr := time.Parse("2006-01-02", f.MaxValue)

		if valueDtMaxErr != nil {
			errs = append(errs, fmt.Sprint("'incorrect maxvalue for field ", f.Label))
			return
		}

		if valueDtMax.Sub(value) < 0 {
			errs = append(errs, fmt.Sprint("maximum date of '", f.Label, "' has to be ", f.MaxValue))
		}
	}
	adErr := false
	wd := value.Weekday()
	for _, ad := range f.AllowDays {
		adErr = true
		switch ad {
		case "Sun":
			if wd == time.Sunday {
				adErr = false
				break
			}
		case "Mon":
			if wd == time.Monday {
				adErr = false
				break
			}
		case "Tue":
			if wd == time.Tuesday {
				adErr = false
				break
			}
		case "Wed":
			if wd == time.Wednesday {
				adErr = false
				break
			}
		case "Thu":
			if wd == time.Thursday {
				adErr = false
				break
			}
		case "Fri":
			if wd == time.Friday {
				adErr = false
				break
			}
		case "Sat":
			if wd == time.Saturday {
				adErr = false
				break
			}
		}
		if !adErr {
			break
		}
	}
	if adErr {
		errs = append(errs, fmt.Sprint("invalid date value for '", f.Label))
	}
	return
}
func (f *DateField) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &f)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
