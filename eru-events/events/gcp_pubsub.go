package events

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/pubsub"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	defaultGmailPublisherSA = "gmail-api-push@system.gserviceaccount.com"
	defaultAckDeadline      = 60
)

type GcpPubSubSubscriber struct {
	SubscriptionId   string `json:"subscription_id" eru:"required"`
	SubscriptionName string `json:"subscription_name" eru:"optional"`
	PushEndpoint     string `json:"push_endpoint" eru:"required"`
	AckDeadline      int    `json:"ack_deadline" eru:"optional"`
	Filter           string `json:"filter" eru:"optional"`
}

type GCP_PUBSUB_Event struct {
	Event
	GcpProjectId       string                `json:"gcp_project_id" eru:"required"`
	Authentication     string                `json:"authentication" eru:"required"`
	SaJsonBase64       string                `json:"sa_json_base64" eru:"optional"`
	TopicId            string                `json:"topic_id" eru:"required"`
	TopicName          string                `json:"-"`
	GmailPublisherSA   string                `json:"gmail_publisher_sa" eru:"optional"`
	OidcAudience       string                `json:"oidc_audience" eru:"optional"`
	OidcServiceAccount string                `json:"oidc_service_account" eru:"optional"`
	Subscribers        []GcpPubSubSubscriber `json:"subscribers" eru:"optional"`
	client             *pubsub.Client
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) GetAttribute(attributeName string) (interface{}, error) {
	switch attributeName {
	case "topic_name":
		return gcp_pubsub_event.TopicName, nil
	case "topic_id":
		return gcp_pubsub_event.TopicId, nil
	case "oidc_audience":
		return gcp_pubsub_event.OidcAudience, nil
	case "oidc_service_account":
		return gcp_pubsub_event.OidcServiceAccount, nil
	case "gcp_project_id":
		return gcp_pubsub_event.GcpProjectId, nil
	}
	return gcp_pubsub_event.Event.GetAttribute(attributeName)
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	if gcp_pubsub_event.GcpProjectId == "" {
		err = errors.New("gcp_project_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if gcp_pubsub_event.TopicId == "" {
		err = errors.New("topic_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	gcp_pubsub_event.TopicName = fmt.Sprintf("projects/%s/topics/%s", gcp_pubsub_event.GcpProjectId, gcp_pubsub_event.TopicId)
	switch gcp_pubsub_event.Authentication {
	case AuthTypeSecret:
		if gcp_pubsub_event.SaJsonBase64 == "" {
			err = errors.New("sa_json_base64 is required for SECRET authentication")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		saBytes, decodeErr := base64.StdEncoding.DecodeString(gcp_pubsub_event.SaJsonBase64)
		if decodeErr != nil {
			err = fmt.Errorf("failed to decode sa_json_base64: %w", decodeErr)
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		gcp_pubsub_event.client, err = pubsub.NewClient(ctx, gcp_pubsub_event.GcpProjectId, option.WithCredentialsJSON(saBytes))
	case AuthTypeIAM:
		logs.WithContext(ctx).Info("connecting GCP Pub/Sub with IAM (ADC / Workload Identity)")
		gcp_pubsub_event.client, err = pubsub.NewClient(ctx, gcp_pubsub_event.GcpProjectId)
	default:
		err = fmt.Errorf("unsupported authentication type: %s", gcp_pubsub_event.Authentication)
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &gcp_pubsub_event)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("CreateEvent - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	if gcp_pubsub_event.TopicId == "" {
		err = errors.New("topic_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	topic := gcp_pubsub_event.client.Topic(gcp_pubsub_event.TopicId)
	exists, existsErr := topic.Exists(ctx)
	if existsErr != nil {
		err = existsErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if !exists {
		topic, err = gcp_pubsub_event.client.CreateTopic(ctx, gcp_pubsub_event.TopicId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		logs.WithContext(ctx).Info(fmt.Sprint("created topic: ", topic.String()))
	} else {
		logs.WithContext(ctx).Info(fmt.Sprint("topic already exists: ", topic.String()))
	}
	publisher := gcp_pubsub_event.GmailPublisherSA
	if publisher == "" {
		publisher = defaultGmailPublisherSA
	}
	publisherMember := fmt.Sprintf("serviceAccount:%s", publisher)
	policy, policyErr := topic.IAM().Policy(ctx)
	if policyErr != nil {
		err = policyErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	role := iam.RoleName("roles/pubsub.publisher")
	if !hasMember(policy, publisherMember, role) {
		policy.Add(publisherMember, role)
		if err = topic.IAM().SetPolicy(ctx, policy); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		logs.WithContext(ctx).Info(fmt.Sprint("granted ", role, " to ", publisherMember))
	}
	return
}

func hasMember(policy *iam.Policy, member string, role iam.RoleName) bool {
	for _, m := range policy.Members(role) {
		if m == member {
			return true
		}
	}
	return false
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("DeleteEvent - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	for _, sub := range gcp_pubsub_event.Subscribers {
		s := gcp_pubsub_event.client.Subscription(sub.SubscriptionId)
		if exists, _ := s.Exists(ctx); exists {
			if delErr := s.Delete(ctx); delErr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("failed to delete subscription ", sub.SubscriptionId, ": ", delErr.Error()))
			}
		}
	}
	gcp_pubsub_event.Subscribers = nil
	topic := gcp_pubsub_event.client.Topic(gcp_pubsub_event.TopicId)
	if exists, _ := topic.Exists(ctx); exists {
		if err = topic.Delete(ctx); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("Publish - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	topic := gcp_pubsub_event.client.Topic(gcp_pubsub_event.TopicId)
	result := topic.Publish(ctx, &pubsub.Message{Data: msgBytes})
	msgId, err = result.Get(ctx)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) Subscribe(ctx context.Context, subscription map[string]interface{}) (err error) {
	logs.WithContext(ctx).Debug("Subscribe - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	subscriber := GcpPubSubSubscriber{}
	if v, ok := subscription["subscription_id"].(string); ok {
		subscriber.SubscriptionId = v
	}
	if v, ok := subscription["push_endpoint"].(string); ok {
		subscriber.PushEndpoint = v
	}
	if v, ok := subscription["filter"].(string); ok {
		subscriber.Filter = v
	}
	switch v := subscription["ack_deadline"].(type) {
	case float64:
		subscriber.AckDeadline = int(v)
	case int:
		subscriber.AckDeadline = v
	}
	if subscriber.AckDeadline <= 0 {
		subscriber.AckDeadline = defaultAckDeadline
	}
	if subscriber.SubscriptionId == "" || subscriber.PushEndpoint == "" {
		err = errors.New("subscription_id and push_endpoint are required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	audience := gcp_pubsub_event.OidcAudience
	if audience == "" {
		audience = subscriber.PushEndpoint
	}

	topic := gcp_pubsub_event.client.Topic(gcp_pubsub_event.TopicId)
	cfg := pubsub.SubscriptionConfig{
		Topic:       topic,
		AckDeadline: secondsToDuration(subscriber.AckDeadline),
		PushConfig: pubsub.PushConfig{
			Endpoint: subscriber.PushEndpoint,
		},
	}
	if gcp_pubsub_event.OidcServiceAccount != "" {
		cfg.PushConfig.AuthenticationMethod = &pubsub.OIDCToken{
			Audience:            audience,
			ServiceAccountEmail: gcp_pubsub_event.OidcServiceAccount,
		}
	}
	if subscriber.Filter != "" {
		cfg.Filter = subscriber.Filter
	}

	existing := gcp_pubsub_event.client.Subscription(subscriber.SubscriptionId)
	if exists, _ := existing.Exists(ctx); exists {
		logs.WithContext(ctx).Info(fmt.Sprint("subscription already exists, updating push config: ", subscriber.SubscriptionId))
		_, updErr := existing.Update(ctx, pubsub.SubscriptionConfigToUpdate{
			PushConfig: &cfg.PushConfig,
		})
		if updErr != nil {
			err = updErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		subscriber.SubscriptionName = existing.String()
	} else {
		created, createErr := gcp_pubsub_event.client.CreateSubscription(ctx, subscriber.SubscriptionId, cfg)
		if createErr != nil {
			err = createErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		subscriber.SubscriptionName = created.String()
		logs.WithContext(ctx).Info(fmt.Sprint("created subscription: ", subscriber.SubscriptionName))
	}

	replaced := false
	for i, s := range gcp_pubsub_event.Subscribers {
		if s.SubscriptionId == subscriber.SubscriptionId {
			gcp_pubsub_event.Subscribers[i] = subscriber
			replaced = true
			break
		}
	}
	if !replaced {
		gcp_pubsub_event.Subscribers = append(gcp_pubsub_event.Subscribers, subscriber)
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) Unsubscribe(ctx context.Context, subscriptionId string) (err error) {
	logs.WithContext(ctx).Debug("Unsubscribe - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	if subscriptionId == "" {
		err = errors.New("subscription_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	sub := gcp_pubsub_event.client.Subscription(subscriptionId)
	if exists, _ := sub.Exists(ctx); exists {
		if err = sub.Delete(ctx); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	}
	for i, s := range gcp_pubsub_event.Subscribers {
		if s.SubscriptionId == subscriptionId {
			gcp_pubsub_event.Subscribers = append(gcp_pubsub_event.Subscribers[:i], gcp_pubsub_event.Subscribers[i+1:]...)
			break
		}
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) ListSubscriptions(ctx context.Context) (subscriptions []map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ListSubscriptions - Start")
	if gcp_pubsub_event.client == nil {
		if err = gcp_pubsub_event.Init(ctx); err != nil {
			return
		}
	}
	topic := gcp_pubsub_event.client.Topic(gcp_pubsub_event.TopicId)
	it := topic.Subscriptions(ctx)
	subscriptions = make([]map[string]interface{}, 0)
	for {
		sub, iterErr := it.Next()
		if iterErr == iterator.Done {
			break
		}
		if iterErr != nil {
			err = iterErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		subscriptions = append(subscriptions, map[string]interface{}{
			"subscription_name": sub.String(),
			"subscription_id":   sub.ID(),
		})
	}
	return
}

func (gcp_pubsub_event *GCP_PUBSUB_Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	cloneEventI, cloneEventIErr := eru_utils.CloneInterface(ctx, gcp_pubsub_event)
	if cloneEventIErr != nil {
		err = cloneEventIErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	cloneEventOk := false
	cloneEvent, cloneEventOk = cloneEventI.(*GCP_PUBSUB_Event)
	if !cloneEventOk {
		err = errors.New("event cloning failed")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return cloneEvent, nil
}

func secondsToDuration(s int) time.Duration {
	return time.Duration(s) * time.Second
}
