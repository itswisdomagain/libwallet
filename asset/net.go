package asset

import (
	"fmt"
	"strings"
)

// Network flags passed to asset backends to signify which network to use.
type Network uint8

const (
	Mainnet Network = iota
	Testnet
	Regtest
)

// Simnet is an alias of Regtest.
const Simnet = Regtest

// String returns the string representation of a Network.
func (n Network) String() string {
	switch n {
	case Mainnet:
		return "mainnet"
	case Testnet:
		return "testnet"
	case Simnet:
		return "simnet"
	}
	return ""
}

// NetFromString returns the Network for the given network name.
func NetFromString(net string) (Network, error) {
	switch strings.ToLower(net) {
	case "mainnet":
		return Mainnet, nil
	case "testnet":
		return Testnet, nil
	case "regtest", "regnet", "simnet":
		return Simnet, nil
	}
	return 255, fmt.Errorf("unknown network %s", net)
}
