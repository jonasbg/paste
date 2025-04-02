package middleware

import (
    "log"
    "sync"
    "time"

    "github.com/jonasbg/paste/m/v2/db"
    "github.com/jonasbg/paste/m/v2/types"
)

const (
    // DefaultChannelSize is the default buffer size for log channels
    DefaultChannelSize = 1000
    // DefaultBatchSize is the default number of logs to accumulate before flushing
    DefaultBatchSize = 100
    // DefaultFlushInterval is the default maximum time to wait before flushing logs
    DefaultFlushInterval = 5 * time.Second
)

// LogManager handles buffering and batch processing of logs
type LogManager struct {
    db             *db.DB
    txLogChannel   chan *types.TransactionLog
    reqLogChannel  chan *types.RequestLog
    batchSize      int
    flushInterval  time.Duration
    wg             sync.WaitGroup
    shutdown       chan struct{}
    txLogBuffer    []*types.TransactionLog
    reqLogBuffer   []*types.RequestLog
    txLogMutex     sync.Mutex
    reqLogMutex    sync.Mutex
    lastFlushTime  time.Time
}

// NewLogManager creates a new LogManager with the specified database connection
func NewLogManager(database *db.DB, options ...func(*LogManager)) *LogManager {
    m := &LogManager{
        db:            database,
        txLogChannel:  make(chan *types.TransactionLog, DefaultChannelSize),
        reqLogChannel: make(chan *types.RequestLog, DefaultChannelSize),
        batchSize:     DefaultBatchSize,
        flushInterval: DefaultFlushInterval,
        shutdown:      make(chan struct{}),
        txLogBuffer:   make([]*types.TransactionLog, 0, DefaultBatchSize),
        reqLogBuffer:  make([]*types.RequestLog, 0, DefaultBatchSize),
        lastFlushTime: time.Now(),
    }

    // Apply options
    for _, option := range options {
        option(m)
    }

    // Start the worker goroutine
    m.wg.Add(1)
    go m.worker()

    return m
}

// WithChannelSize sets the channel buffer size
func WithChannelSize(size int) func(*LogManager) {
    return func(m *LogManager) {
        m.txLogChannel = make(chan *types.TransactionLog, size)
        m.reqLogChannel = make(chan *types.RequestLog, size)
    }
}

// WithBatchSize sets the batch size for log flushing
func WithBatchSize(size int) func(*LogManager) {
    return func(m *LogManager) {
        m.batchSize = size
        m.txLogBuffer = make([]*types.TransactionLog, 0, size)
        m.reqLogBuffer = make([]*types.RequestLog, 0, size)
    }
}

// WithFlushInterval sets the maximum time between flushes
func WithFlushInterval(interval time.Duration) func(*LogManager) {
    return func(m *LogManager) {
        m.flushInterval = interval
    }
}

// LogTransaction adds a transaction log to the buffer
func (m *LogManager) LogTransaction(tx *types.TransactionLog) {
    select {
    case m.txLogChannel <- tx:
        // Log added to channel
    default:
        // Channel is full, log directly to database
        go func(tx *types.TransactionLog) {
            if err := m.db.LogTransaction(tx); err != nil {
                log.Printf("Error logging transaction directly: %v", err)
            }
        }(tx)
    }
}

// LogRequest adds a request log to the buffer
func (m *LogManager) LogRequest(req *types.RequestLog) {
    select {
    case m.reqLogChannel <- req:
        // Log added to channel
    default:
        // Channel is full, log directly to database
        go func(req *types.RequestLog) {
            if err := m.db.LogRequest(req); err != nil {
                log.Printf("Error logging request directly: %v", err)
            }
        }(req)
    }
}

// worker processes logs from channels and periodically flushes them to the database
func (m *LogManager) worker() {
    defer m.wg.Done()

    ticker := time.NewTicker(m.flushInterval)
    defer ticker.Stop()

    for {
        select {
        case <-m.shutdown:
            // Flush remaining logs before shutting down
            m.flush()
            return
        case tx := <-m.txLogChannel:
            m.txLogMutex.Lock()
            m.txLogBuffer = append(m.txLogBuffer, tx)
            shouldFlush := len(m.txLogBuffer) >= m.batchSize
            m.txLogMutex.Unlock()

            if shouldFlush {
                m.flush()
            }
        case req := <-m.reqLogChannel:
            m.reqLogMutex.Lock()
            m.reqLogBuffer = append(m.reqLogBuffer, req)
            shouldFlush := len(m.reqLogBuffer) >= m.batchSize
            m.reqLogMutex.Unlock()

            if shouldFlush {
                m.flush()
            }
        case <-ticker.C:
            if time.Since(m.lastFlushTime) >= m.flushInterval {
                m.flush()
            }
        }
    }
}

// flush writes buffered logs to the database
func (m *LogManager) flush() {
    // Handle transaction logs
    m.txLogMutex.Lock()
    if len(m.txLogBuffer) > 0 {
        logs := make([]*types.TransactionLog, len(m.txLogBuffer))
        copy(logs, m.txLogBuffer)
        m.txLogBuffer = m.txLogBuffer[:0] // Clear buffer but keep capacity
        m.txLogMutex.Unlock()

        if len(logs) > 0 {
            if err := m.db.BatchInsertTransactionLogs(logs); err != nil {
                log.Printf("Error batch logging transactions: %v", err)

                // Fall back to individual inserts
                for _, tx := range logs {
                    if err := m.db.LogTransaction(tx); err != nil {
                        log.Printf("Error logging transaction fallback: %v", err)
                    }
                }
            }
        }
    } else {
        m.txLogMutex.Unlock()
    }

    // Handle request logs
    m.reqLogMutex.Lock()
    if len(m.reqLogBuffer) > 0 {
        logs := make([]*types.RequestLog, len(m.reqLogBuffer))
        copy(logs, m.reqLogBuffer)
        m.reqLogBuffer = m.reqLogBuffer[:0] // Clear buffer but keep capacity
        m.reqLogMutex.Unlock()

        if len(logs) > 0 {
            if err := m.db.BatchInsertRequestLogs(logs); err != nil {
                log.Printf("Error batch logging requests: %v", err)

                // Fall back to individual inserts
                for _, req := range logs {
                    if err := m.db.LogRequest(req); err != nil {
                        log.Printf("Error logging request fallback: %v", err)
                    }
                }
            }
        }
    } else {
        m.reqLogMutex.Unlock()
    }

    m.lastFlushTime = time.Now()
}

// Close shuts down the log manager and ensures all logs are flushed
func (m *LogManager) Close() {
    close(m.shutdown)
    m.wg.Wait()
}

// FlushAndWait immediately flushes all logs and waits for completion
func (m *LogManager) FlushAndWait() {
    m.flush()
}