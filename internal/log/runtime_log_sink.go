package log

import (
	"context"
	"time"

	"github.com/samber/lo"
	"go.uber.org/zap/zapcore"
)

type RuntimeLogLevel string

const (
	RuntimeLogLevelDebug RuntimeLogLevel = "debug"
	RuntimeLogLevelInfo  RuntimeLogLevel = "info"
	RuntimeLogLevelWarn  RuntimeLogLevel = "warn"
	RuntimeLogLevelError RuntimeLogLevel = "error"
)

type RuntimeLogRecord struct {
	CreatedAt     time.Time
	Logger        string
	Level         RuntimeLogLevel
	Message       string
	Caller        string
	TraceID       string
	RequestID     string
	OperationName string
	ChannelID     *int
	ChannelName   string
	ModelID       string
	FieldsJSON    map[string]any
}

type RuntimeLogSink interface {
	WriteRuntimeLog(ctx context.Context, record RuntimeLogRecord)
}

type runtimeLogSinkContextKey struct{}

var runtimeLogSink RuntimeLogSink

func SetRuntimeLogSink(sink RuntimeLogSink) {
	runtimeLogSink = sink
}

func WithoutRuntimeLogSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, runtimeLogSinkContextKey{}, true)
}

func runtimeLogSinkDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	disabled, _ := ctx.Value(runtimeLogSinkContextKey{}).(bool)
	return disabled
}

func runtimeLogLevelFromZap(level zapcore.Level) RuntimeLogLevel {
	switch level {
	case DebugLevel:
		return RuntimeLogLevelDebug
	case InfoLevel:
		return RuntimeLogLevelInfo
	case WarnLevel:
		return RuntimeLogLevelWarn
	case ErrorLevel, zapcore.DPanicLevel, PanicLevel, FatalLevel:
		return RuntimeLogLevelError
	case zapcore.InvalidLevel:
		return RuntimeLogLevelInfo
	}

	return RuntimeLogLevelInfo
}

func publishRuntimeLogRecord(ctx context.Context, loggerName string, level zapcore.Level, msg, caller string, fields []Field) {
	if runtimeLogSink == nil || runtimeLogSinkDisabled(ctx) || level == DebugLevel {
		return
	}

	runtimeLogSink.WriteRuntimeLog(ctx, buildRuntimeLogRecord(time.Now().UTC(), loggerName, level, msg, caller, fields))
}

func buildRuntimeLogRecord(createdAt time.Time, loggerName string, level zapcore.Level, msg, caller string, fields []Field) RuntimeLogRecord {
	encoder := zapcore.NewMapObjectEncoder()
	addFields(encoder, fields)

	fieldsJSON := lo.Assign(map[string]any{}, encoder.Fields)
	record := RuntimeLogRecord{
		CreatedAt:  createdAt,
		Logger:     loggerName,
		Level:      runtimeLogLevelFromZap(level),
		Message:    msg,
		Caller:     caller,
		FieldsJSON: fieldsJSON,
	}

	if traceID, ok := fieldsJSON["trace_id"].(string); ok {
		record.TraceID = traceID
		delete(fieldsJSON, "trace_id")
	}
	if requestID, ok := fieldsJSON["request_id"].(string); ok {
		record.RequestID = requestID
		delete(fieldsJSON, "request_id")
	}
	if operationName, ok := fieldsJSON["operation_name"].(string); ok {
		record.OperationName = operationName
		delete(fieldsJSON, "operation_name")
	}
	if channelID, ok := fieldsJSON["channel_id"].(int64); ok {
		record.ChannelID = lo.ToPtr(int(channelID))
		delete(fieldsJSON, "channel_id")
	} else if channelID, ok := fieldsJSON["channel_id"].(int); ok {
		record.ChannelID = lo.ToPtr(channelID)
		delete(fieldsJSON, "channel_id")
	}
	if channelName, ok := fieldsJSON["channel_name"].(string); ok {
		record.ChannelName = channelName
		delete(fieldsJSON, "channel_name")
	}
	if modelID, ok := fieldsJSON["model_id"].(string); ok {
		record.ModelID = modelID
		delete(fieldsJSON, "model_id")
	}

	return record
}
