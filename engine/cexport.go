//go:build cgo
// +build cgo

// Package engine — cexport.go
//
// C-compatible export layer so Python (ctypes / cffi) can drive the engine
// as a shared library without any Python-side Go dependency.
//
// Build as a shared library:
//   go build -buildmode=c-shared -o brass_engine.so ./engine
//
// Python usage:
//   import ctypes, numpy as np
//   lib = ctypes.CDLL("./brass_engine.so")
//   env_id = lib.BrassNewEnv(2)
//   obs_buf = np.zeros(lib.BrassObsSize(), dtype=np.float32)
//   lib.BrassGetObs(env_id, obs_buf.ctypes.data_as(ctypes.POINTER(ctypes.c_float)))
//
// Thread safety: each env_id is independent; callers must not share an env_id
// across goroutines. The global envPool is protected by a RWMutex.

package engine

/*
#include <stdint.h>
#include <stdbool.h>
*/
import "C"

import (
	"sync"
	"unsafe"
)

// ─── Env pool ─────────────────────────────────────────────────────────────────

var (
	poolMu  sync.RWMutex
	envPool = make(map[C.int32_t]*Env)
	poolSeq C.int32_t
)

func lookupEnv(id C.int32_t) *Env {
	poolMu.RLock()
	defer poolMu.RUnlock()
	return envPool[id]
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

//export BrassNewEnv
func BrassNewEnv(numPlayers C.int32_t) C.int32_t {
	env := NewEnv(int(numPlayers))
	poolMu.Lock()
	poolSeq++
	id := poolSeq
	envPool[id] = env
	poolMu.Unlock()
	return id
}

//export BrassReset
func BrassReset(envID C.int32_t) {
	if env := lookupEnv(envID); env != nil {
		env.Reset()
	}
}

//export BrassFreeEnv
func BrassFreeEnv(envID C.int32_t) {
	poolMu.Lock()
	delete(envPool, envID)
	poolMu.Unlock()
}

// ─── Step ─────────────────────────────────────────────────────────────────────

//export BrassStep
func BrassStep(envID C.int32_t, actionID C.int32_t, rewardOut *C.float, doneOut *C.int32_t) {
	env := lookupEnv(envID)
	if env == nil {
		return
	}
	reward, done := env.Step(int(actionID))
	*rewardOut = C.float(reward)
	if done {
		*doneOut = 1
	} else {
		*doneOut = 0
	}
}

// ─── Observation ──────────────────────────────────────────────────────────────

//export BrassObsSize
func BrassObsSize() C.int32_t {
	return C.int32_t(ObsTotalSize)
}

//export BrassGetObs
func BrassGetObs(envID C.int32_t, bufOut *C.float) {
	env := lookupEnv(envID)
	if env == nil || bufOut == nil {
		return
	}
	// Wrap the C buffer as a Go slice (zero-copy, no allocation).
	slice := unsafe.Slice((*float32)(unsafe.Pointer(bufOut)), ObsTotalSize)
	FillObservation(env.State, slice)
}

// ─── Action mask ──────────────────────────────────────────────────────────────

//export BrassActionSize
func BrassActionSize() C.int32_t {
	EnsureActionRegistry(nil) // registry is board-independent for sizing
	return C.int32_t(GetActionSpaceSize())
}

//export BrassGetMask
func BrassGetMask(envID C.int32_t, bufOut *C.int32_t) {
	env := lookupEnv(envID)
	if env == nil || bufOut == nil {
		return
	}
	mask := env.GetActionMask()
	// Write 1/0 int32 values into the C buffer (avoids Go bool layout ambiguity).
	cSlice := unsafe.Slice((*C.int32_t)(unsafe.Pointer(bufOut)), len(mask))
	for i, valid := range mask {
		if valid {
			cSlice[i] = 1
		} else {
			cSlice[i] = 0
		}
	}
}

// ─── State queries ────────────────────────────────────────────────────────────

//export BrassActivePlayer
func BrassActivePlayer(envID C.int32_t) C.int32_t {
	env := lookupEnv(envID)
	if env == nil {
		return -1
	}
	return C.int32_t(env.State.Active)
}

//export BrassIsGameOver
func BrassIsGameOver(envID C.int32_t) C.int32_t {
	env := lookupEnv(envID)
	if env == nil || env.State.GameOver {
		return 1
	}
	return 0
}

//export BrassPlayerVP
func BrassPlayerVP(envID C.int32_t, playerID C.int32_t) C.int32_t {
	env := lookupEnv(envID)
	if env == nil || int(playerID) >= env.State.NumPlayers {
		return -1
	}
	return C.int32_t(env.State.Players[playerID].VP)
}
