package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/auth"
)

type fakeMetrics struct {
	statusCounts      map[string]int64
	failureCounts     map[string]int64
	providerHits      map[string]int64
	providerMisses    map[string]int64
	instrumentalCount int64
	statusErr         error
	failureErr        error
	hitsErr           error
	missesErr         error
	instrumentalErr   error
}

func (f *fakeMetrics) CountByStatus(_ context.Context) (map[string]int64, error) {
	return f.statusCounts, f.statusErr
}

func (f *fakeMetrics) CountFailuresByReason(_ context.Context) (map[string]int64, error) {
	return f.failureCounts, f.failureErr
}

func (f *fakeMetrics) ProviderHits(_ context.Context) (map[string]int64, error) {
	return f.providerHits, f.hitsErr
}

func (f *fakeMetrics) ProviderMisses(_ context.Context) (map[string]int64, error) {
	return f.providerMisses, f.missesErr
}

func (f *fakeMetrics) CountInstrumental(_ context.Context) (int64, error) {
	return f.instrumentalCount, f.instrumentalErr
}

func TestMetricsRequiresAdminAuth(t *testing.T) {
	t.Run("no api key returns 401", func(t *testing.T) {
		h := NewHandler(&fakeAuth{err: auth.ErrInvalidKey}, &fakeQueue{}, "lyrics",
			WithMetricsReporter(&fakeMetrics{}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=bad", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
	})

	t.Run("webhook-scoped key returns 403", func(t *testing.T) {
		h := NewHandler(&fakeAuth{err: auth.ErrForbiddenScope}, &fakeQueue{}, "lyrics",
			WithMetricsReporter(&fakeMetrics{}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=webhook-key", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d; want 403", rec.Code)
		}
	})

	t.Run("auth backend error returns 500", func(t *testing.T) {
		h := NewHandler(&fakeAuth{err: errors.New("auth store down")}, &fakeQueue{}, "lyrics",
			WithMetricsReporter(&fakeMetrics{}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d; want 500", rec.Code)
		}
	})

	t.Run("valid admin key passes auth gate", func(t *testing.T) {
		a := &fakeAuth{}
		h := NewHandler(a, &fakeQueue{}, "lyrics",
			WithMetricsReporter(&fakeMetrics{
				statusCounts:  map[string]int64{},
				failureCounts: map[string]int64{},
			}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=admin", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200 (body %q)", rec.Code, rec.Body.String())
		}
		if a.required != auth.ScopeAdmin {
			t.Fatalf("required scope = %q; want admin", a.required)
		}
	})
}

func TestMetricsWithoutReporterReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics") // no WithMetricsReporter
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 when no reporter configured", rec.Code)
	}
}

func TestMetricsResponseIsValidPrometheusFormat(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:  map[string]int64{"pending": 5, "done": 12, "failed": 2},
			failureCounts: map[string]int64{"connection refused": 2},
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body %q)", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("Content-Type = %q; want text/plain", ct)
	}

	cc := rec.Header().Get("Cache-Control")
	if cc != "no-store" {
		t.Fatalf("Cache-Control = %q; want no-store", cc)
	}

	body := rec.Body.String()

	// Metric family: queue items.
	if !strings.Contains(body, "# HELP mxlrcgo_queue_items") {
		t.Errorf("missing HELP line for mxlrcgo_queue_items\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_queue_items gauge") {
		t.Errorf("missing TYPE gauge line for mxlrcgo_queue_items\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_queue_items{status="pending"} 5`) {
		t.Errorf("missing pending sample\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_queue_items{status="done"} 12`) {
		t.Errorf("missing done sample\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_queue_items{status="failed"} 2`) {
		t.Errorf("missing failed sample\nbody:\n%s", body)
	}

	// Metric family: failures.
	if !strings.Contains(body, "# HELP mxlrcgo_queue_failures") {
		t.Errorf("missing HELP line for mxlrcgo_queue_failures\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_queue_failures gauge") {
		t.Errorf("missing TYPE gauge line for mxlrcgo_queue_failures\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_queue_failures{reason="connection refused"} 2`) {
		t.Errorf("missing failure sample\nbody:\n%s", body)
	}
}

func TestMetricsEmptyQueueProducesHelpAndTypeLines(t *testing.T) {
	// No items in the queue: HELP/TYPE lines must still appear, but no samples.
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:  map[string]int64{},
			failureCounts: map[string]int64{},
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "# HELP mxlrcgo_queue_items") {
		t.Errorf("missing HELP for mxlrcgo_queue_items\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_queue_items gauge") {
		t.Errorf("missing TYPE for mxlrcgo_queue_items\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# HELP mxlrcgo_queue_failures") {
		t.Errorf("missing HELP for mxlrcgo_queue_failures\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_queue_failures gauge") {
		t.Errorf("missing TYPE for mxlrcgo_queue_failures\nbody:\n%s", body)
	}
}

func TestMetricsStatusQueryErrorReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{statusErr: errors.New("db error")}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 on query error", rec.Code)
	}
}

func TestMetricsFailureQueryErrorReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts: map[string]int64{"pending": 1},
			failureErr:   errors.New("db error"),
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 on query error", rec.Code)
	}
}

func TestWriteMetricsLabelEscaping(t *testing.T) {
	// Verify promEscape handles the characters mandated by the Prometheus spec.
	cases := []struct {
		input string
		want  string
	}{
		{`normal`, `normal`},
		{`has "quotes"`, `has \"quotes\"`},
		{`back\slash`, `back\\slash`},
		{"new\nline", `new\nline`},
	}
	for _, tc := range cases {
		got := promEscape(tc.input)
		if got != tc.want {
			t.Errorf("promEscape(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestWriteMetricsSortedOutput(t *testing.T) {
	// Samples must appear in lexicographic order regardless of map iteration.
	var sb strings.Builder
	writeMetrics(&sb,
		map[string]int64{"pending": 1, "done": 2, "failed": 3},
		map[string]int64{},
		map[string]int64{},
		map[string]int64{},
		0,
	)
	body := sb.String()

	doneIdx := strings.Index(body, `status="done"`)
	failedIdx := strings.Index(body, `status="failed"`)
	pendingIdx := strings.Index(body, `status="pending"`)

	if doneIdx < 0 || failedIdx < 0 || pendingIdx < 0 {
		t.Fatalf("missing sample lines\nbody:\n%s", body)
	}
	if doneIdx >= failedIdx || failedIdx >= pendingIdx {
		t.Errorf("samples not in sorted order (done=%d failed=%d pending=%d)\nbody:\n%s",
			doneIdx, failedIdx, pendingIdx, body)
	}
}

func TestMetricsProviderOutcomesAndInstrumental(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:      map[string]int64{"done": 10},
			failureCounts:     map[string]int64{},
			providerHits:      map[string]int64{"musixmatch": 8, "petitlyrics": 2},
			providerMisses:    map[string]int64{"musixmatch": 3},
			instrumentalCount: 5,
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body %q)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// Provider hits counter family.
	if !strings.Contains(body, "# HELP mxlrcgo_provider_hits_total") {
		t.Errorf("missing HELP for mxlrcgo_provider_hits_total\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_provider_hits_total counter") {
		t.Errorf("missing TYPE counter for mxlrcgo_provider_hits_total\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_provider_hits_total{lane="musixmatch"} 8`) {
		t.Errorf("missing musixmatch hit sample\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_provider_hits_total{lane="petitlyrics"} 2`) {
		t.Errorf("missing petitlyrics hit sample\nbody:\n%s", body)
	}

	// Provider misses counter family.
	if !strings.Contains(body, "# HELP mxlrcgo_provider_misses_total") {
		t.Errorf("missing HELP for mxlrcgo_provider_misses_total\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_provider_misses_total counter") {
		t.Errorf("missing TYPE counter for mxlrcgo_provider_misses_total\nbody:\n%s", body)
	}
	if !strings.Contains(body, `mxlrcgo_provider_misses_total{lane="musixmatch"} 3`) {
		t.Errorf("missing musixmatch miss sample\nbody:\n%s", body)
	}

	// Instrumental gauge.
	if !strings.Contains(body, "# HELP mxlrcgo_instrumental_tracks") {
		t.Errorf("missing HELP for mxlrcgo_instrumental_tracks\nbody:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE mxlrcgo_instrumental_tracks gauge") {
		t.Errorf("missing TYPE gauge for mxlrcgo_instrumental_tracks\nbody:\n%s", body)
	}
	if !strings.Contains(body, "mxlrcgo_instrumental_tracks 5") {
		t.Errorf("missing instrumental_tracks sample\nbody:\n%s", body)
	}

	// Sorted output within provider hits (musixmatch < petitlyrics).
	mmIdx := strings.Index(body, `lane="musixmatch"`)
	ptIdx := strings.Index(body, `lane="petitlyrics"`)
	if mmIdx < 0 || ptIdx < 0 {
		t.Fatalf("missing lane samples\nbody:\n%s", body)
	}
	if mmIdx >= ptIdx {
		t.Errorf("provider hits not in sorted lane order (musixmatch=%d petitlyrics=%d)\nbody:\n%s",
			mmIdx, ptIdx, body)
	}
}

func TestMetricsEmptyProviderTablesProduceHelpAndTypeLines(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:   map[string]int64{},
			failureCounts:  map[string]int64{},
			providerHits:   map[string]int64{},
			providerMisses: map[string]int64{},
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# HELP mxlrcgo_provider_hits_total",
		"# TYPE mxlrcgo_provider_hits_total counter",
		"# HELP mxlrcgo_provider_misses_total",
		"# TYPE mxlrcgo_provider_misses_total counter",
		"# HELP mxlrcgo_instrumental_tracks",
		"# TYPE mxlrcgo_instrumental_tracks gauge",
		"mxlrcgo_instrumental_tracks 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestMetricsProviderHitsQueryErrorReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:  map[string]int64{},
			failureCounts: map[string]int64{},
			hitsErr:       errors.New("db error"),
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 on provider hits query error", rec.Code)
	}
}

func TestMetricsProviderMissesQueryErrorReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:  map[string]int64{},
			failureCounts: map[string]int64{},
			providerHits:  map[string]int64{},
			missesErr:     errors.New("db error"),
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 on provider misses query error", rec.Code)
	}
}

func TestMetricsInstrumentalQueryErrorReturns500(t *testing.T) {
	h := NewHandler(&fakeAuth{}, &fakeQueue{}, "lyrics",
		WithMetricsReporter(&fakeMetrics{
			statusCounts:    map[string]int64{},
			failureCounts:   map[string]int64{},
			providerHits:    map[string]int64{},
			providerMisses:  map[string]int64{},
			instrumentalErr: errors.New("db error"),
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics?apikey=key", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 on instrumental query error", rec.Code)
	}
}
