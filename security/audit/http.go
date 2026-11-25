package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.opentelemetry.io/otel/trace"
)

type httpConfig struct {
	onlyErrors bool
	postHooks  []func(ctx context.Context, r *http.Request, statusCode int)
}

// HTTPOption customizes HTTPHandler behavior.
type HTTPOption func(*httpConfig)

// WithHTTPOnlyErrors suppresses event emission when the HTTP response succeeds
// (status < 400 and SetError was not called).
func WithHTTPOnlyErrors() HTTPOption {
	return func(c *httpConfig) {
		c.onlyErrors = true
	}
}

// WithHTTPPostHook registers a callback executed after the wrapped http.Handler
// returns and before the audit event is finalized. Hooks may call SetDetail,
// SetError, SetActor, SetResource, SetEventOverride, or Skip on ctx.
func WithHTTPPostHook(fn func(ctx context.Context, r *http.Request, statusCode int)) HTTPOption {
	return func(c *httpConfig) {
		if fn != nil {
			c.postHooks = append(c.postHooks, fn)
		}
	}
}

// HTTPHandler wraps a native http.Handler to emit CloudEvents control-plane
// audit events while keeping protocol handlers decoupled from Auditor wiring.
func HTTPHandler(auditor Auditor, eventType, category string, opts ...HTTPOption) func(http.Handler) http.Handler {
	if auditor == nil {
		panic("audit: auditor is nil")
	}
	var cfg httpConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("audit: http handler is nil")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
				if sc, ok := remoteSpanContextFromHeader(r.Header); ok {
					ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
				}
			}

			slot := &requestSlot{}
			enrichedCtx := context.WithValue(ctx, slotContextKey{}, slot)
			req := r.WithContext(enrichedCtx)
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			completed := false
			defer func() {
				aborted := !completed
				statusCode := rec.status
				if aborted {
					statusCode = http.StatusInternalServerError
				}

				for _, hook := range cfg.postHooks {
					hook(enrichedCtx, req, statusCode)
				}

				slot.mu.Lock()
				skip := slot.skip
				hasActor := slot.hasActor
				actor := slot.actor
				resType := slot.resourceType
				resName := slot.resourceName
				hasError := slot.hasError
				errCode := slot.errorCode
				errMsg := slot.errorMessage
				evTypeOverride := slot.eventType
				catOverride := slot.category
				var detailSnapshot map[string]any
				if len(slot.detail) > 0 {
					detailSnapshot = maps.Clone(slot.detail)
				}
				slot.mu.Unlock()

				if skip && !aborted {
					return
				}

				failed := aborted || statusCode >= http.StatusBadRequest || hasError
				if cfg.onlyErrors && !failed {
					return
				}

				finalType := eventType
				if evTypeOverride != "" {
					finalType = evTypeOverride
				}
				finalCategory := category
				if catOverride != "" {
					finalCategory = catOverride
				}
				subject := req.Method + " " + requestPath(req)

				evOpts := []EventOption{
					WithType(finalType),
					WithSubject(subject),
					WithCategory(finalCategory),
					WithHTTPRequest(req),
				}
				if hasActor {
					evOpts = append(evOpts, WithActor(actor))
				}
				evOpts = append(evOpts, WithResource(resType, resName))

				if failed {
					if errCode == "" {
						errCode = httpStatusErrorCode(statusCode)
					}
					if errMsg == "" && statusCode >= http.StatusBadRequest {
						errMsg = httpStatusSafeMessage(statusCode)
					}
					evOpts = append(evOpts, func(e *cloudevents.Event) {
						e.SetExtension(ExtOutcome, OutcomeFailure)
						e.SetExtension(ExtErrorCode, errCode)
						if errMsg != "" {
							e.SetExtension(ExtErrorMessage, errMsg)
						}
					})
				} else {
					evOpts = append(evOpts, WithOutcome(OutcomeSuccess))
				}

				event := NewEvent(enrichedCtx, evOpts...)
				if len(detailSnapshot) > 0 {
					raw, marshalErr := json.Marshal(detailSnapshot)
					if marshalErr != nil {
						slog.WarnContext(enrichedCtx, "audit marshal http detail failed",
							slog.String("operation", subject),
							slog.String("error", marshalErr.Error()),
						)
					} else if dataErr := event.SetData(cloudevents.ApplicationJSON, raw); dataErr != nil {
						slog.WarnContext(enrichedCtx, "audit set http detail data failed",
							slog.String("operation", subject),
							slog.String("error", dataErr.Error()),
						)
					}
				}

				if emitErr := auditor.Emit(enrichedCtx, event); emitErr != nil {
					slog.ErrorContext(enrichedCtx, "audit emit failed",
						slog.String("operation", subject),
						slog.String("error", emitErr.Error()),
					)
				}
			}()

			next.ServeHTTP(rec, req)
			completed = true
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	if !r.wroteHeader {
		r.status = statusCode
		r.wroteHeader = true
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func requestPath(r *http.Request) string {
	if r != nil && r.URL != nil && r.URL.Path != "" {
		return r.URL.Path
	}
	return "/"
}

func httpStatusErrorCode(statusCode int) string {
	if statusCode < http.StatusBadRequest {
		return "ERROR"
	}
	if text := http.StatusText(statusCode); text != "" {
		return strings.ReplaceAll(strings.ToUpper(text), " ", "_")
	}
	return fmt.Sprintf("HTTP_%d", statusCode)
}

func httpStatusSafeMessage(statusCode int) string {
	if statusCode >= http.StatusInternalServerError {
		return "internal server error"
	}
	if text := http.StatusText(statusCode); text != "" {
		return strings.ToLower(text)
	}
	return "request failed"
}

func remoteSpanContextFromHeader(hdr http.Header) (trace.SpanContext, bool) {
	if hdr == nil {
		return trace.SpanContext{}, false
	}
	tp := strings.ToLower(strings.TrimSpace(hdr.Get("Traceparent")))
	if len(tp) != 55 || tp[2] != '-' || tp[35] != '-' || tp[52] != '-' || tp[:2] == "ff" {
		return trace.SpanContext{}, false
	}
	if _, err := strconv.ParseUint(tp[:2], 16, 8); err != nil {
		return trace.SpanContext{}, false
	}
	tid, err := trace.TraceIDFromHex(tp[3:35])
	if err != nil {
		return trace.SpanContext{}, false
	}
	sid, err := trace.SpanIDFromHex(tp[36:52])
	if err != nil {
		return trace.SpanContext{}, false
	}
	flags, err := strconv.ParseUint(tp[53:55], 16, 8)
	if err != nil {
		return trace.SpanContext{}, false
	}
	cfg := trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.TraceFlags(flags),
		Remote:     true,
	}
	if rawTS := strings.TrimSpace(hdr.Get("Tracestate")); rawTS != "" {
		if ts, tsErr := trace.ParseTraceState(rawTS); tsErr == nil {
			cfg.TraceState = ts
		}
	}
	sc := trace.NewSpanContext(cfg)
	return sc, sc.IsValid()
}
