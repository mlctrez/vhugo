package hlog

import (
	"fmt"
	"log"
	"time"

	"github.com/mlctrez/web"
)

type HLog struct {
	logger   *log.Logger
	prefixes []interface{}
}

func New(logger *log.Logger, prefixes ...interface{}) *HLog {
	if len(prefixes) == 1 {
		if s, ok := prefixes[0].(string); ok {
			prefixes = []interface{}{fmt.Sprintf("%-15s", s)}
		}
	}
	return &HLog{logger: logger, prefixes: prefixes}
}

func (h *HLog) Println(v ...interface{}) {
	h.logger.Println(append(h.prefixes, v...)...)
}

func (h *HLog) LoggerMiddleware(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	startTime := time.Now()

	next(rw, req)

	duration := time.Since(startTime).Nanoseconds()
	var durationUnits string
	switch {
	case duration > 2000000:
		durationUnits = "ms"
		duration /= 1000000
	case duration > 1000:
		durationUnits = "μs"
		duration /= 1000
	default:
		durationUnits = "ns"
	}

	h.Println(fmt.Sprintf("[%04d %2s] %d '%s'", duration, durationUnits, rw.StatusCode(), req.URL.Path))
}
