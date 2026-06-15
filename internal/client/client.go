package client

import (
	"sync"
	"time"

	"github.com/bensema/gotdx"
)

var (
	once     sync.Once
	instance *gotdx.Client
)

func Get() *gotdx.Client {
	once.Do(func() {
		mainHosts := gotdx.MainHostAddresses()
		exHosts := gotdx.ExHostAddresses()

		instance = gotdx.New(
			gotdx.WithTCPAddress(mainHosts[0]),
			gotdx.WithTCPAddressPool(mainHosts[1:]...),
			gotdx.WithExTCPAddress(exHosts[0]),
			gotdx.WithExTCPAddressPool(exHosts[1:]...),
			gotdx.WithAutoSelectFastest(true),
			gotdx.WithTimeoutSec(10),
		)
	})
	return instance
}

func Probe(hosts []gotdx.HostInfo, timeout time.Duration) []gotdx.HostProbeResult {
	return gotdx.ProbeHosts(hosts, timeout)
}

func Fastest(hosts []gotdx.HostInfo, timeout time.Duration) (gotdx.HostProbeResult, error) {
	return gotdx.FastestHost(hosts, timeout)
}

func MainHosts() []gotdx.HostInfo {
	return gotdx.MainHosts()
}

func ExHosts() []gotdx.HostInfo {
	return gotdx.ExHosts()
}

func MACHosts() []gotdx.HostInfo {
	return gotdx.MACHosts()
}

func MACExHosts() []gotdx.HostInfo {
	return gotdx.MACExHosts()
}

func BrokerHosts() []gotdx.HostInfo {
	return gotdx.BrokerHosts()
}
