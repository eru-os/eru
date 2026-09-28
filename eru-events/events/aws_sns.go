package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

// Subscriber represents an SNS subscription configuration
type AwsSnsSubscriber struct {
	Protocol           string                 `json:"protocol" eru:"required"`             // HTTP, HTTPS, Email, SMS, Lambda, SQS
	Endpoint           string                 `json:"endpoint" eru:"required"`             // URL, email, phone, ARN, etc.
	FilterPolicy       map[string]interface{} `json:"filter_policy" eru:"optional"`        // JSON filter policy for message filtering
	RawMessageDelivery bool                   `json:"raw_message_delivery" eru:"optional"` // Deliver raw message instead of JSON
	SubscriptionArn    string                 `json:"subscription_arn" eru:"optional"`
	UnsubscribeURL     string                 `json:"unsubscribe_url" eru:"optional"` // Subscription ARN returned from AWS (populated after subscription)
}

type AWS_SNS_Event struct {
	Event
	Region         string `json:"region" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key" eru:"required"`
	Secret         string `json:"secret" eru:"required"`
	client         *sns.Client
	TopicArn       string            `json:"topic_arn" eru:"required"`
	Attributes     map[string]string `json:"attributes" eru:"required"`
	Tags           map[string]string `json:"tags" eru:"required"`
	// SNS subscription configuration - array of subscribers
	Subscribers []AwsSnsSubscriber `json:"subscribers" eru:"optional"` // Array of subscribers for this topic
	// FIFO topic support
	Fifo                      bool `json:"fifo" eru:"optional"`                        // Enable FIFO topic with message ordering
	ContentBasedDeduplication bool `json:"content_based_deduplication" eru:"optional"` // Enable automatic deduplication based on message content
	HighThroughputFIFO        bool `json:"high_throughput_fifo" eru:"optional"`        // Enable High Throughput FIFO for improved performance (up to 300 msg/sec without batching)
}

func (aws_sns_event *AWS_SNS_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aws_sns_event.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	switch aws_sns_event.Authentication {
	case AuthTypeSecret:
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			aws_sns_event.Key,
			aws_sns_event.Secret,
			"", // a token will be created when the session is used.
		)
	case AuthTypeIAM:
		logs.WithContext(ctx).Info("connecting AWS SNS with IAM role")
		// do nothing - no new attributes to set in config
	}
	aws_sns_event.client = sns.NewFromConfig(awsConf)

	return err
}

func (aws_sns_event *AWS_SNS_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("CreateEvent - Start")
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}

	// Validate FIFO configuration if this is a FIFO topic
	if err = aws_sns_event.ValidateFIFOConfig(ctx); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	// Handle FIFO topic naming convention
	topicName := aws_sns_event.EventName
	if aws_sns_event.Fifo && !strings.HasSuffix(topicName, ".fifo") {
		topicName = topicName + ".fifo"
	}

	// Prepare attributes for FIFO topics
	attributes := make(map[string]string)
	for k, v := range aws_sns_event.Attributes {
		attributes[k] = v
	}

	// Set FIFO-specific attributes
	if aws_sns_event.Fifo {
		attributes["FifoTopic"] = "true"
		attributes["ContentBasedDeduplication"] = fmt.Sprintf("%t", aws_sns_event.ContentBasedDeduplication)

		// Enable High Throughput FIFO if requested
		if aws_sns_event.HighThroughputFIFO {
			attributes["HighThroughputFifo"] = "true"
			logs.WithContext(ctx).Info("Creating High Throughput FIFO topic")
		}
	}

	input := &sns.CreateTopicInput{
		Name:       aws.String(topicName),
		Attributes: attributes,
	}
	result, err := aws_sns_event.client.CreateTopic(context.Background(), input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(*result.TopicArn))
	aws_sns_event.TopicArn = *result.TopicArn

	return
}

