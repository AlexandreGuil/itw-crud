package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/observability"
)

func metricsMiddleware(m *observability.Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			endpoint := chiRouteOrFallback(r)
			labels := []string{r.Method, endpoint, strconv.Itoa(ww.Status())}
			m.RequestsTotal.WithLabelValues(labels...).Inc()
			m.RequestDuration.WithLabelValues(r.Method, endpoint).Observe(time.Since(start).Seconds())
			if ww.Status() == http.StatusPreconditionFailed {
				m.ConcurrencyConflicts.Inc()
			}
		})
	}
}

func chiRouteOrFallback(r *http.Request) string {
	rc := r.URL.Path
	switch {
	case len(rc) > 9 && rc[:10] == "/articles/" && rc != "/articles/orphans":
		return "/articles/{url_b64}"
	case len(rc) > 19 && rc[:20] == "/translation-state/":
		return "/translation-state/{url_b64}"
	default:
		return rc
	}
}
