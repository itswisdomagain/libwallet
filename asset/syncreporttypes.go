package asset

import (
	"math"
	"time"
)

type SyncStage int8

const (
	InvalidSyncStage SyncStage = iota
	HeadersFetchSyncStage
	CFiltersFetchSyncStage
	HeadersRescanSyncStage
	AddressDiscoverySyncStage
	SyncCompleteSyncStage
)

type StatTimeTracker struct {
	BeginTimeStamp time.Time
	EndTimeStamp   time.Time
}

func (stat *StatTimeTracker) TimeTaken() time.Duration {
	return time.Since(stat.BeginTimeStamp)
}

type GeneralSyncProgress struct {
	TotalSyncProgress         int32
	TotalTimeRemainingSeconds int64
}

type CFiltersFetchProgressReport struct {
	BeginFetchCFiltersTimeStamp int64
	StartCFiltersHeight         int32
	CfiltersFetchTimeSpent      int64
	TotalFetchedCFiltersCount   int32
	TotalCFiltersToFetch        int32
	CurrentCFilterHeight        int32
	CFiltersFetchProgress       int32
}

type HeadersFetchStat struct {
	StatTimeTracker
	StartHeight       int32
	TargetHeight      int32
	LastFetchedHeight int32
}

// Progress returns the progress of the headers fetching phase of the sync as a
// percentage value between 0 and 100.
func (stat *HeadersFetchStat) Progress() float64 {
	headersFetchedSoFar := math.Min(float64(stat.LastFetchedHeight-stat.StartHeight), 1)
	remainingHeaders := math.Min(float64(stat.TargetHeight-stat.LastFetchedHeight), 1)
	allHeadersToFetch := headersFetchedSoFar + remainingHeaders
	return (headersFetchedSoFar * 100) / allHeadersToFetch
}

// EstimateTimeRemaining returns an estimate of the amount of time it will take
// to complete this phase of the sync.
func (stat *HeadersFetchStat) EstimateTimeRemaining() time.Duration {
	headersFetchedSoFar := math.Min(float64(stat.LastFetchedHeight-stat.StartHeight), 1)
	remainingHeaders := math.Min(float64(stat.TargetHeight-stat.LastFetchedHeight), 1)
	return stat.TimeTaken() * time.Duration(headersFetchedSoFar/remainingHeaders)
}

type AddressDiscoveryProgressReport struct {
	AddressDiscoveryStartTime int64
	TotalDiscoveryTimeSpent   int64
	AddressDiscoveryProgress  int32
}

type HeadersRescanProgressReport struct {
	TotalHeadersToScan  int32
	CurrentRescanHeight int32
	RescanProgress      int32
	RescanTimeRemaining int64
}

type SyncProgressListener interface {
	OnSyncStarted()
	OnPeerConnectedOrDisconnected(numberOfConnectedPeers int32)
	OnHeadersFetchProgress(stat *HeadersFetchStat)
	OnCFiltersFetchProgress(cfiltersFetchProgress *CFiltersFetchProgressReport)
	OnAddressDiscoveryProgress(addressDiscoveryProgress *AddressDiscoveryProgressReport)
	OnHeadersRescanProgress(headersRescanProgress *HeadersRescanProgressReport)
	OnSyncCompleted()
	OnSyncCanceled(willRestart bool)
	OnSyncEndedWithError(err error)
}
