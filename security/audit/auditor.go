// Package audit provides Plateau's CloudEvents audit runtime, configuration-driven
// Auditor construction, RPC middleware, AIP resource extraction, and redaction.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/proto"
)

// Auditor emits structured audit events enveloped as CloudEvents v1.0.
type Auditor interface {
	Emit(ctx context.Context, event cloudevents.Event) error
}

// BackendFunc constructs an Auditor for an external backend registered via WithBackend.
type BackendFunc func(cfg *auditpb.AuditEmitter, logger *slog.Logger) (Auditor, error)

// Option configures external backend constructors for New.
type Option func(map[string]BackendFunc)

// WithBackend registers an external backend constructor (such as kafka.Backend)
// resolved when AuditEmitter.backend matches name.
func WithBackend(name string, fn BackendFunc) Option {
	return func(m map[string]BackendFunc) {
		if key := strings.ToLower(strings.TrimSpace(name)); key != "" && fn != nil {
			m[key] = fn
		}
	}
}

// New constructs an Auditor from Plateau's AuditEmitter configuration section.
// Built-in stdlib backends ("noop", "stdout", "log") work out of the box;
// external broker backends are plugged in via Options (e.g. kafka.Backend(client)).
func New(cfg *auditpb.AuditEmitter, logger *slog.Logger, opts ...Option) (Auditor, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return Noop(), nil
	}
	config := proto.Clone(cfg).(*auditpb.AuditEmitter)
	if err := config.Apply(); err != nil {
		return nil, err
	}

	backends := make(map[string]BackendFunc, len(opts))
	for _, opt := range opts {
		if opt != nil {
			opt(backends)
		}
	}

	backend := strings.ToLower(strings.TrimSpace(config.GetBackend()))
	if fn, ok := backends[backend]; ok {
		return fn(config, logger)
	}
	switch backend {
	case "", "noop":
		return Noop(), nil
	case "stdout":
		return NewStdout(os.Stdout), nil
	case "log":
		return NewLog(logger), nil
	default:
		return nil, fmt.Errorf("audit: unsupported backend %q", config.GetBackend())
	}
}

type noopAuditor struct{}

// Noop returns an Auditor that discards all events without error.
func Noop() Auditor { return noopAuditor{} }

func (noopAuditor) Emit(context.Context, cloudevents.Event) error { return nil }

type stdoutAuditor struct{ w io.Writer }

// NewStdout returns an Auditor that writes newline-delimited CloudEvents JSON to w
// (defaulting to os.Stdout when w is nil).
func NewStdout(w io.Writer) Auditor {
	if w == nil {
		w = os.Stdout
	}
	return stdoutAuditor{w: w}
}

func (a stdoutAuditor) Emit(_ context.Context, event cloudevents.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("stdout auditor: marshal event: %w", err)
	}
	data = append(data, '\n')
	if _, err := a.w.Write(data); err != nil {
		return fmt.Errorf("stdout auditor: write event: %w", err)
	}
	return nil
}

type logAuditor struct{ logger *slog.Logger }

// NewLog returns an Auditor that emits structured CloudEvents records via slog.
func NewLog(l *slog.Logger) Auditor {
	if l == nil {
		l = slog.Default()
	}
	return logAuditor{logger: l}
}

func (a logAuditor) Emit(ctx context.Context, event cloudevents.Event) error {
	attrs := []any{
		slog.String("id", event.ID()),
		slog.String("type", event.Type()),
		slog.String("source", event.Source()),
		slog.String("subject", event.Subject()),
		slog.Time("time", event.Time()),
	}
	exts := event.Extensions()
	keys := make([]string, 0, len(exts))
	for k := range exts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		attrs = append(attrs, slog.Any(k, exts[k]))
	}
	a.logger.InfoContext(ctx, "audit_event", slog.Group("cloudevent", attrs...))
	return nil
}

type multiAuditor []Auditor

// Multi returns an Auditor that fans out events across all non-nil auditors.
func Multi(auditors ...Auditor) Auditor {
	out := make(multiAuditor, 0, len(auditors))
	for _, a := range auditors {
		if a != nil {
			out = append(out, a)
		}
	}
	return out
}

func (m multiAuditor) Emit(ctx context.Context, event cloudevents.Event) error {
	var errs []error
	for _, a := range m {
		if err := a.Emit(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
