package main

import "C"
import (
	"encoding/json"
	"fmt"
)

const (
	// ErrCodeNotSynced is returned when the wallet must be synced to perform an
	// action but is not.
	ErrCodeNotSynced = 1
)

// CResponse is used for all returns when using the cgo libwallet. Payload only
// populated if no error. Error only populated if error. ErrorCode may be
// populated if an error needs special handling.
type CResponse struct {
	Payload   string `json:"payload,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode int    `json:"errorcode,omitempty"`
}

// errCResponse will return an error to the consumer, and log it if possible.
func errCResponse(errStr string, args ...any) *C.char {
	es := fmt.Sprintf(errStr, args...)
	b, err := json.Marshal(CResponse{Error: es})
	if err != nil {
		panic(err)
	}
	logMtx.RLock()
	if log != nil {
		log.Errorf("returning error to consumer: %v", es)
	}
	logMtx.RUnlock()
	return cString(string(b))
}

// errCResponseWithCode will return an error to the consumer, and log it if possible.
func errCResponseWithCode(errCode int, errStr string, args ...any) *C.char {
	es := fmt.Sprintf(errStr, args...)
	b, err := json.Marshal(CResponse{Error: es, ErrorCode: errCode})
	if err != nil {
		panic(err)
	}
	logMtx.RLock()
	if log != nil {
		log.Errorf("returning error with error code %d to consumer: %v", errCode, es)
	}
	logMtx.RUnlock()
	return cString(string(b))
}

// successCResponse will return a payload the consumer, and log it if possible.
func successCResponse(payload string) *C.char {
	b, err := json.Marshal(CResponse{Payload: payload})
	if err != nil {
		panic(err)
	}
	logMtx.RLock()
	if log != nil {
		log.Tracef("returning payload to consumer: %v", payload)
	}
	logMtx.RUnlock()
	return cString(string(b))
}

type SyncStatusCode int

const (
	SSCNotStarted SyncStatusCode = iota
	SSCFetchingCFilters
	SSCFetchingHeaders
	SSCDiscoveringAddrs
	SSCRescanning
	SSCComplete
)

func (ssc SyncStatusCode) String() string {
	return [...]string{"not started", "fetching cfilters", "fetching headers",
		"discovering addresses", "rescanning", "sync complete"}[ssc]
}

type SyncStatusRes struct {
	SyncStatusCode int    `json:"syncstatuscode"`
	SyncStatus     string `json:"syncstatus"`
	TargetHeight   int    `json:"targetheight"`
	NumPeers       int    `json:"numpeers"`
	CFiltersHeight int    `json:"cfiltersheight,omitempty"`
	HeadersHeight  int    `json:"headersheight,omitempty"`
	RescanHeight   int    `json:"rescanheight,omitempty"`
}
