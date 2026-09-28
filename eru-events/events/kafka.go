//go:build cgo
// +build cgo

package events

/* import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/jmoiron/sqlx"
)

type Kafka_Event struct {
	Event

	// Connection Configuration
	Brokers string `json:"brokers" eru:"required"`
	Topic   string `json:"topic" eru:"required"`
	GroupId string `json:"group_id" eru:"required"`

	// Security Configuration
	SecurityProtocol        string `json:"security_protocol"`
	SaslMechanism           string `json:"sasl_mechanism"`
	SaslUsername            string `json:"sasl_username"`
	SaslPassword            string `json:"sasl_password"`
	SaslKerberosServiceName string `json:"sasl_kerberos_service_name"`
	SaslKerberosPrincipal   string `json:"sasl_kerberos_principal"`
	SaslKerberosKinitCmd    string `json:"sasl_kerberos_kinit_cmd"`
	SaslOauthToken          string `json:"sasl_oauth_token"`

	// AWS MSK IAM Authentication
	AwsRegion          string `json:"aws_region"`
	AwsAccessKeyId     string `json:"aws_access_key_id"`
	AwsSecretAccessKey string `json:"aws_secret_access_key"`
	AwsSessionToken    string `json:"aws_session_token"`

	// SSL/TLS Configuration
	SslCaLocation          string `json:"ssl_ca_location"`
	SslCertificateLocation string `json:"ssl_certificate_location"`
	SslKeyLocation         string `json:"ssl_key_location"`
	SslKeyPassword         string `json:"ssl_key_password"`
	SslCrlLocation         string `json:"ssl_crl_location"`
	SslKeystoreLocation    string `json:"ssl_keystore_location"`
	SslKeystorePassword    string `json:"ssl_keystore_password"`
	SslTruststoreLocation  string `json:"ssl_truststore_location"`
	SslTruststorePassword  string `json:"ssl_truststore_password"`

	// Topic Configuration
	Partitions        int32             `json:"partitions"`
	ReplicationFactor int16             `json:"replication_factor"`
	TopicConfig       map[string]string `json:"topic_config"`

	// Consumer Configuration
	SessionTimeoutMs       int    `json:"session_timeout_ms"`
	AutoOffsetReset        string `json:"auto_offset_reset"`
	EnableAutoCommit       bool   `json:"enable_auto_commit"`
	FetchMinBytes          int    `json:"fetch_min_bytes"`
	FetchMaxWaitMs         int    `json:"fetch_max_wait_ms"`
	MaxPartitionFetchBytes int    `json:"max_partition_fetch_bytes"`
	HeartbeatIntervalMs    int    `json:"heartbeat_interval_ms"`
	MaxPollRecords         int    `json:"max_poll_records"`
	IsolationLevel         string `json:"isolation_level"`

	// Producer Configuration
	Acks                string `json:"acks"`
	Retries             int    `json:"retries"`
	RetryBackoffMs      int    `json:"retry_backoff_ms"`
	RequestTimeoutMs    int    `json:"request_timeout_ms"`
	DeliveryTimeoutMs   int    `json:"delivery_timeout_ms"`
	BatchSize           int    `json:"batch_size"`
	LingerMs            int    `json:"linger_ms"`
	CompressionType     string `json:"compression_type"`
	MaxInFlightRequests int    `json:"max_in_flight_requests"`
	BufferMemory        int    `json:"buffer_memory"`

	// Internal client instances
	producer    *kafka.Producer
	consumer    *kafka.Consumer
	adminClient *kafka.AdminClient
}

func (kafkaEvent *Kafka_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Kafka Init - Start")

	// Base configuration for all clients
	baseConfig := kafka.ConfigMap{
		"bootstrap.servers": kafkaEvent.Brokers,
	}

	// Apply security configuration
	kafkaEvent.applySecurityConfig(ctx, baseConfig)

	// Create producer config with performance settings
	producerConfig := kafka.ConfigMap{}
	for k, v := range baseConfig {
		producerConfig[k] = v
	}
	kafkaEvent.applyProducerConfig(ctx, producerConfig)

	// Create consumer config with consumer settings
	consumerConfig := kafka.ConfigMap{}
	for k, v := range baseConfig {
		consumerConfig[k] = v
	}
	consumerConfig["group.id"] = kafkaEvent.GroupId
	consumerConfig["auto.offset.reset"] = kafkaEvent.getAutoOffsetReset()
	consumerConfig["enable.auto.commit"] = kafkaEvent.EnableAutoCommit
	kafkaEvent.applyConsumerConfig(ctx, consumerConfig)

	// Create admin config
	adminConfig := kafka.ConfigMap{}
	for k, v := range baseConfig {
		adminConfig[k] = v
	}

	kafkaEvent.producer, err = kafka.NewProducer(&producerConfig)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create Kafka producer: %v", err))
		return err
	}

	kafkaEvent.consumer, err = kafka.NewConsumer(&consumerConfig)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create Kafka consumer: %v", err))
		return err
	}

	kafkaEvent.adminClient, err = kafka.NewAdminClient(&adminConfig)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create Kafka admin client: %v", err))
		return err
	}

	err = kafkaEvent.consumer.Subscribe(kafkaEvent.Topic, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to subscribe to topic %s: %v", kafkaEvent.Topic, err))
		return err
	}

	logs.WithContext(ctx).Debug("Kafka Init - Complete")
	return nil
}

func (kafkaEvent *Kafka_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("Kafka CreateEvent - Start")

	if kafkaEvent.adminClient == nil {
		err = kafkaEvent.Init(ctx)
		if err != nil {
			return err
		}
	}

	topicSpec := kafka.TopicSpecification{
		Topic:             kafkaEvent.Topic,
		NumPartitions:     int(kafkaEvent.getPartitions()),
		ReplicationFactor: int(kafkaEvent.getReplicationFactor()),
		Config:            kafkaEvent.TopicConfig,
	}

	results, err := kafkaEvent.adminClient.CreateTopics(ctx, []kafka.TopicSpecification{topicSpec})
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create topic: %v", err))
		return err
	}

	for _, result := range results {
		if result.Error.Code() != kafka.ErrNoError && result.Error.Code() != kafka.ErrTopicAlreadyExists {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to create topic %s: %v", result.Topic, result.Error))
			return errors.New(result.Error.String())
		}
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Topic %s created successfully", kafkaEvent.Topic))
	return nil
}

func (kafkaEvent *Kafka_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("Kafka DeleteEvent - Start")

	if kafkaEvent.adminClient == nil {
		err = kafkaEvent.Init(ctx)
		if err != nil {
			return err
		}
	}

	results, err := kafkaEvent.adminClient.DeleteTopics(ctx, []string{kafkaEvent.Topic})
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to delete topic: %v", err))
		return err
	}

	for _, result := range results {
		if result.Error.Code() != kafka.ErrNoError {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to delete topic %s: %v", result.Topic, result.Error))
			return errors.New(result.Error.String())
		}
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Topic %s deleted successfully", kafkaEvent.Topic))
	return nil
}

func (kafkaEvent *Kafka_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("Kafka MakeFromJson - Start")
	err := json.Unmarshal(*rj, &kafkaEvent)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (kafkaEvent *Kafka_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("Kafka Publish - Start")

	if kafkaEvent.producer == nil {
		err = kafkaEvent.Init(ctx)
		if err != nil {
			return "", err
		}
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}

	deliveryChan := make(chan kafka.Event)
	defer close(deliveryChan)

	err = kafkaEvent.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &kafkaEvent.Topic, Partition: kafka.PartitionAny},
		Value:          msgBytes,
	}, deliveryChan)

	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to produce message: %v", err))
		return "", err
	}

	event := <-deliveryChan
	m := event.(*kafka.Message)

	if m.TopicPartition.Error != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Delivery failed: %v", m.TopicPartition.Error))
		return "", m.TopicPartition.Error
	}

	msgId = fmt.Sprintf("%s-%d-%d", *m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
	logs.WithContext(ctx).Info(fmt.Sprintf("Message delivered to %v", m.TopicPartition))
	return msgId, nil
}

func (kafkaEvent *Kafka_Event) Poll(ctx context.Context) (eventMsgs []EventMsg, err error) {
	logs.WithContext(ctx).Debug("Kafka Poll - Start")

	if kafkaEvent.consumer == nil {
		err = kafkaEvent.Init(ctx)
		if err != nil {
			return nil, err
		}
	}

	timeoutMs := time.Duration(kafkaEvent.PollingInterval) * time.Second
	msg, err := kafkaEvent.consumer.ReadMessage(timeoutMs)
	if err != nil {
		if err.(kafka.Error).Code() == kafka.ErrTimedOut {
			return eventMsgs, nil
		}
		logs.WithContext(ctx).Error(fmt.Sprintf("Consumer error: %v", err))
		return nil, err
	}

	msgIdentifier := fmt.Sprintf("%s-%d-%d", *msg.TopicPartition.Topic, msg.TopicPartition.Partition, msg.TopicPartition.Offset)
	eventMsg := EventMsg{
		Msg:          string(msg.Value),
		MsgIdentifer: msgIdentifier,
	}
	eventMsgs = append(eventMsgs, eventMsg)

	logs.WithContext(ctx).Info(fmt.Sprintf("Message polled from %v: %s", msg.TopicPartition, string(msg.Value)))
	return eventMsgs, nil
}

func (kafkaEvent *Kafka_Event) DeleteMessage(ctx context.Context, msgIdentifier string) (err error) {
	logs.WithContext(ctx).Info("Kafka DeleteMessage - Start")
	return nil
}

func (kafkaEvent *Kafka_Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	cloneEventI, cloneEventIErr := eru_utils.CloneInterface(ctx, kafkaEvent)
	if cloneEventIErr != nil {
		err = cloneEventIErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	cloneEventOk := false
	cloneEvent, cloneEventOk = cloneEventI.(*Kafka_Event)
	if !cloneEventOk {
		err = errors.New("kafka event cloning failed")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return cloneEvent, nil
}

func (kafkaEvent *Kafka_Event) SetCon(con *sqlx.DB, dbType string) {
}

func (kafkaEvent *Kafka_Event) InitiatPollingInterval(ctx context.Context) {
	time.Sleep(time.Duration(kafkaEvent.PollingInterval) * time.Second)
}

func (kafkaEvent *Kafka_Event) GetAttribute(attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	// Base event attributes
	case "event_name":
		return kafkaEvent.EventName, nil
	case "event_type":
		return kafkaEvent.EventType, nil
	case "polling_interval":
		return kafkaEvent.PollingInterval, nil

	// Connection attributes
	case "brokers":
		return kafkaEvent.Brokers, nil
	case "topic":
		return kafkaEvent.Topic, nil
	case "group_id":
		return kafkaEvent.GroupId, nil

	// Security attributes
	case "security_protocol":
		return kafkaEvent.SecurityProtocol, nil
	case "sasl_mechanism":
		return kafkaEvent.SaslMechanism, nil
	case "sasl_username":
		return kafkaEvent.SaslUsername, nil
	case "sasl_kerberos_service_name":
		return kafkaEvent.SaslKerberosServiceName, nil
	case "sasl_kerberos_principal":
		return kafkaEvent.SaslKerberosPrincipal, nil
	case "aws_region":
		return kafkaEvent.AwsRegion, nil
	case "aws_access_key_id":
		return kafkaEvent.AwsAccessKeyId, nil

	// SSL attributes
	case "ssl_ca_location":
		return kafkaEvent.SslCaLocation, nil
	case "ssl_certificate_location":
		return kafkaEvent.SslCertificateLocation, nil
	case "ssl_key_location":
		return kafkaEvent.SslKeyLocation, nil

	// Topic configuration
	case "partitions":
		return kafkaEvent.Partitions, nil
	case "replication_factor":
		return kafkaEvent.ReplicationFactor, nil
	case "topic_config":
		return kafkaEvent.TopicConfig, nil

	// Consumer configuration
	case "session_timeout_ms":
		return kafkaEvent.SessionTimeoutMs, nil
	case "auto_offset_reset":
		return kafkaEvent.AutoOffsetReset, nil
	case "enable_auto_commit":
		return kafkaEvent.EnableAutoCommit, nil
	case "fetch_min_bytes":
		return kafkaEvent.FetchMinBytes, nil
	case "max_poll_records":
		return kafkaEvent.MaxPollRecords, nil
	case "isolation_level":
		return kafkaEvent.IsolationLevel, nil

	// Producer configuration
	case "acks":
		return kafkaEvent.Acks, nil
	case "retries":
		return kafkaEvent.Retries, nil
	case "batch_size":
		return kafkaEvent.BatchSize, nil
	case "compression_type":
		return kafkaEvent.CompressionType, nil
	case "max_in_flight_requests":
		return kafkaEvent.MaxInFlightRequests, nil

	default:
		return kafkaEvent.Event.GetAttribute(attributeName)
	}
}

func (kafkaEvent *Kafka_Event) getPartitions() int32 {
	if kafkaEvent.Partitions <= 0 {
		return 1
	}
	return kafkaEvent.Partitions
}

func (kafkaEvent *Kafka_Event) getReplicationFactor() int16 {
	if kafkaEvent.ReplicationFactor <= 0 {
		return 1
	}
	return kafkaEvent.ReplicationFactor
}

func (kafkaEvent *Kafka_Event) getAutoOffsetReset() string {
	if kafkaEvent.AutoOffsetReset == "" {
		return "earliest"
	}
	return kafkaEvent.AutoOffsetReset
}

// applySecurityConfig applies security settings to the base configuration
func (kafkaEvent *Kafka_Event) applySecurityConfig(ctx context.Context, config kafka.ConfigMap) {
	if kafkaEvent.SecurityProtocol != "" {
		config["security.protocol"] = kafkaEvent.SecurityProtocol
	}

	// SASL Authentication
	if kafkaEvent.SaslMechanism != "" {
		config["sasl.mechanism"] = kafkaEvent.SaslMechanism

		// Username/Password (PLAIN, SCRAM)
		if kafkaEvent.SaslUsername != "" && kafkaEvent.SaslPassword != "" {
			config["sasl.username"] = kafkaEvent.SaslUsername
			config["sasl.password"] = kafkaEvent.SaslPassword
		}

		// Kerberos (GSSAPI)
		if kafkaEvent.SaslKerberosServiceName != "" {
			config["sasl.kerberos.service.name"] = kafkaEvent.SaslKerberosServiceName
		}
		if kafkaEvent.SaslKerberosPrincipal != "" {
			config["sasl.kerberos.principal"] = kafkaEvent.SaslKerberosPrincipal
		}
		if kafkaEvent.SaslKerberosKinitCmd != "" {
			config["sasl.kerberos.kinit.cmd"] = kafkaEvent.SaslKerberosKinitCmd
		}

		// OAuth
		if kafkaEvent.SaslOauthToken != "" {
			config["sasl.oauthbearer.token"] = kafkaEvent.SaslOauthToken
		}

		// AWS MSK IAM
		if kafkaEvent.SaslMechanism == "AWS_MSK_IAM" {
			if kafkaEvent.AwsRegion != "" {
				config["sasl.oauthbearer.config"] = fmt.Sprintf("region=%s", kafkaEvent.AwsRegion)
			}
			if kafkaEvent.AwsAccessKeyId != "" {
				config["sasl.oauthbearer.client.id"] = kafkaEvent.AwsAccessKeyId
			}
			if kafkaEvent.AwsSecretAccessKey != "" {
				config["sasl.oauthbearer.client.secret"] = kafkaEvent.AwsSecretAccessKey
			}
			if kafkaEvent.AwsSessionToken != "" {
				config["sasl.oauthbearer.token"] = kafkaEvent.AwsSessionToken
			}
		}
	}

	// SSL/TLS Configuration
	if kafkaEvent.SslCaLocation != "" {
		config["ssl.ca.location"] = kafkaEvent.SslCaLocation
	}
	if kafkaEvent.SslCertificateLocation != "" {
		config["ssl.certificate.location"] = kafkaEvent.SslCertificateLocation
	}
	if kafkaEvent.SslKeyLocation != "" {
		config["ssl.key.location"] = kafkaEvent.SslKeyLocation
	}
	if kafkaEvent.SslKeyPassword != "" {
		config["ssl.key.password"] = kafkaEvent.SslKeyPassword
	}
	if kafkaEvent.SslCrlLocation != "" {
		config["ssl.crl.location"] = kafkaEvent.SslCrlLocation
	}
	if kafkaEvent.SslKeystoreLocation != "" {
		config["ssl.keystore.location"] = kafkaEvent.SslKeystoreLocation
	}
	if kafkaEvent.SslKeystorePassword != "" {
		config["ssl.keystore.password"] = kafkaEvent.SslKeystorePassword
	}
	if kafkaEvent.SslTruststoreLocation != "" {
		config["ssl.truststore.location"] = kafkaEvent.SslTruststoreLocation
	}
	if kafkaEvent.SslTruststorePassword != "" {
		config["ssl.truststore.password"] = kafkaEvent.SslTruststorePassword
	}
}

// applyProducerConfig applies producer-specific settings
func (kafkaEvent *Kafka_Event) applyProducerConfig(ctx context.Context, config kafka.ConfigMap) {
	// Reliability settings
	if kafkaEvent.Acks != "" {
		config["acks"] = kafkaEvent.Acks
	} else {
		config["acks"] = "all" // Default to strongest durability
	}

	if kafkaEvent.Retries > 0 {
		config["retries"] = kafkaEvent.Retries
	} else {
		config["retries"] = 3 // Default retries
	}

	if kafkaEvent.RetryBackoffMs > 0 {
		config["retry.backoff.ms"] = kafkaEvent.RetryBackoffMs
	}

	if kafkaEvent.RequestTimeoutMs > 0 {
		config["request.timeout.ms"] = kafkaEvent.RequestTimeoutMs
	}

	if kafkaEvent.DeliveryTimeoutMs > 0 {
		config["delivery.timeout.ms"] = kafkaEvent.DeliveryTimeoutMs
	}

	// Performance settings
	if kafkaEvent.BatchSize > 0 {
		config["batch.size"] = kafkaEvent.BatchSize
	}

	if kafkaEvent.LingerMs >= 0 {
		config["linger.ms"] = kafkaEvent.LingerMs
	}

	if kafkaEvent.CompressionType != "" {
		config["compression.type"] = kafkaEvent.CompressionType
	} else {
		config["compression.type"] = "snappy" // Default compression
	}

	if kafkaEvent.MaxInFlightRequests > 0 {
		config["max.in.flight.requests.per.connection"] = kafkaEvent.MaxInFlightRequests
	}

	if kafkaEvent.BufferMemory > 0 {
		config["buffer.memory"] = kafkaEvent.BufferMemory
	}
}

// applyConsumerConfig applies consumer-specific settings
func (kafkaEvent *Kafka_Event) applyConsumerConfig(ctx context.Context, config kafka.ConfigMap) {
	if kafkaEvent.SessionTimeoutMs > 0 {
		config["session.timeout.ms"] = kafkaEvent.SessionTimeoutMs
	}

	if kafkaEvent.FetchMinBytes > 0 {
		config["fetch.min.bytes"] = kafkaEvent.FetchMinBytes
	}

	if kafkaEvent.FetchMaxWaitMs > 0 {
		config["fetch.max.wait.ms"] = kafkaEvent.FetchMaxWaitMs
	}

	if kafkaEvent.MaxPartitionFetchBytes > 0 {
		config["max.partition.fetch.bytes"] = kafkaEvent.MaxPartitionFetchBytes
	}

	if kafkaEvent.HeartbeatIntervalMs > 0 {
		config["heartbeat.interval.ms"] = kafkaEvent.HeartbeatIntervalMs
	}

	if kafkaEvent.MaxPollRecords > 0 {
		config["max.poll.records"] = kafkaEvent.MaxPollRecords
	}

	if kafkaEvent.IsolationLevel != "" {
		config["isolation.level"] = kafkaEvent.IsolationLevel
	}
} */
