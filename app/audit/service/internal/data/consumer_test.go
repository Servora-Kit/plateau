package data

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	auditkafka "github.com/Servora-Kit/servora/obs/audit/kafka"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestConsumerHandleDecodesCloudEventsKafkaRecords(t *testing.T) {
	tests := []struct {
		name string
		mode auditkafka.Mode
	}{
		{name: "binary", mode: auditkafka.BinaryMode},
		{name: "structured-json", mode: auditkafka.StructuredJSONMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := NewBatchWriter(&Data{}, nil, nil, testLogger())
			consumer := &Consumer{
				writer: writer,
				log:    testLogger(),
			}
			record, err := auditkafka.EncodeRecord(defaultTopic, testCloudEvent(t), tt.mode, nil)
			if err != nil {
				t.Fatalf("EncodeRecord() error = %v", err)
			}

			if err := consumer.handle(context.Background(), record); err != nil {
				t.Fatalf("handle() error = %v", err)
			}

			writer.mu.Lock()
			defer writer.mu.Unlock()
			if len(writer.buffer) != 1 {
				t.Fatalf("buffer length = %d, want 1", len(writer.buffer))
			}
			if got := writer.buffer[0].event.ID(); got != "event-1" {
				t.Fatalf("event ID = %q, want event-1", got)
			}
			if writer.buffer[0].record != record {
				t.Fatal("buffered record does not match consumed record")
			}
		})
	}
}

func testCloudEvent(t *testing.T) cloudevents.Event {
	t.Helper()
	ev := cloudevents.NewEvent()
	ev.SetID("event-1")
	ev.SetType("servora.audit.test")
	ev.SetSource("/audit.Service/Test")
	ev.SetTime(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	if err := ev.SetData("application/json", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	return ev
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
