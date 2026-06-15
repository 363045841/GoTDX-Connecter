package api

import (
	"encoding/json"
	"net/http"
	"time"

	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx"
)

type hostProbeRequest struct {
	Type    string `json:"type"`
	Timeout int    `json:"timeout"`
}

type hostListResponse struct {
	Main   []string `json:"main"`
	Ex     []string `json:"ex"`
	MAC    []string `json:"mac"`
	MACEx  []string `json:"mac_ex"`
	Broker []string `json:"broker"`
}

func handleHostProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req hostProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	var hosts []gotdx.HostInfo
	switch req.Type {
	case "ex":
		hosts = client.ExHosts()
	case "mac":
		hosts = client.MACHosts()
	case "mac_ex":
		hosts = client.MACExHosts()
	case "broker":
		hosts = client.BrokerHosts()
	default:
		hosts = client.MainHosts()
	}

	results := client.Probe(hosts, timeout)
	writeJSON(w, http.StatusOK, results)
}

func handleHostList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	resp := hostListResponse{
		Main:   hostAddresses(client.MainHosts()),
		Ex:     hostAddresses(client.ExHosts()),
		MAC:    hostAddresses(client.MACHosts()),
		MACEx:  hostAddresses(client.MACExHosts()),
		Broker: hostAddresses(client.BrokerHosts()),
	}
	writeJSON(w, http.StatusOK, resp)
}

func hostAddresses(hosts []gotdx.HostInfo) []string {
	addrs := make([]string, len(hosts))
	for i, h := range hosts {
		addrs[i] = h.Address()
	}
	return addrs
}
