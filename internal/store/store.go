// Package store persists sessions: an append-only events.jsonl (source of
// truth) plus a state.json snapshot cache (docs/spec/core.md §4). All writes go
// through temp-file + rename for atomicity.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentwiki/squiz/internal/engine"
)

const dirName = ".squiz"

type Store struct {
	Root string // the .squiz directory
}

// Open finds or creates .squiz under baseDir.
func Open(baseDir string) (*Store, error) {
	root := filepath.Join(baseDir, dirName)
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o755); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (st *Store) sessionDir(id string) string {
	return filepath.Join(st.Root, "sessions", id)
}

func (st *Store) QueueDir(id string) string {
	return filepath.Join(st.sessionDir(id), "q")
}

type pointer struct {
	ID string `json:"id"`
}

func (st *Store) ActiveID() (string, error) {
	b, err := os.ReadFile(filepath.Join(st.Root, "session.json"))
	if err != nil {
		return "", err
	}
	var p pointer
	if err := json.Unmarshal(b, &p); err != nil {
		return "", err
	}
	return p.ID, nil
}

// Create initializes a new session directory and makes it active.
func (st *Store) Create(source engine.Source, role engine.Role) (*engine.Session, error) {
	// a random suffix keeps two inits in the same second from colliding
	var rnd [3]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return nil, err
	}
	id := time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(rnd[:])
	dir := st.sessionDir(id)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("session %s already exists", id)
	}
	if err := os.MkdirAll(filepath.Join(dir, "q"), 0o755); err != nil {
		return nil, err
	}
	s, err := engine.NewSession(id, source, role)
	if err != nil {
		return nil, err
	}
	init := engine.InitPayload{ID: id, Source: source, Role: role}
	if err := st.appendEvent(id, engine.EvInit, init, 1); err != nil {
		return nil, err
	}
	s.Seq = 1
	if err := st.snapshot(s); err != nil {
		return nil, err
	}
	if err := AtomicWriteJSON(filepath.Join(st.Root, "session.json"), pointer{ID: id}); err != nil {
		return nil, err
	}
	return s, nil
}

// Load replays events.jsonl. state.json is only a cache; the log wins.
func (st *Store) Load(id string) (*engine.Session, error) {
	f, err := os.Open(filepath.Join(st.sessionDir(id), "events.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s *engine.Session
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev engine.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("corrupt event log: %w", err)
		}
		if ev.Type == engine.EvInit {
			var p engine.InitPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, err
			}
			s, err = engine.NewSession(p.ID, p.Source, p.Role)
			if err != nil {
				return nil, err
			}
			s.Seq = ev.Seq
			continue
		}
		if s == nil {
			return nil, errors.New("event log does not start with init")
		}
		if err := s.Apply(ev.Type, ev.Payload); err != nil {
			return nil, fmt.Errorf("replay seq %d (%s): %w", ev.Seq, ev.Type, err)
		}
		s.Seq = ev.Seq
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("empty event log")
	}
	return s, nil
}

func (st *Store) LoadActive() (*engine.Session, error) {
	id, err := st.ActiveID()
	if err != nil {
		return nil, fmt.Errorf("no active session (run `squiz init` first): %w", err)
	}
	return st.Load(id)
}

// Commit validates+applies the event on s, then persists log and snapshot.
// The caller passes the already-loaded session; on Apply error nothing is
// written (I8: rejected events leave no trace beyond the error).
func (st *Store) Commit(s *engine.Session, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := s.Apply(typ, raw); err != nil {
		return err
	}
	seq := s.Seq + 1
	if err := st.appendEvent(s.ID, typ, payload, seq); err != nil {
		return err
	}
	s.Seq = seq
	return st.snapshot(s)
}

func (st *Store) appendEvent(id, typ string, payload any, seq int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ev := engine.Event{
		Seq: seq, TS: time.Now().UTC().Format(time.RFC3339), Type: typ, Payload: raw,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(st.sessionDir(id), "events.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (st *Store) snapshot(s *engine.Session) error {
	return AtomicWriteJSON(filepath.Join(st.sessionDir(s.ID), "state.json"), s)
}

// AtomicWriteJSON writes via temp file + rename (docs/spec/core.md §4).
func AtomicWriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
