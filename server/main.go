package main

// Brass Birmingham game server — exposes the engine over HTTP/JSON so Python
// can drive it without CGO or a C compiler.
//
// Run:   go run ./server      (from f:\Projects\brass)
// Build: go build -o server\brass_server.exe ./server

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"fmt"

	eng "brass_engine/engine"
)

// ─── Env pool ─────────────────────────────────────────────────────────────────

type server struct {
	mu      sync.RWMutex
	envs    map[int64]*eng.Env
	counter int64
	// Pre-allocated obs buffer per env — eliminates per-step allocation.
	// Keyed by env id.
	obsBufs map[int64][]float32
}

func newServer() *server {
	// Warm up the action registry with a dummy env so /health can report action_size.
	dummy := eng.NewEnv(2)
	eng.EnsureActionRegistry(dummy.State.Board)
	return &server{
		envs:    make(map[int64]*eng.Env),
		obsBufs: make(map[int64][]float32),
	}
}

func (s *server) getEnv(id int64) *eng.Env {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.envs[id]
}

func (s *server) getObsBuf(id int64) []float32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.obsBufs[id]
}

// ─── Serialization helpers ────────────────────────────────────────────────────

// obs2b64 encodes a float32 slice as little-endian bytes, then base64.
// This is ~3× faster for Python to decode than a JSON float array.
func obs2b64(obs []float32) string {
	b := make([]byte, len(obs)*4)
	for i, f := range obs {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// mask2b64 bit-packs the bool mask (LSB first) then base64-encodes it.
// Reduces ~1000-entry JSON array to ~128 bytes.
func mask2b64(mask []bool) string {
	n := (len(mask) + 7) / 8
	b := make([]byte, n)
	for i, v := range mask {
		if v {
			b[i/8] |= 1 << uint(i%8)
		}
	}
	return base64.StdEncoding.EncodeToString(b)
}

// ─── Response types ───────────────────────────────────────────────────────────

type stateResp struct {
	ObsB64  string `json:"obs_b64"`  // base64(LE float32[])
	MaskB64 string `json:"mask_b64"` // base64(bit-packed bools)
	ObsSize int    `json:"obs_size"`
	MaskSize int   `json:"mask_size"`
	VPs     []int  `json:"vps"`
	
	// Audit/Diagnostic fields
	VPsIndustries         []int `json:"vps_industries"`
	VPsLinks              []int `json:"vps_links"`
	ConsumedOpponentCoal  []int `json:"consumed_opponent_coal"`
	ConsumedOpponentIron  []int `json:"consumed_opponent_iron"`
	VPsMerchant           []int `json:"vps_merchant"`

	State *eng.GameState `json:"state,omitempty"`
}

type stepResp struct {
	stateResp
	Reward   float64          `json:"reward"`
	Done     bool             `json:"done"`
	Metadata eng.StepMetadata `json:"metadata"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

func buildStateResp(e *eng.Env, buf []float32, fullState bool) stateResp {
	eng.FillObservation(e.State, buf)
	mask := e.GetActionMask()
	numPlayers := len(e.State.Players)
	vps := make([]int, numPlayers)
	vpsInd := make([]int, numPlayers)
	vpsLink := make([]int, numPlayers)
	consCoal := make([]int, numPlayers)
	consIron := make([]int, numPlayers)
	vpsMerc := make([]int, numPlayers)

	for i, p := range e.State.Players {
		vps[i] = p.VP
		vpsInd[i] = p.VPAuditIndustries
		vpsLink[i] = p.VPAuditLinks
		consCoal[i] = p.ConsumedOpponentCoal
		consIron[i] = p.ConsumedOpponentIron
		vpsMerc[i] = 0 // Placeholder
	}

	resp := stateResp{
		ObsB64:   obs2b64(buf),
		MaskB64:  mask2b64(mask),
		ObsSize:  len(buf),
		MaskSize: len(mask),
		VPs:      vps,
		VPsIndustries:       vpsInd,
		VPsLinks:            vpsLink,
		ConsumedOpponentCoal: consCoal,
		ConsumedOpponentIron: consIron,
		VPsMerchant:         vpsMerc,
	}

	if fullState {
		resp.State = e.State
	}

	return resp
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// GET /health
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]int{
		"obs_size":    eng.ObsTotalSize,
		"action_size": eng.GetActionSpaceSize(),
	})
}

// GET /actions
func (s *server) getActionNames(w http.ResponseWriter, r *http.Request) {
	dummy := eng.NewEnv(2)
	eng.EnsureActionRegistry(dummy.State.Board)

	actionSize := eng.ActionSpaceSize
	names := make([]string, actionSize)
	for i := 0; i < actionSize; i++ {
		baseID := i % 1500
		slotID := i / 1500
		if baseID < len(eng.ActionRegistry) {
			a := eng.ActionRegistry[baseID]
			names[i] = fmt.Sprintf("[Slot %d] %s", slotID, a.Name(dummy.State.Board))
		} else {
			names[i] = fmt.Sprintf("[Slot %d] Padding", slotID)
		}
	}
	writeJSON(w, names)
}

// POST /envs?players=N  → {"env_id": N}
func (s *server) createEnv(w http.ResponseWriter, r *http.Request) {
	players := 2
	if p := r.URL.Query().Get("players"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 2 && n <= 4 {
			players = n
		}
	}

	id := atomic.AddInt64(&s.counter, 1)
	e := eng.NewEnv(players)

	s.mu.Lock()
	s.envs[id] = e
	s.obsBufs[id] = make([]float32, eng.ObsTotalSize)
	s.mu.Unlock()

	writeJSON(w, map[string]int64{"env_id": id})
}

// DELETE /envs/{id}
func (s *server) freeEnv(w http.ResponseWriter, r *http.Request, id int64) {
	s.mu.Lock()
	delete(s.envs, id)
	delete(s.obsBufs, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// POST /envs/{id}/reset  → stateResp
func (s *server) resetEnv(w http.ResponseWriter, r *http.Request, id int64) {
	e := s.getEnv(id)
	if e == nil {
		writeErr(w, 404, "env not found")
		return
	}
	e.Reset()
	buf := s.getObsBuf(id)
	includeState := r.URL.Query().Get("include_state") == "true"
	writeJSON(w, buildStateResp(e, buf, includeState))
}

// POST /envs/{id}/step  body: {"action": N}  → stepResp
func (s *server) stepEnv(w http.ResponseWriter, r *http.Request, id int64) {
	e := s.getEnv(id)
	if e == nil {
		writeErr(w, 404, "env not found")
		return
	}

	var body struct {
		Action            int     `json:"action"`
		DenseRewardScale float64 `json:"dense_reward_scale"`
	}
	// Default to 1.0 if not provided (backwards compatibility)
	body.DenseRewardScale = 1.0

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}

	includeState := r.URL.Query().Get("include_state") == "true"
	reward, done := e.Step(body.Action, includeState, body.DenseRewardScale)
	buf := s.getObsBuf(id)
	writeJSON(w, stepResp{
		stateResp: buildStateResp(e, buf, includeState),
		Reward:    reward,
		Done:      done,
		Metadata:  e.LastMetadata,
	})
}

// ─── Router ───────────────────────────────────────────────────────────────────

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers so browser-based debugging tools work
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 3)

	switch {
	case path == "health" && r.Method == http.MethodGet:
		s.health(w, r)

	case path == "actions" && r.Method == http.MethodGet:
		s.getActionNames(w, r)

	case parts[0] == "envs" && len(parts) == 1 && r.Method == http.MethodPost:
		s.createEnv(w, r)

	case parts[0] == "envs" && len(parts) == 3:
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid env id")
			return
		}
		switch parts[2] {
		case "reset":
			if r.Method == http.MethodPost {
				s.resetEnv(w, r, id)
			}
		case "step":
			if r.Method == http.MethodPost {
				s.stepEnv(w, r, id)
			}
		case "free":
			if r.Method == http.MethodDelete {
				s.freeEnv(w, r, id)
			}
		default:
			writeErr(w, 404, "unknown action")
		}

	default:
		writeErr(w, 404, "not found")
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	s := newServer()
	addr := ":8765"
	log.Printf("Brass engine server starting on %s (obs_size=%d action_size=%d)",
		addr, eng.ObsTotalSize, eng.GetActionSpaceSize())
	log.Fatal(http.ListenAndServe(addr, s))
}
