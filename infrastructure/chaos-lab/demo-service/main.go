// Demo service for the REFORGE chaos lab. It only misbehaves when its own /chaos endpoints are called,
// and only inside the isolated docker-compose network. It never touches the host.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type state struct {
	mu          sync.Mutex
	healthFail  bool
	errorSpike  bool
	slowMS      int
	cpuUntil    time.Time
	memory      [][]byte
}

var s state

func main() {
	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		name = "demo-service"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		hf, slow := s.healthFail, s.slowMS
		s.mu.Unlock()
		if slow > 0 {
			time.Sleep(time.Duration(slow) * time.Millisecond)
		}
		if hf {
			http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"status":"ok","service":%q}`, name)
	})
	mux.HandleFunc("GET /work", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		spike := s.errorSpike
		s.mu.Unlock()
		if spike {
			http.Error(w, "injected failure", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s did some work", name)
	})
	mux.HandleFunc("POST /recover", func(w http.ResponseWriter, r *http.Request) { // application recovery hook
		s.clear()
		fmt.Fprint(w, `{"recovered":true}`)
	})
	mux.HandleFunc("POST /chaos/{type}", func(w http.ResponseWriter, r *http.Request) {
		intensity, _ := strconv.Atoi(r.URL.Query().Get("intensity"))
		duration, _ := strconv.Atoi(r.URL.Query().Get("duration"))
		if intensity < 1 || intensity > 100 {
			intensity = 50
		}
		if duration < 1 || duration > 600 {
			duration = 60
		}
		switch t := r.PathValue("type"); t {
		case "crash":
			fmt.Fprint(w, `{"injected":"crash"}`)
			go func() { time.Sleep(200 * time.Millisecond); log.Println("chaos: crashing process"); os.Exit(1) }()
			return
		case "health_fail":
			s.mu.Lock()
			s.healthFail = true
			s.mu.Unlock()
		case "error_spike":
			s.mu.Lock()
			s.errorSpike, s.healthFail = true, true
			s.mu.Unlock()
		case "slow_response":
			s.mu.Lock()
			s.slowMS = 1000 + intensity*30
			s.mu.Unlock()
		case "cpu_burn":
			s.mu.Lock()
			s.cpuUntil = time.Now().Add(time.Duration(duration) * time.Second)
			s.mu.Unlock()
			for i := 0; i < 1+intensity/50; i++ {
				go func() {
					for {
						s.mu.Lock()
						until := s.cpuUntil
						s.mu.Unlock()
						if time.Now().After(until) {
							return
						}
					}
				}()
			}
		case "memory_pressure":
			s.mu.Lock()
			s.memory = append(s.memory, make([]byte, intensity*1024*1024/4)) // up to ~25MB per call
			for i := range s.memory[len(s.memory)-1] {
				s.memory[len(s.memory)-1][i] = 1
			}
			s.mu.Unlock()
		case "clear":
			s.clear()
		default:
			http.Error(w, "unknown chaos type", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"injected":%q}`, r.PathValue("type"))
	})
	log.Printf("%s listening on :8080", name)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (st *state) clear() {
	st.mu.Lock()
	st.healthFail, st.errorSpike, st.slowMS, st.cpuUntil, st.memory = false, false, 0, time.Time{}, nil
	st.mu.Unlock()
}
