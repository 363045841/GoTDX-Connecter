package client

import (
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bensema/gotdx"
)

var (
	mu       sync.Mutex
	instance *gotdx.Client
)

func Get() *gotdx.Client {
	mu.Lock()
	defer mu.Unlock()
	if instance == nil {
		instance = buildClient()
	}
	return instance
}

func Reprobe() error {
	mu.Lock()
	defer mu.Unlock()
	old := instance
	if old != nil {
		old.Disconnect()
	}
	newClient := buildClient()
	if newClient == nil {
		if old != nil {
			instance = old
		}
		return errors.New("Reprobe: no reachable hosts")
	}
	instance = newClient
	log.Println("[gotdx] re-probe complete, new client created")
	return nil
}

func buildClient() *gotdx.Client {
	mainHosts := resolveHosts("main", "GOTDX_MAIN_HOSTS", gotdx.MainHostAddresses(), time.Second*2)
	exHosts := resolveHosts("ex", "GOTDX_EX_HOSTS", gotdx.ExHostAddresses(), time.Second*2)

	if len(mainHosts) == 0 {
		log.Println("[gotdx] no reachable main hosts, using full default list")
		mainHosts = gotdx.MainHostAddresses()
	}
	if len(exHosts) == 0 {
		log.Println("[gotdx] no reachable ex hosts, using full default list")
		exHosts = gotdx.ExHostAddresses()
	}
	if len(mainHosts) == 0 {
		return nil
	}

	log.Printf("[gotdx] main hosts (%d): %v", len(mainHosts), mainHosts)
	log.Printf("[gotdx] ex hosts (%d): %v", len(exHosts), exHosts)

	opts := []gotdx.Option{
		gotdx.WithTCPAddress(mainHosts[0]),
		gotdx.WithTCPAddressPool(mainHosts[1:]...),
		gotdx.WithExTCPAddress(exHosts[0]),
		gotdx.WithExTCPAddressPool(exHosts[1:]...),
		gotdx.WithTimeoutSec(6),
	}
	if os.Getenv("GOTDX_AUTO_SELECT") == "1" {
		opts = append(opts, gotdx.WithAutoSelectFastest(true))
	}
	return gotdx.New(opts...)
}

func resolveHosts(label, envKey string, all []string, timeout time.Duration) []string {
	if v := os.Getenv(envKey); v != "" {
		addrs := strings.Split(v, ",")
		log.Printf("[gotdx] %s: using env %s=%v", label, envKey, addrs)
		return addrs
	}

	log.Printf("[gotdx] %s: probing %d hosts (timeout=%v)...", label, len(all), timeout)
	results := gotdx.ProbeAddresses(all, timeout)

	reachable := make([]string, 0, len(results))
	for _, r := range results {
		if r.Reachable {
			log.Printf("[gotdx] %s: %-21s reachable  latency=%v", label, r.Address, r.Latency)
			reachable = append(reachable, r.Address)
		} else {
			log.Printf("[gotdx] %s: %-21s unreachable", label, r.Address)
		}
	}
	log.Printf("[gotdx] %s: %d/%d hosts reachable", label, len(reachable), len(all))
	return reachable
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
