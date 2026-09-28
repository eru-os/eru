package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

type AWS_SQS_Event struct {
	Event
	Region            string `json:"region" eru:"required"`
	Authentication    string `json:"authentication" eru:"required"`
	Key               string `json:"key" eru:"required"`
	Secret            string `json:"secret" eru:"required"`
	client            *sqs.Client
	QueueUrl          string            `json:"queue_url" eru:"required"`
	Fifo              bool              `json:"fifo" eru:"required"`
	Attributes        map[string]string `json:"attributes" eru:"required"`
	Tags              map[string]string `json:"tags" eru:"required"`
	MsgToPoll         int32             `json:"msg_to_poll" eru:"required"`
	WaitTime          int32             `json:"wait_time" eru:"required"`
	MsgVisibleTimeOut int32             `json:"msg_visible_time_out" eru:"required"`
}

func (aws_sqs_event *AWS_SQS_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aws_sqs_event.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if aws_sqs_event.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			aws_sqs_event.Key,
			aws_sqs_event.Secret,
			"", // a token will be created when the session is used.
		)
	} else if aws_sqs_event.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting AWS SQS with IAM role")
		// do nothing - no new attributes to set in config
	}
	aws_sqs_event.client = sqs.NewFromConfig(awsConf)

	return err
}

func (aws_sqs_event *AWS_SQS_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("CreateEvent - Start")
	if aws_sqs_event.client == nil {
		err = aws_sqs_event.Init(ctx)
		if err != nil {
			return
		}
	}

	if aws_sqs_event.Fifo {
		if aws_sqs_event.Attributes == nil {
			aws_sqs_event.Attributes = make(map[string]string)
		}
		aws_sqs_event.Attributes["FifoQueue"] = "true"
		if !strings.HasSuffix(aws_sqs_event.EventName, ".fifo") {
			aws_sqs_event.EventName = fmt.Sprint(aws_sqs_event.EventName, ".fifo")
		}
	}

	input := &sqs.CreateQueueInput{
		QueueName:  aws.String(aws_sqs_event.EventName),
		Attributes: aws_sqs_event.Attributes,
		Tags:       aws_sqs_event.Tags,
	}
	result, err := aws_sqs_event.client.CreateQueue(context.Background(), input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(*result.QueueUrl))
	aws_sqs_event.QueueUrl = *result.QueueUrl

	return
}

func (aws_sqs_event *AWS_SQS_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("DeleteEvent - Start")
	if aws_sqs_event.client == nil {
		err = aws_sqs_event.Init(ctx)
		if err != nil {
			return
		}
	}
	_, err = aws_sqs_event.client.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{
		QueueUrl: aws.String(aws_sqs_event.QueueUrl),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (aws_sqs_event *AWS_SQS_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &aws_sqs_event)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (aws_sqs_event *AWS_SQS_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("Publish - Start")
	msgBytes := []byte("")
	msgBytes, err = json.Marshal(msg)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if aws_sqs_event.client == nil {
		err = aws_sqs_event.Init(ctx)
		if err != nil {
			return
		}
	}
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(aws_sqs_event.EventName),
		MessageBody: aws.String(string(msgBytes)),
	}

	result, err := aws_sqs_event.client.SendMessage(context.Background(), input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	msgId = *result.MessageId
	return
}

func (aws_sqs_event *AWS_SQS_Event) Poll(ctx context.Context) (eventMsgs []EventMsg, err error) {
	logs.WithContext(ctx).Debug("Poll - Start")

	if aws_sqs_event.client == nil {
		err = aws_sqs_event.Init(ctx)
		if err != nil {
			return
		}
	}

	msgResult, err := aws_sqs_event.client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(aws_sqs_event.QueueUrl),
		MaxNumberOfMessages: aws_sqs_event.MsgToPoll,
		WaitTimeSeconds:     aws_sqs_event.WaitTime,
		VisibilityTimeout:   aws_sqs_event.MsgVisibleTimeOut,
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	for _, message := range msgResult.Messages {
		logs.WithContext(ctx).Info(fmt.Sprint(*message.Body))
		async_id, err := strconv.Unquote(*message.Body)
		logs.WithContext(ctx).Info(fmt.Sprint(async_id))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		eventMsg := EventMsg{Msg: async_id, MsgIdentifer: *message.ReceiptHandle}
		eventMsgs = append(eventMsgs, eventMsg)
	}
	return
}

func (aws_sqs_event *AWS_SQS_Event) DeleteMessage(ctx context.Context, msgIdentifier string) (err error) {
	logs.WithContext(ctx).Info(fmt.Sprint("DeleteMessage"))
	if aws_sqs_event.client == nil {
		err = aws_sqs_event.Init(ctx)
		if err != nil {
			return
		}
	}
	_, err = aws_sqs_event.client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(aws_sqs_event.QueueUrl),
		ReceiptHandle: aws.String(msgIdentifier),
	})
	return
}

func (aws_sqs_event *AWS_SQS_Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	cloneEventI, cloneEventIErr := eru_utils.CloneInterface(ctx, aws_sqs_event)
	if cloneEventIErr != nil {
		err = cloneEventIErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	cloneEventOk := false
	cloneEvent, cloneEventOk = cloneEventI.(*AWS_SQS_Event)
	if !cloneEventOk {
		err = errors.New("event cloning failed")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return new(AWS_SQS_Event), nil
}
