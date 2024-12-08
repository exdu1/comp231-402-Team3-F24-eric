package logger

import (
    "fmt"
    "log"
    "os"
    "time"
)

// LogLevel represents the level of logging (INFO, ERROR, DEBUG).
type LogLevel int

// Constants for log levels.
const (
    INFO LogLevel = iota
    ERROR
    DEBUG
)

// logLevelStrings maps log level values to strings.
var logLevelStrings = []string{
    "INFO",
    "ERROR",
    "DEBUG",
}

// Logger struct holds the log level and log file (if any).
type Logger struct {
    level  LogLevel
    logger *log.Logger
    file   *os.File
}

// NewLogger creates a new logger instance with the specified log level and optional log file.
func NewLogger(level LogLevel, logFile string) (*Logger, error) {
    var logger *log.Logger
    var file *os.File
    var err error

    // Open log file if specified, otherwise use console output.
    if logFile != "" {
        file, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
        if err != nil {
            return nil, err
        }
        logger = log.New(file, "", log.LstdFlags)
    } else {
        logger = log.New(os.Stdout, "", log.LstdFlags)
    }

    return &Logger{
        level:  level,
        logger: logger,
        file:   file,
    }, nil
}

// logMessage handles logging a message with the appropriate level.
func (l *Logger) logMessage(level LogLevel, msg string) {
    if level >= l.level {
        timestamp := time.Now().Format("2006-01-02 15:04:05")
        logMessage := fmt.Sprintf("[%s] [%s] %s", timestamp, logLevelStrings[level], msg)
        l.logger.Println(logMessage)
    }
}

// Info logs an INFO level message.
func (l *Logger) Info(msg string) {
    l.logMessage(INFO, msg)
}

// Error logs an ERROR level message.
func (l *Logger) Error(msg string) {
    l.logMessage(ERROR, msg)
}

// Debug logs a DEBUG level message.
func (l *Logger) Debug(msg string) {
    l.logMessage(DEBUG, msg)
}

// Close closes the log file if it was opened.
func (l *Logger) Close() {
    if l.file != nil {
        l.file.Close()
    }
}


