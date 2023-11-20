package syncutils

import (
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

func (stage SyncStage) String() string {
	switch stage {
	case HeadersFetchSyncStage:
		return "feching headers"
	case CFiltersFetchSyncStage:
		return "fetching cfilters"
	case HeadersRescanSyncStage:
		return "rescanning"
	case AddressDiscoverySyncStage:
		return "discovering addresses"
	case SyncCompleteSyncStage:
		return "sync completed"
	}
	return "InvalidSyncStage"
}

// TODO: Remove.
type StatTimeTracker struct {
	BeginTimeStamp time.Time
	EndTimeStamp   time.Time
}

func (stat *StatTimeTracker) TimeTaken() time.Duration {
	if stat.EndTimeStamp.IsZero() {
		return time.Since(stat.BeginTimeStamp)
	}
	return stat.EndTimeStamp.Sub(stat.BeginTimeStamp)
}

// SyncActivityReport is the progress report for a sync activity.
type SyncActivityReport struct {
	// StartTimeStamp is the time that this activity was started.
	StartTimeStamp time.Time
	// StartHeight is the block height at the start of this activity.
	StartHeight int32
	// TargetHeight is the intended final height for this activity.
	TargetHeight int32
	// LastHeight is the last block height processed by this activity.
	LastHeight int32
}

// CalculateProgress returns the progress of this activity as a percentage value
// between 0 and 100, and the estimated amount of time left for this activity to
// be completed.
func (report *SyncActivityReport) CalculateProgress() (float64, time.Duration) {
	report.cleanValues()

	done := float64(report.LastHeight - report.StartHeight)
	allToDo := float64(report.TargetHeight - report.StartHeight)
	progress := done / allToDo

	nanosecondsTakenSoFar := float64(time.Since(report.StartTimeStamp))
	estimatedTotalNanoseconds := nanosecondsTakenSoFar / progress
	estimatedNanosecondsLeft := estimatedTotalNanoseconds - nanosecondsTakenSoFar

	return progress * 100, time.Duration(estimatedNanosecondsLeft)
}

// cleanValues ensures that the StartHeight and TargetHeight have sane values.
func (report *SyncActivityReport) cleanValues() {
	if report.LastHeight < report.StartHeight {
		// Doesn't make sense that the last processed height is before the start
		// height. Reduce the start height to the last processed height.
		report.StartHeight = report.LastHeight
	}

	if report.TargetHeight < report.LastHeight {
		// If the last processed height exceeds the initial target, adjust the
		// TargetHeight.
		report.TargetHeight = report.LastHeight
	}
}
