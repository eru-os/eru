//go:build !cgo
// +build !cgo

package events

/* import (
	"context"
	"encoding/json"
	"errors"

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
}

func (kafkaEvent *Kafka_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
}

func (kafkaEvent *Kafka_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
}

func (kafkaEvent *Kafka_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
}

func (kafkaEvent *Kafka_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("Kafka MakeFromJson - Start (CGO disabled)")
	err := json.Unmarshal(*rj, &kafkaEvent)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (kafkaEvent *Kafka_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return "", errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
}

func (kafkaEvent *Kafka_Event) Poll(ctx context.Context) (eventMsgs []EventMsg, err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return nil, errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
}

func (kafkaEvent *Kafka_Event) DeleteMessage(ctx context.Context, msgIdentifier string) (err error) {
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
	return errors.New("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
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
	logs.WithContext(ctx).Error("Kafka is not available when CGO is disabled. Please use DB event type instead or build with CGO_ENABLED=1")
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
} */