func (aws_sns_event *AWS_SNS_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("DeleteEvent - Start")
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}
	_, err = aws_sns_event.client.DeleteTopic(context.Background(), &sns.DeleteTopicInput{
		TopicArn: aws.String(aws_sns_event.TopicArn),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (aws_sns_event *AWS_SNS_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &aws_sns_event)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (aws_sns_event *AWS_SNS_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("Publish - Start")
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}
	input := &sns.PublishInput{
		TopicArn: aws.String(aws_sns_event.TopicArn),
		Message:  aws.String(string(msgBytes)),
	}

	// Handle FIFO-specific message attributes
	if aws_sns_event.Fifo {
		// For FIFO topics, we need MessageGroupId and optionally MessageDeduplicationId
		// If content-based deduplication is enabled, MessageDeduplicationId is optional
		if !aws_sns_event.ContentBasedDeduplication {
			// Generate a deduplication ID based on message content
			hash := sha256.Sum256([]byte(string(msgBytes)))
			dedupId := hex.EncodeToString(hash[:])
			input.MessageDeduplicationId = aws.String(dedupId)
		}

		// MessageGroupId is required for FIFO topics
		// Using a default group if not specified in attributes
		messageGroupId := "default"
		if groupId, exists := aws_sns_event.Attributes["MessageGroupId"]; exists {
			messageGroupId = groupId
		}
		input.MessageGroupId = aws.String(messageGroupId)
	}

	result, err := aws_sns_event.client.Publish(context.Background(), input)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	msgId = *result.MessageId
	return
}

func (aws_sns_event *AWS_SNS_Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	cloneEventI, cloneEventIErr := eru_utils.CloneInterface(ctx, aws_sns_event)
	if cloneEventIErr != nil {
		err = cloneEventIErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	cloneEventOk := false
	cloneEvent, cloneEventOk = cloneEventI.(*AWS_SNS_Event)
	if !cloneEventOk {
		err = errors.New("event cloning failed")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return cloneEvent, nil
}

func (aws_sns_event *AWS_SNS_Event) GetTopicAttributes(ctx context.Context) (attributes map[string]string, err error) {
	logs.WithContext(ctx).Debug("GetTopicAttributes - Start")
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}

	result, err := aws_sns_event.client.GetTopicAttributes(context.Background(), &sns.GetTopicAttributesInput{
		TopicArn: aws.String(aws_sns_event.TopicArn),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	return result.Attributes, nil
}

func (aws_sns_event *AWS_SNS_Event) ValidateFIFOConfig(ctx context.Context) error {
	if !aws_sns_event.Fifo {
		return nil
	}

	// Check if topic name ends with .fifo
	if !strings.HasSuffix(aws_sns_event.EventName, ".fifo") {
		return errors.New("FIFO topic names must end with '.fifo'")
	}

	// Validate that required attributes are set for FIFO topics
	if aws_sns_event.Attributes == nil {
		aws_sns_event.Attributes = make(map[string]string)
	}

	// Set default MessageGroupId if not provided
	if _, exists := aws_sns_event.Attributes["MessageGroupId"]; !exists {
		aws_sns_event.Attributes["MessageGroupId"] = "default"
	}

	return nil
}

func (aws_sns_event *AWS_SNS_Event) Subscribe(ctx context.Context, subscription map[string]interface{}) (err error) {
	logs.WithContext(ctx).Debug("Subscribe - Start")
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Subscription: %+v", subscription))
	subscriber := AwsSnsSubscriber{}
	if protocol, ok := subscription["protocol"]; ok {
		if protocolStr, ok := protocol.(string); ok {
			subscriber.Protocol = protocolStr
		}
	}
	if endpoint, ok := subscription["endpoint"]; ok {
		if endpointStr, ok := endpoint.(string); ok {
			subscriber.Endpoint = endpointStr
		}
	}
	if filterPolicy, ok := subscription["filter_policy"]; ok {
		if filterPolicyMap, ok := filterPolicy.(map[string]interface{}); ok {
			subscriber.FilterPolicy = filterPolicyMap
		}
	}
	subscriber.RawMessageDelivery = true
	if rawMessageDelivery, ok := subscription["raw_message_delivery"]; ok {
		if rawMessageDeliveryBool, ok := rawMessageDelivery.(bool); ok {
			subscriber.RawMessageDelivery = rawMessageDeliveryBool
		}
	}

	if subscriptionArn, ok := subscription["subscription_arn"]; ok {
		if subscriptionArnStr, ok := subscriptionArn.(string); ok {
			subscriber.SubscriptionArn = subscriptionArnStr
		}
	}
	if subscriber.Protocol == "" || subscriber.Endpoint == "" {
		err = errors.New("protocol and endpoint are required for subscriber")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	attributes := make(map[string]string)
	attributes["RawMessageDelivery"] = fmt.Sprintf("%t", subscriber.RawMessageDelivery)
	if subscriber.FilterPolicy != nil {
		filterPolicyBytes, fpErr := json.Marshal(subscriber.FilterPolicy)
		if fpErr != nil {
			logs.WithContext(ctx).Error(fpErr.Error())
			return fpErr
		}
		attributes["FilterPolicy"] = string(filterPolicyBytes)
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Attributes: %+v", attributes))
	input := &sns.SubscribeInput{
		TopicArn:   aws.String(aws_sns_event.TopicArn),
		Protocol:   aws.String(subscriber.Protocol),
		Endpoint:   aws.String(subscriber.Endpoint),
		Attributes: attributes,
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Input: %v", input))

	result, err := aws_sns_event.client.Subscribe(context.Background(), input)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to subscribe subscriber: %s", err.Error()))
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Subscribed to: %s", *result.SubscriptionArn))
	logs.WithContext(ctx).Info(fmt.Sprintf("Result: %+v", result))

	subscriber.SubscriptionArn = *result.SubscriptionArn
	aws_sns_event.Subscribers = append(aws_sns_event.Subscribers, subscriber)
	return
}

func (aws_sns_event *AWS_SNS_Event) Unsubscribe(ctx context.Context, subscriptionId string) (err error) {
	logs.WithContext(ctx).Debug("Unsubscribe - Start")
	if aws_sns_event.client == nil {
		err := aws_sns_event.Init(ctx)
		if err != nil {
			return err
		}
	}
	subscriptionArn := ""
	for i, sub := range aws_sns_event.Subscribers {
		if sub.SubscriptionArn == subscriptionId || strings.Contains(sub.Endpoint, subscriptionId) {
			subscriptionArn = sub.SubscriptionArn
			aws_sns_event.Subscribers = append(aws_sns_event.Subscribers[:i], aws_sns_event.Subscribers[i+1:]...)
			break
		}
	}
	if subscriptionArn == "" {
		subscriptionArn = subscriptionId
	}
	_, err = aws_sns_event.client.Unsubscribe(context.Background(), &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(subscriptionArn),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Unsubscribed from: %s", subscriptionArn))
	return nil
}

func (aws_sns_event *AWS_SNS_Event) ListSubscriptions(ctx context.Context) (subscriptions []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ListSubscriptions - Start")
	if aws_sns_event.client == nil {
		err = aws_sns_event.Init(ctx)
		if err != nil {
			return
		}
	}

	result, err := aws_sns_event.client.ListSubscriptionsByTopic(context.Background(), &sns.ListSubscriptionsByTopicInput{
		TopicArn: aws.String(aws_sns_event.TopicArn),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	subscriptions = make([]map[string]interface{}, 0)
	for _, sub := range result.Subscriptions {
		subMap := map[string]interface{}{
			"SubscriptionArn": *sub.SubscriptionArn,
			"Protocol":        *sub.Protocol,
			"Endpoint":        *sub.Endpoint,
			"Owner":           *sub.Owner,
		}
		if sub.TopicArn != nil {
			subMap["TopicArn"] = *sub.TopicArn
		}
		subscriptions = append(subscriptions, subMap)
	}

	return subscriptions, nil
}
