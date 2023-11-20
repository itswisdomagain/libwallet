package syncutils

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/decred/slog"
)

type SyncReportPublisher interface {
	PublishSyncProgressReport(notifyListener func(SyncProgressListener))
}

type SyncProgressReporter struct {
	SyncReportPublisher
	log slog.Logger

	mtx              sync.RWMutex
	currentSyncStage SyncStage
	syncReports      map[SyncStage]*SyncActivityReport
}

func NewSyncProgressReporter(srp SyncReportPublisher, log slog.Logger) *SyncProgressReporter {
	return &SyncProgressReporter{
		SyncReportPublisher: srp,
		log:                 log,
	}
}

func (reporter *SyncProgressReporter) SyncStarted(startHeight, targetHeight int32) {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	reporter.currentSyncStage = HeadersFetchSyncStage
	reporter.syncReports = map[SyncStage]*SyncActivityReport{
		HeadersFetchSyncStage: {
			StartTimeStamp: time.Now(),
			StartHeight:    startHeight,
			TargetHeight:   targetHeight,
		},
	}

	reporter.PublishSyncProgressReport(func(progressListener SyncProgressListener) {
		progressListener.OnSyncStarted()
	})
}

func (reporter *SyncProgressReporter) HandlePeerConnectedOrDisconnected(peersCount uint32) {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	reporter.PublishSyncProgressReport(func(progressListener SyncProgressListener) {
		progressListener.OnPeerConnectedOrDisconnected(peersCount)
	})
}

func (reporter *SyncProgressReporter) HandleBlockConnected(connectedHeight, targetHeight int32, relevantTxs []*wtxmgr.TxRecord, logProgress ...bool) {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	currentStage, ok := reporter.reportForStage(HeadersFetchSyncStage)
	if !ok {
		return
	}

	currentStage.LastHeight = connectedHeight
	currentStage.TargetHeight = targetHeight

	// Calculate the overall progress and time remaining.
	overallProgress, overallTimeRemaining := currentStage.CalculateProgress()
	if isdcr := false; isdcr {
		// TODO: overall progress is calculated differently for dcr
	}

	// Send this progress report to interested parties.
	report := &SyncProgressReport{
		CurrentStage:       reporter.currentSyncStage,
		CurrentHeight:      currentStage.LastHeight,
		TargetHeight:       currentStage.TargetHeight,
		PercentageProgress: overallProgress,
		TimeRemaining:      overallTimeRemaining,
	}
	reporter.PublishSyncProgressReport(func(progressListener SyncProgressListener) {
		progressListener.OnProgress(report)
	})

	// Log progress report if the caller did not explicitly prevent doing so.
	if len(logProgress) == 0 || logProgress[0] {
		reporter.logProgress(report)
	}
}

func (reporter *SyncProgressReporter) HandleSyncCompleted() {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	reporter.currentSyncStage = SyncCompleteSyncStage

	reporter.PublishSyncProgressReport(func(progressListener SyncProgressListener) {
		progressListener.OnSyncCompleted()
	})

	reporter.log.Infof("Syncing 100%% complete")
}

func (reporter *SyncProgressReporter) reportForStage(stage SyncStage) (*SyncActivityReport, bool) {
	if reporter.currentSyncStage != stage {
		reporter.log.Errorf("Invalid attempt to update %s progress in %s stage", stage, reporter.currentSyncStage)
		return nil, false
	}

	report, ok := reporter.syncReports[stage]
	if !ok {
		reporter.log.Errorf("No report to update for sync stage %s", stage)
		return nil, false
	}

	return report, true
}

func (reporter *SyncProgressReporter) logProgress(report *SyncProgressReport) {
	timeRemaining := func() string {
		seconds := report.TimeRemaining.Seconds()
		if minutes := seconds / 60; minutes > 0 {
			return fmt.Sprintf("%.0f mins", math.Ceil(minutes))
		}
		if seconds == 1 {
			return fmt.Sprintf("1 sec")
		}
		return fmt.Sprintf("%.0f secs", math.Ceil(seconds))
	}()

	reporter.log.Infof("Syncing %.2f%% complete, remaining %s. Current stage: %s, %d/%d blocks processed.",
		report.PercentageProgress, timeRemaining, report.CurrentStage, report.CurrentHeight, report.TargetHeight)

}
