package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/eru-os/eru/eru-events/events"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

func main() {
	logs.LogInit("eru-events-kafka-test", "eru-events-kafka-test-123")

	fmt.Println("🚀 Starting Kafka Event Test...")

	testKafkaEvent()
}

func testKafkaEvent() {
	ctx := context.Background()

	// Test configuration
	kafkaConfig := map[string]interface{}{
		"event_type":         "KAFKA",
		"event_name":         "eru-test-topic",
		"polling_interval":   5,
		"brokers":            getBrokers(),
		"topic":              "eru-test-topic",
		"group_id":           "eru-test-group",
		"partitions":         3,
		"replication_factor": 1,
		"auto_offset_reset":  "earliest",
		"enable_auto_commit": true,
	}

	// Convert to JSON
	configJSON, err := json.Marshal(kafkaConfig)
	if err != nil {
		log.Fatalf("❌ Failed to marshal config: %v", err)
	}

	var rawMsg json.RawMessage = configJSON

	// Create Kafka event
	fmt.Println("📝 Creating Kafka event...")
	kafkaEvent := events.GetEvent("KAFKA")
	if kafkaEvent == nil {
		log.Fatal("❌ Failed to create Kafka event")
	}

	// Initialize from JSON
	err = kafkaEvent.MakeFromJson(ctx, &rawMsg)
	if err != nil {
		log.Fatalf("❌ Failed to initialize from JSON: %v", err)
	}
	fmt.Println("✅ Kafka event created successfully")

	// Test attribute access
	fmt.Println("🔍 Testing attribute access...")
	testAttributes(kafkaEvent)

	// Initialize Kafka connection
	fmt.Println("🔌 Initializing Kafka connection...")
	err = kafkaEvent.Init(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to initialize Kafka: %v", err)
	}
	fmt.Println("✅ Kafka connection initialized")

	// Create topic
	fmt.Println("📂 Creating Kafka topic...")
	err = kafkaEvent.CreateEvent(ctx)
	if err != nil {
		log.Printf("⚠️  Topic creation failed (may already exist): %v", err)
	} else {
		fmt.Println("✅ Topic created successfully")
	}

	// Test publishing messages
	fmt.Println("📤 Testing message publishing...")
	testPublishing(ctx, kafkaEvent)

	// Test cloning
	fmt.Println("🔄 Testing event cloning...")
	testCloning(ctx, kafkaEvent)

	// Test polling messages
	fmt.Println("📥 Testing message polling...")
	testPolling(ctx, kafkaEvent)

	fmt.Println("🎉 All tests completed successfully!")
}

func testAttributes(event events.EventI) {
	attributes := []string{
		"event_type", "event_name", "polling_interval",
		"brokers", "topic", "group_id",
	}

	for _, attr := range attributes {
		value, err := event.GetAttribute(attr)
		if err != nil {
			log.Printf("⚠️  Failed to get attribute %s: %v", attr, err)
		} else {
			fmt.Printf("   %s: %v\n", attr, value)
		}
	}
}

func testPublishing(ctx context.Context, event events.EventI) {
	messages := []map[string]interface{}{
		{
			"id":        "msg-001",
			"type":      "test",
			"message":   "Hello Kafka from Eru Events!",
			"timestamp": time.Now().Unix(),
		},
		{
			"id":        "msg-002",
			"type":      "test",
			"message":   "Second test message",
			"timestamp": time.Now().Unix(),
		},
		{
			"id":        "msg-003",
			"type":      "test",
			"message":   "Third test message with more data",
			"data":      map[string]string{"key1": "value1", "key2": "value2"},
			"timestamp": time.Now().Unix(),
		},
	}

	for i, msg := range messages {
		msgId, err := event.Publish(ctx, msg, event)
		if err != nil {
			log.Printf("❌ Failed to publish message %d: %v", i+1, err)
		} else {
			fmt.Printf("   ✅ Published message %d with ID: %s\n", i+1, msgId)
		}
		time.Sleep(100 * time.Millisecond) // Small delay between messages
	}
}

func testCloning(ctx context.Context, event events.EventI) {
	clonedEvent, err := event.Clone(ctx)
	if err != nil {
		log.Printf("❌ Failed to clone event: %v", err)
		return
	}

	// Test that cloned event has same attributes
	originalBrokers, _ := event.GetAttribute("brokers")
	clonedBrokers, _ := clonedEvent.GetAttribute("brokers")

	if originalBrokers == clonedBrokers {
		fmt.Println("   ✅ Event cloned successfully")
	} else {
		fmt.Printf("   ❌ Clone mismatch: original=%v, clone=%v\n", originalBrokers, clonedBrokers)
	}
}

func testPolling(ctx context.Context, event events.EventI) {
	fmt.Println("   Polling for messages (will wait up to 10 seconds)...")

	// Poll for messages with timeout
	for i := 0; i < 3; i++ {
		eventMsgs, err := event.Poll(ctx)
		if err != nil {
			log.Printf("❌ Failed to poll messages: %v", err)
			return
		}

		if len(eventMsgs) > 0 {
			for j, msg := range eventMsgs {
				fmt.Printf("   📨 Received message %d: %s\n", j+1, msg.Msg)

				// Acknowledge message (delete it)
				err := event.DeleteMessage(ctx, msg.MsgIdentifer)
				if err != nil {
					log.Printf("⚠️  Failed to acknowledge message: %v", err)
				}
			}
			break
		} else {
			fmt.Printf("   ⏳ No messages received in poll %d, waiting...\n", i+1)
			time.Sleep(2 * time.Second)
		}
	}
}

func getBrokers() string {
	// Always use localhost:9092 for external testing
	// This connects to the Kafka broker exposed on the host machine
	return "localhost:9092"
}
