package handlers

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"gopkg.in/errgo.v1"

	"github.com/codegangsta/negroni"
	"github.com/sirupsen/logrus"
)

var (
	loggerFuncMap = map[logrus.Level]func(logrus.FieldLogger, string, ...interface{}){
		logrus.DebugLevel: logrus.FieldLogger.Debugf,
		logrus.InfoLevel:  logrus.FieldLogger.Infof,
		logrus.WarnLevel:  logrus.FieldLogger.Warnf,
		logrus.ErrorLevel: logrus.FieldLogger.Errorf,
		logrus.FatalLevel: logrus.FieldLogger.Fatalf,
		logrus.PanicLevel: logrus.FieldLogger.Panicf,
	}
)

type patternInfo struct {
	re    *regexp.Regexp
	level logrus.Level
}

type LoggingMiddleware struct {
	logger  logrus.FieldLogger
	filters []patternInfo
}

func NewLoggingMiddleware(logger logrus.FieldLogger) Middleware {
	m := &LoggingMiddleware{logger: logger, filters: []patternInfo{}}
	return m
}

func NewLoggingMiddlewareWithFilters(logger logrus.FieldLogger, filters map[string]logrus.Level) (*LoggingMiddleware, error) {
	refilters := []patternInfo{}
	for pattern, level := range filters {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, errgo.Notef(err, "invalid regexp '%v'", pattern)
		}
		refilters = append(refilters, patternInfo{re: re, level: level})
	}
	m := &LoggingMiddleware{logger: logger, filters: refilters}
	return m, nil
}

func (l *LoggingMiddleware) Apply(next HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, vars map[string]string) error {
		logger := l.logger
		before := time.Now()

		id, ok := r.Context().Value("request_id").(string)
		if ok {
			logger = logger.WithField("request_id", id)
		}

		r = r.WithContext(context.WithValue(r.Context(), "logger", logger))

		fields := logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.String(),
			"host":       r.Host,
			"from":       r.RemoteAddr,
			"protocol":   r.Proto,
			"referer":    r.Referer(),
			"user_agent": r.UserAgent(),
		}
		for k, v := range fields {
			if len(v.(string)) == 0 {
				delete(fields, k)
			}
		}
		logger = logger.WithFields(fields)

		loglevel := logrus.InfoLevel
		for _, info := range l.filters {
			if info.re.MatchString(r.URL.Path) {
				loglevel = info.level
			}
		}
		loggerFuncMap[loglevel](logger, "starting request")

		rw := negroni.NewResponseWriter(w)
		err := next(rw, r, vars)
		after := time.Now()

		status := rw.Status()
		if status == 0 {
			status = 200
		}

		logger = logger.WithFields(logrus.Fields{
			"status":   status,
			"duration": after.Sub(before).Seconds(),
			"bytes":    rw.Size(),
		})
		loggerFuncMap[loglevel](logger, "request completed")

		return err
	}
}
