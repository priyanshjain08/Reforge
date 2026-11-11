package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type persisted struct {
	Services    []*Service    `json:"services"`
	Agents      []*Agent      `json:"agents"`
	Incidents   []*Incident   `json:"incidents"`
	Operations  []*Operation  `json:"operations"`
	Policies    []*Policy     `json:"policies"`
	Experiments []*Experiment `json:"experiments"`
	Audit       []*Audit      `json:"audit"`
	AuditSeq    int64         `json:"audit_seq"`
	KillSwitch  bool          `json:"kill_switch"`
}

type Store struct {
	mu          sync.RWMutex
	Services    map[string]*Service
	Agents      map[string]*Agent
	Incidents   []*Incident
	Operations  []*Operation
	Policies    map[string]*Policy
	Experiments []*Experiment
	AuditLog    []*Audit
	auditSeq    int64
	KillSwitch  bool
	History     map[string][]Point // "svc:<name>" or "<agentID>|<metric>"
	Commands    map[string]*Command
	path        string
	dirty       bool
	subs        map[chan []byte]struct{}
	subMu       sync.Mutex
}

func NewStore(dataDir string) *Store {
	s := &Store{Services: map[string]*Service{}, Agents: map[string]*Agent{}, Policies: map[string]*Policy{},
		History: map[string][]Point{}, Commands: map[string]*Command{}, subs: map[chan []byte]struct{}{}}
	if dataDir != "" {
		_ = os.MkdirAll(dataDir, 0o700)
		s.path = filepath.Join(dataDir, "state.json")
		s.load()
	}
	return s
}

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var p persisted
	if json.Unmarshal(data, &p) != nil {
		return
	}
	for _, v := range p.Services {
		v.Status, v.ConsecutiveFailures, v.SlowStreak = "unknown", 0, 0
		s.Services[v.ID] = v
	}
	for _, v := range p.Agents {
		v.Connected = false
		s.Agents[v.ID] = v
	}
	for _, v := range p.Policies {
		s.Policies[v.ID] = v
	}
	s.Incidents, s.Operations, s.Experiments, s.AuditLog = p.Incidents, p.Operations, p.Experiments, p.Audit
	s.auditSeq, s.KillSwitch = p.AuditSeq, p.KillSwitch
}

// MarkDirty must be called with s.mu held or not; it only flips a flag.
func (s *Store) MarkDirty() { s.dirty = true }

func (s *Store) Flush() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	p := persisted{Incidents: s.Incidents, Operations: s.Operations, Experiments: s.Experiments,
		Audit: s.AuditLog, AuditSeq: s.auditSeq, KillSwitch: s.KillSwitch}
	for _, v := range s.Services {
		p.Services = append(p.Services, v)
	}
	for _, v := range s.Agents {
		p.Agents = append(p.Agents, v)
	}
	for _, v := range s.Policies {
		p.Policies = append(p.Policies, v)
	}
	data, err := json.Marshal(p)
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// Audit appends an immutable audit record (the store exposes no update/delete for audit entries).
func (s *Store) Audit(actor, action, target, outcome, incident string, meta map[string]string) {
	s.mu.Lock()
	s.auditSeq++
	s.AuditLog = append(s.AuditLog, &Audit{ID: s.auditSeq, TS: time.Now().UTC(), Actor: actor, Action: action,
		Target: target, Outcome: outcome, IncidentID: incident, Metadata: meta})
	s.dirty = true
	s.mu.Unlock()
	s.Publish("audit", map[string]string{"action": action, "target": target, "outcome": outcome})
}

func (s *Store) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	return ch
}

func (s *Store) Unsubscribe(ch chan []byte) {
	s.subMu.Lock()
	delete(s.subs, ch)
	s.subMu.Unlock()
}

func (s *Store) Publish(kind string, data any) {
	b, err := json.Marshal(map[string]any{"type": kind, "data": data, "ts": time.Now().UTC()})
	if err != nil {
		return
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- b:
		default: // slow consumer: drop
		}
	}
}

func (s *Store) AddPoint(key string, p Point) {
	s.mu.Lock()
	h := append(s.History[key], p)
	if len(h) > 180 {
		h = h[len(h)-180:]
	}
	s.History[key] = h
	s.mu.Unlock()
}

// helpers (caller must hold lock)
func (s *Store) incidentByID(id string) *Incident {
	for _, i := range s.Incidents {
		if i.ID == id {
			return i
		}
	}
	return nil
}

func (s *Store) openIncident(dedup string) *Incident {
	for _, i := range s.Incidents {
		if i.DedupKey == dedup && i.Status != "resolved" {
			return i
		}
	}
	return nil
}

func (i *Incident) addEvent(kind, detail string) {
	i.Events = append(i.Events, IncidentEvent{TS: time.Now().UTC(), Kind: kind, Detail: detail})
}
