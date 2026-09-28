package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type ScheduleConfig struct {
	ExecutionTime  string   `json:"execution_time" eru:"required"`
	SchedulerName  string   `json:"scheduler_name" eru:"required"`
	SchedulerLabel string   `json:"scheduler_label" eru:"required"`
	StartDate      string   `json:"start_date" eru:"required"`
	EndDate        string   `json:"end_date"`
	RepeatEvery    int      `json:"repeat_every"`
	Frequency      string   `json:"frequency" eru:"required"`
	FrequencyDay   []string `json:"frequency_day"`
	FrequencyDate  int      `json:"frequency_date"`
}

const (
	FrequencyDaily     = "daily"
	FrequencyWeekly    = "weekly"
	FrequencyMonthly   = "monthly"
	FrequencyQuarterly = "quarterly"
	FrequencyYearly    = "yearly"
)

type Scheduler struct {
	SchedulerType string `json:"scheduler_type" eru:"required"`
	SchedulerName string `json:"scheduler_name" eru:"required"`
}

type SchedulerI interface {
	Schedule(ctx context.Context, scheduleJobName string, scheduleCommand string, scheduleCron string) (jobId string, err error)
	Unschedule(ctx context.Context, scheduleJobId string, scheduleJobName string) (err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	Init(ctx context.Context, rj *json.RawMessage) (err error)
	GetSchedulerName() string
}

func GetScheduler(schedulerType string) SchedulerI {
	switch schedulerType {
	case "pg_cron":
		return new(PgCronScheduler)
	default:
		return nil
	}
}

func (scheduler *Scheduler) Schedule(ctx context.Context, scheduleCommand string, scheduleCron string) (jobId string, err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (scheduler *Scheduler) Unschedule(ctx context.Context, scheduleJobId string, scheduleJobName string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}
func (scheduler *Scheduler) Init(ctx context.Context, rj *json.RawMessage) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (scheduler *Scheduler) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &scheduler)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (scheduler *Scheduler) GetSchedulerName() string {
	return scheduler.SchedulerName
}

func (scheduleConfig *ScheduleConfig) GetCronStr(ctx context.Context) string {
	etSplit := strings.Split(scheduleConfig.ExecutionTime, ":")
	hour, err := strconv.Atoi(etSplit[0])
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return ""
	}
	min := 0
	if len(etSplit) > 1 {
		min, err = strconv.Atoi(etSplit[1])
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return ""
		}
	}
	sd, err := time.Parse("2006-01-02", scheduleConfig.StartDate)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return ""
	}

	var cronStr string

	switch strings.ToLower(scheduleConfig.Frequency) {
	case "daily":
		cronStr = fmt.Sprintf("%d %d * * *", min, hour)

	case "weekly":
		// Convert frequency days to cron format
		weekdayNums := make([]string, 0)
		for _, day := range scheduleConfig.FrequencyDay {
			switch strings.ToLower(day) {
			case "monday":
				weekdayNums = append(weekdayNums, "1")
			case "tuesday":
				weekdayNums = append(weekdayNums, "2")
			case "wednesday":
				weekdayNums = append(weekdayNums, "3")
			case "thursday":
				weekdayNums = append(weekdayNums, "4")
			case "friday":
				weekdayNums = append(weekdayNums, "5")
			case "saturday":
				weekdayNums = append(weekdayNums, "6")
			case "sunday":
				weekdayNums = append(weekdayNums, "0")
			}
		}
		cronStr = fmt.Sprintf("%d %d * * %s", min, hour, strings.Join(weekdayNums, ","))

	case "monthly":
		cronStr = fmt.Sprintf("%d %d %d * *", min, hour, scheduleConfig.FrequencyDate)

	case "quarterly":
		// For quarterly, run on specified date every 3 months
		// Calculate which months to run based on start date

		startMonth := sd.Month()
		monthsToRun := make([]int, 0)

		// Get the first quarter month (rounds down to nearest quarter start)
		firstMonth := ((int(startMonth)-1)/3)*3 + 1

		// Add all 4 quarters
		for i := 0; i < 4; i++ {
			month := firstMonth + (i * 3)
			if month > 12 {
				month = month - 12
			}
			monthsToRun = append(monthsToRun, month)
		}
		cronStr = fmt.Sprintf("%d %d %d %s *", min, hour, scheduleConfig.FrequencyDate, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(monthsToRun)), ","), "[]"))

	case "yearly":
		// For yearly, run on specified date and month
		cronStr = fmt.Sprintf("%d %d %d %d *", min, hour, scheduleConfig.FrequencyDate, sd.Month())
	}

	logs.WithContext(ctx).Info(fmt.Sprint("Generated cron expression: ", cronStr))
	return cronStr
}
