package syncutils

import "time"

type SyncProgressReport struct {
	CurrentStage       SyncStage
	CurrentHeight      int32
	TargetHeight       int32
	PercentageProgress float64
	TimeRemaining      time.Duration
}

type SyncProgressListener interface {
	OnSyncStarted()
	OnPeerConnectedOrDisconnected(numberOfConnectedPeers uint32)
	OnProgress(report *SyncProgressReport)
	// OnSyncCompleted means wallet has caught up with main chain, not that
	// wallet synchronization has stopped. There's no listener for when wallet
	// disconnects from the network.
	OnSyncCompleted()
}

type BlockWithTxs[Tx any] struct {
	Height int32
	Hash   string
	Txs    []*Tx
}

type TxAndBlockNtfnListener[Tx any] interface {
	OnTxOrBlockUpdate(unminedTxs []*Tx, blks []*BlockWithTxs[Tx])
}

type RescanProgress struct {
	ScannedThrough int32
	Err            error
}

type BlocksRescanListener interface {
	// nil report means rescan has ended, check err.
	OnBlocksRescanProgress(report *SyncActivityReport, err error)
}
