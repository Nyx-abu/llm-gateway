package dashboard

import (
	"embed"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	//go:embed index.html
	StaticFiles embed.FS
	
	TotalRequests uint64
	StartTime     time.Time = time.Now()
)

type LogRingBuffer struct {
	mu    sync.Mutex
	logs  []json.RawMessage
	head  int
	count int
	size  int
}

func NewLogRingBuffer(size int) *LogRingBuffer {
	return &LogRingBuffer{
		logs: make([]json.RawMessage, size),
		size: size,
	}
}

func (r *LogRingBuffer) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Copy the log line (as json.RawMessage)
	b := make([]byte, len(p))
	copy(b, p)
	
	r.logs[r.head] = json.RawMessage(b)
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
	return len(p), nil
}

func (r *LogRingBuffer) GetLogs() []json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	res := make([]json.RawMessage, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.head - r.count + i + r.size) % r.size
		res = append(res, r.logs[idx])
	}
	return res
}

func StartDashboardServer(logBuffer *LogRingBuffer, activeKeys int) {
	mux := http.NewServeMux()
	
	// We handle /dashboard/ to serve the embedded files
	// However, index.html might be requested at /dashboard/ or /dashboard/index.html
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(StaticFiles))))
	
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		logs := logBuffer.GetLogs()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": logs,
		})
	})
	
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		uptime := int(time.Since(StartTime).Seconds())
		reqs := atomic.LoadUint64(&TotalRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active_keys":    activeKeys,
			"total_requests": reqs,
			"uptime_seconds": uptime,
		})
	})
	
	go http.ListenAndServe(":8082", mux)
}
