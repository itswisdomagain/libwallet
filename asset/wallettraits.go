package asset

// WalletTrait is a bitset indicating various optional wallet properties, such
// as if the wallet was restored or if it's watch only.
type WalletTrait uint64

const (
	WalletTraitRestored  WalletTrait = 1 << iota // The Wallet is a restored wallet.
	WalletTraitWatchOnly                         // The Wallet is a watch only wallet.
)

func isRestored(t WalletTrait) bool {
	return t&WalletTraitRestored == WalletTraitRestored
}

func isWatchOnly(t WalletTrait) bool {
	return t&WalletTraitWatchOnly == WalletTraitWatchOnly
}
