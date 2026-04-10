package main

/*
#include <stdint.h>
#include <stdbool.h>
*/
import "C"

import (
	"encoding/json"
	"sync"
	"unsafe"

	"brass_engine/engine"
)

// ─── Env pool ─────────────────────────────────────────────────────────────────

var (
	poolMu  sync.RWMutex
	envPool = make(map[C.int32_t]*engine.Env)
	poolSeq C.int32_t
)

func lookupEnv(id C.int32_t) *engine.Env {
	poolMu.RLock()
	defer poolMu.RUnlock()
	return envPool[id]
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

//export BrassNewEnv
func BrassNewEnv(numPlayers C.int32_t) C.int32_t {
	env := engine.NewEnv(int(numPlayers))
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
func BrassStep(envID C.int32_t, actionID C.int32_t, denseRewardScale C.double, rewardOut *C.float, doneOut *C.int32_t) {
	env := lookupEnv(envID)
	if env == nil {
		return
	}
	// Action names/details are required for dashboard traces, so we always include metadata in BrassStep.
	reward, done := env.Step(int(actionID), true, float64(denseRewardScale))
	*rewardOut = C.float(reward)
	if done {
		*doneOut = 1
	} else {
		*doneOut = 0
	}
}

//export BrassGetStepMetadataJSON
func BrassGetStepMetadataJSON(envID C.int32_t, bufOut *C.char, maxLen C.int) C.int {
	env := lookupEnv(envID)
	if env == nil {
		return 0
	}
	jsonData, err := json.Marshal(env.LastMetadata)
	if err != nil {
		return 0
	}
	if len(jsonData) >= int(maxLen) {
		return C.int(-len(jsonData))
	}
	cSlice := unsafe.Slice((*byte)(unsafe.Pointer(bufOut)), maxLen)
	copy(cSlice, jsonData)
	return C.int(len(jsonData))
}

// ─── Observation ──────────────────────────────────────────────────────────────

//export BrassObsSize
func BrassObsSize() C.int32_t {
	return C.int32_t(engine.ObsTotalSize)
}

//export BrassObsSlotEnd
func BrassObsSlotEnd() C.int32_t {
	return C.int32_t(engine.ObsSlotEnd)
}

//export BrassGetObs
func BrassGetObs(envID C.int32_t, bufOut *C.float) {
	env := lookupEnv(envID)
	if env == nil || bufOut == nil {
		return
	}
	// Wrap the C buffer as a Go slice (zero-copy, no allocation).
	slice := unsafe.Slice((*float32)(unsafe.Pointer(bufOut)), engine.ObsTotalSize)
	engine.FillObservation(env.State, slice)
}

// ─── Action mask ──────────────────────────────────────────────────────────────

//export BrassActionSize
func BrassActionSize() C.int32_t {
	dummy := engine.NewEnv(2)
	engine.EnsureActionRegistry(dummy.State.Board)
	return C.int32_t(engine.GetActionSpaceSize())
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

// ─── New Diagnostic State Query ───────────────────────────────────────────────

//export BrassGetStateJSON
func BrassGetStateJSON(envID C.int32_t, bufOut *C.char, maxLen C.int) C.int {
	env := lookupEnv(envID)
	if env == nil {
		return 0
	}
	jsonData, err := json.Marshal(env.State)
	if err != nil {
		return 0
	}
	if len(jsonData) >= int(maxLen) {
		return C.int(-len(jsonData)) // Indicate needed size
	}
	// Copy to C buffer
	cSlice := unsafe.Slice((*byte)(unsafe.Pointer(bufOut)), maxLen)
	copy(cSlice, jsonData)
	return C.int(len(jsonData))
}

//export BrassGetActionNamesJSON
func BrassGetActionNamesJSON(bufOut *C.char, maxLen C.int) C.int {
	dummy := engine.NewEnv(2)
	engine.EnsureActionRegistry(dummy.State.Board)
	actionSize := engine.GetActionSpaceSize()
	names := make([]string, actionSize)
	for i := 0; i < actionSize; i++ {
		if i < len(engine.ActionRegistry) {
			a := engine.ActionRegistry[i]
			names[i] = a.Name(dummy.State.Board)
		} else {
			names[i] = "Padding"
		}
	}
	jsonData, err := json.Marshal(names)
	if err != nil {
		return 0
	}
	if len(jsonData) >= int(maxLen) {
		return C.int(-len(jsonData))
	}
	cSlice := unsafe.Slice((*byte)(unsafe.Pointer(bufOut)), maxLen)
	copy(cSlice, jsonData)
	return C.int(len(jsonData))
}

func main() {
	// Required for buildmode=c-shared
}
