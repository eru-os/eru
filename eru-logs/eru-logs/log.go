package eru_logs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const loggerKey contextKey = "ctxlog"

var Logger *zap.Logger

//var FileLogger *zap.Logger

func LogInit(serviceName string, instanceId string) {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	initialFields := fmt.Sprintf(`{"service": "%s", "instance_id": "%s"}`, serviceName, instanceId)

	logConfig := []byte(fmt.Sprint(`{
       "level" :"`, logLevel, `",
       "encoding": "json",
       "outputPaths":["stdout"],
       "errorOutputPaths":["stderr"],
 	   "initialFields": `, initialFields, `,
       "encoderConfig": {
           "messageKey":"msg",
           "levelKey":"level",
           "timeKey":"ts",
           "callerKey":"src",
           "levelEncoder":"lowercase"
       }
   }`))

	var zapConfig zap.Config

	if err := json.Unmarshal(logConfig, &zapConfig); err != nil {
		panic(err)
	}
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapConfig.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var err error
	Logger, err = zapConfig.Build()
	if err != nil {
		panic(err)
	}

	// Wrap the logger with our error limiting core
	Logger = WithErrorLimit(Logger)

	undo := zap.ReplaceGlobals(Logger)
	defer undo()

	defer Logger.Sync()

	/*fileLogConfig := []byte(fmt.Sprint(`{
	       "level" :"`, logLevel, `",
	       "encoding": "json",
	       "outputPaths":["stdout","func_profile.log"],
	       "errorOutputPaths":["stderr"],
	 	   "initialFields": {"service": "`, serviceName, `"},
	       "encoderConfig": {
	           "messageKey":"msg",
	           "timeKey":"ts"
	       }
	   }`))

		var fileZapConfig zap.Config

		if err := json.Unmarshal(fileLogConfig, &fileZapConfig); err != nil {
			panic(err)
		}
		fileZapConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
		fileZapConfig.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		var fileErr error
		FileLogger, fileErr = fileZapConfig.Build()
		if fileErr != nil {
			panic(fileErr)
		}
		fileundo := zap.ReplaceGlobals(FileLogger)
		defer fileundo()

		defer Logger.Sync()
		defer FileLogger.Sync()

	*/

}

func NewContext(ctx context.Context, fields ...zap.Field) context.Context {
	return context.WithValue(ctx, loggerKey, WithContext(ctx).With(fields...))
}

func WithContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return Logger
	}
	if ctxLogger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return ctxLogger
	} else {
		return Logger
	}
}

// limitErrorMsg limits error messages to 1000 characters
func limitErrorMsg(msg string) string {
	if len(msg) > 1000 {
		return msg[:1000]
	}
	return msg
}

// ErrorWithLimit logs an error message with a 1000 character limit
func ErrorWithLimit(logger *zap.Logger, msg string, fields ...zap.Field) {
	logger.Error(limitErrorMsg(msg), fields...)
}

// WithErrorLimit returns a logger that limits error messages to 1000 characters
func WithErrorLimit(logger *zap.Logger) *zap.Logger {
	return logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return &errorLimitCore{Core: core}
	}))
}

// errorLimitCore wraps a zapcore.Core to limit error message length
type errorLimitCore struct {
	zapcore.Core
}

func (c *errorLimitCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if ent.Level == zapcore.ErrorLevel {
		ent.Message = limitErrorMsg(ent.Message)
	}
	return c.Core.Write(ent, fields)
}

func (c *errorLimitCore) With(fields []zapcore.Field) zapcore.Core {
	return &errorLimitCore{Core: c.Core.With(fields)}
}

func Err(ctx context.Context, orgErr error, errMsg string) (err error) {
	errCode := uuid.New().String()
	if errMsg == "" {
		errMsg = "something went wrong - please contact support"
	}
	WithContext(ctx).Error(fmt.Sprintf("Error Code : %s, Error : %s", errCode, orgErr.Error()))
	return fmt.Errorf("error code : %s, error : %s", errCode, errMsg)
}
