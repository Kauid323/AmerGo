package web

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type LogCollector struct {
	mu          sync.RWMutex
	maxLines    int
	buffer      []string
	subscribers map[chan string]struct{}
}

var GlobalLogCollector *LogCollector

// InitLogCollector initializes the log collector and attaches it to standard logger and Gin writer.
func InitLogCollector(maxLines int) *LogCollector {
	if maxLines <= 0 {
		maxLines = 2000
	}
	lc := &LogCollector{
		maxLines:    maxLines,
		buffer:      make([]string, 0, maxLines),
		subscribers: make(map[chan string]struct{}),
	}
	GlobalLogCollector = lc

	multiOut := io.MultiWriter(os.Stdout, lc)
	log.SetOutput(multiOut)
	gin.DefaultWriter = multiOut

	return lc
}

func (lc *LogCollector) Write(p []byte) (n int, err error) {
	text := string(p)
	lc.mu.Lock()
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(lc.buffer) >= lc.maxLines {
			lc.buffer = lc.buffer[1:]
		}
		lc.buffer = append(lc.buffer, line)

		for ch := range lc.subscribers {
			select {
			case ch <- line:
			default:
			}
		}
	}
	lc.mu.Unlock()
	return len(p), nil
}

func (lc *LogCollector) GetRecentLogs(count int) []string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if count <= 0 || count > len(lc.buffer) {
		count = len(lc.buffer)
	}
	start := len(lc.buffer) - count
	result := make([]string, count)
	copy(result, lc.buffer[start:])
	return result
}

func (lc *LogCollector) Subscribe() chan string {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	ch := make(chan string, 128)
	lc.subscribers[ch] = struct{}{}
	return ch
}

func (lc *LogCollector) Unsubscribe(ch chan string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if _, ok := lc.subscribers[ch]; ok {
		delete(lc.subscribers, ch)
		close(ch)
	}
}

// LogsAPIHandler returns static log snapshot (non-real-time mode).
func LogsAPIHandler(c *gin.Context) {
	if GlobalLogCollector == nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"logs":  []string{},
				"total": 0,
			},
		})
		return
	}

	linesStr := c.DefaultQuery("lines", "300")
	lines, err := strconv.Atoi(linesStr)
	if err != nil || lines <= 0 {
		lines = 300
	}

	recent := GlobalLogCollector.GetRecentLogs(lines)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"logs":  recent,
			"total": len(recent),
		},
	})
}

// LogsStreamHandler streams real-time console logs via Server-Sent Events (SSE).
func LogsStreamHandler(c *gin.Context) {
	if GlobalLogCollector == nil {
		c.String(http.StatusInternalServerError, "日志收集器未初始化")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	ch := GlobalLogCollector.Subscribe()
	defer GlobalLogCollector.Unsubscribe(ch)

	// 先将最近 80 条日志下发给客户端，建立上下文
	recent := GlobalLogCollector.GetRecentLogs(80)
	for _, line := range recent {
		fmt.Fprintf(c.Writer, "data: %s\n\n", line)
	}
	c.Writer.Flush()

	clientGone := c.Request.Context().Done()
	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-clientGone:
			return
		case <-keepAliveTicker.C:
			fmt.Fprintf(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		case line, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", line)
			c.Writer.Flush()
		}
	}
}
