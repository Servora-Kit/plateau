package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	auditconfv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/audit/conf/v1"
	kafkapb "github.com/Servora-Kit/plateau/api/gen/go/plateau/infra/kafka/v1"
	"github.com/Servora-Kit/plateau/app/audit/service/internal/service"
	infrakafka "github.com/Servora-Kit/plateau/infra/kafka"
	auditkafka "github.com/Servora-Kit/plateau/security/audit/kafka"
	svrkafka "github.com/Servora-Kit/plateau/transport/server/kafka"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
)

const defaultKafkaProbeTimeout = 10 * time.Second

// NewKafkaServer verifies mandatory Kafka connectivity and constructs the inbound
// CloudEvents consumer server backed by transport/server/kafka.
func NewKafkaServer(
	kafkaCfg *kafkapb.Kafka,
	auditCfg *auditconfv1.Audit,
	events *service.EventService,
	l *slog.Logger,
) (*svrkafka.Server, error) {
	log := l.With("scope", "audit/server/kafka")

	cfg := &auditconfv1.Audit{}
	if auditCfg != nil {
		cfg = proto.Clone(auditCfg).(*auditconfv1.Audit)
	}
	if err := cfg.Apply(); err != nil {
		return nil, fmt.Errorf("audit kafka config: %w", err)
	}

	probeTimeout := defaultKafkaProbeTimeout
	if d := kafkaCfg.GetDialTimeout(); d != nil && d.AsDuration() > 0 {
		probeTimeout = d.AsDuration()
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	probe, err := infrakafka.NewClient(probeCtx, kafkaCfg, infrakafka.WithSlogLogger(log))
	if err != nil {
		return nil, fmt.Errorf("audit kafka connect: %w", err)
	}
	probe.Close()

	handleBatch := func(ctx context.Context, records []*kgo.Record) error {
		decoded := make([]cloudevents.Event, 0, len(records))
		for _, rec := range records {
			if rec == nil {
				continue
			}
			ev, decodeErr := auditkafka.DecodeRecord(rec)
			if decodeErr != nil || ev == nil {
				log.WarnContext(ctx, "dropping undecodable kafka audit record",
					"topic", rec.Topic,
					"partition", rec.Partition,
					"offset", rec.Offset,
					"err", decodeErr,
				)
				continue
			}
			decoded = append(decoded, *ev)
		}
		if len(decoded) == 0 {
			return nil
		}
		return events.IngestEvents(ctx, decoded)
	}

	opts := []svrkafka.ServerOption{
		svrkafka.WithLogger(log),
		svrkafka.WithBatchSize(int(cfg.GetBatchSize())),
		svrkafka.WithFlushInterval(cfg.GetFlushInterval().AsDuration()),
		svrkafka.WithTopicHandler(cfg.GetTopic(), handleBatch),
	}

	return svrkafka.NewServer(kafkaCfg, opts...)
}
