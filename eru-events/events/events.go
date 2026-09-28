package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/jmoiron/sqlx"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

type EventI interface {
	GetAttribute(attributeName string) (attributeValue interface{}, err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	Init(ctx context.Context) error
	CreateEvent(ctx context.Context) (err error)
	DeleteEvent(ctx context.Context) (err error)
	Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error)
	Poll(ctx context.Context) (eventMsgs []EventMsg, err error)
	DeleteMessage(ctx context.Context, msgIdentifier string) (err error)
	Clone(ctx context.Context) (cloneEvent EventI, err error)
	SetCon(con *sqlx.DB, dbType string)
	InitiatPollingInterval(ctx context.Context)
	Subscribe(ctx context.Context, subscription map[string]interface{}) (err error)
	Unsubscribe(ctx context.Context, subscriptionId string) (err error)
	ListSubscriptions(ctx context.Context) (subscriptions []map[string]interface{}, err error)
}

type EventNotification struct {
	DataType    string `json:"data_type" eru:"required"`
	BinaryValue []byte `json:"binary_value"`
	StringValue string `json:"string_value"`
}
type Event struct {
	EventType       string `json:"event_type" eru:"required"`
	EventName       string `json:"event_name" eru:"required"`
	PollingInterval int32  `json:"polling_interval" eru:"required"`
}
type EventMsg struct {
	Msg          string `json:"msg" `
	MsgIdentifer string `json:"msg_identifer" `
}

func (event *Event) GetAttribute(attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "event_name":
		return event.EventName, nil
	case "event_type":
		return event.EventType, nil
	case "polling_interval":
		return event.PollingInterval, nil
	default:
		return nil, errors.New("attribute not found")
	}
}

func GetEvent(eventType string) EventI {
	switch eventType {
	case "AWS_SQS":
		return new(AWS_SQS_Event)
	case "AWS_SNS":
		return new(AWS_SNS_Event)
	case "GCP_PUBSUB":
		return new(GCP_PUBSUB_Event)
	case "DB":
		return new(DB_Event)
	/* case "KAFKA":
	return new(Kafka_Event) */
	default:
		return nil
	}
}
func (event *Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) Init(ctx context.Context) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) CreateEvent(ctx context.Context) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) DeleteEvent(ctx context.Context) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	return e.Publish(ctx, msg, nil)
}

func (event *Event) Poll(ctx context.Context) (eventMsgs []EventMsg, err error) {
	err = errors.New("this event does not support polling")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) DeleteMessage(ctx context.Context, msgIdentifier string) (err error) {
	err = errors.New("method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	err = errors.New("Clone method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) SetCon(con *sqlx.DB, dbType string) {
	//do nothing
}

func (event *Event) InitiatPollingInterval(ctx context.Context) {
	time.Sleep(time.Duration(event.PollingInterval) * time.Second)
}

func (event *Event) Subscribe(ctx context.Context, subscription map[string]interface{}) (err error) {
	err = errors.New("this event does not support subscription")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) Unsubscribe(ctx context.Context, subscriptionId string) (err error) {
	err = errors.New("this event does not support subscription")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (event *Event) ListSubscriptions(ctx context.Context) (subscriptions []map[string]interface{}, err error) {
	err = errors.New("this event does not support subscriptions")
	logs.WithContext(ctx).Error(err.Error())
	return
}
