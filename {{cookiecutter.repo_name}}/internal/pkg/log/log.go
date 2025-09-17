package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"{{cookiecutter.module_name}}/internal/conf"

	zaplog "github.com/go-kratos/kratos/contrib/log/zap/v2"
	"github.com/go-kratos/kratos/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger 创建一个新的日志记录器
// 根据配置支持文本格式和JSON格式
func NewLogger(c *conf.Log) log.Logger {
	if c == nil {
		return log.NewStdLogger(os.Stdout)
	}

	format := strings.ToLower(c.Format)

	switch format {
	case "json":
		return newJSONLogger(c)
	case "text", "":
		return newTextLogger(c)
	default:
		// 默认使用文本格式
		return newTextLogger(c)
	}
}

// newJSONLogger 创建JSON格式的日志记录器（使用zap）
func newJSONLogger(c *conf.Log) log.Logger {
	// 配置编码器为JSON格式
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.LevelKey = "level"
	encoderConfig.MessageKey = "msg"
	// 禁用zap自带的caller，使用Kratos的caller
	encoderConfig.CallerKey = ""
	// 使用自定义时间格式，移除时区和T分隔符
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000000"))
	}
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// 配置输出
	var cores []zapcore.Core

	// 控制台输出
	if c.Console {
		consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), getZapLevel(c.Level))
		cores = append(cores, consoleCore)
	}

	// 文件输出
	if c.Filename != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(c.Filename)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			panic(fmt.Sprintf("failed to create log directory: %v", err))
		}

		// 配置日志轮转
		lumberjackLogger := &lumberjack.Logger{
			Filename:   c.Filename,
			MaxSize:    int(c.MaxSize), // MB
			MaxAge:     int(c.MaxAge),  // days
			MaxBackups: int(c.MaxBackups),
			Compress:   c.Compress,
		}

		fileCore := zapcore.NewCore(encoder, zapcore.AddSync(lumberjackLogger), getZapLevel(c.Level))
		cores = append(cores, fileCore)
	}

	// 如果没有配置任何输出，默认使用标准输出
	if len(cores) == 0 {
		consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), getZapLevel(c.Level))
		cores = append(cores, consoleCore)
	}

	// 创建zap logger，不添加caller（使用Kratos的caller）
	core := zapcore.NewTee(cores...)
	zapLogger := zap.New(core)

	// 包装为Kratos Logger
	return zaplog.NewLogger(zapLogger)
}

// newTextLogger 创建文本格式的日志记录器（使用Kratos标准实现）
func newTextLogger(c *conf.Log) log.Logger {
	var writers []io.Writer

	// 如果启用控制台输出
	if c.Console {
		writers = append(writers, os.Stdout)
	}

	// 如果配置了文件输出
	if c.Filename != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(c.Filename)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			panic(fmt.Sprintf("failed to create log directory: %v", err))
		}

		// 使用lumberjack进行日志轮转
		lumberjackLogger := &lumberjack.Logger{
			Filename:   c.Filename,
			MaxSize:    int(c.MaxSize), // MB
			MaxAge:     int(c.MaxAge),  // days
			MaxBackups: int(c.MaxBackups),
			Compress:   c.Compress,
		}

		writers = append(writers, lumberjackLogger)
	}

	// 如果没有配置任何输出，默认使用标准输出
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	// 创建多重写入器
	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	return log.NewStdLogger(writer)
}

// getZapLevel 将字符串级别转换为zap级别
func getZapLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// GetLogLevel 获取Kratos日志级别（保持向后兼容）
func GetLogLevel(level string) log.Level {
	switch strings.ToLower(level) {
	case "debug":
		return log.LevelDebug
	case "info":
		return log.LevelInfo
	case "warn":
		return log.LevelWarn
	case "error":
		return log.LevelError
	case "fatal":
		return log.LevelFatal
	default:
		return log.LevelInfo
	}
}
