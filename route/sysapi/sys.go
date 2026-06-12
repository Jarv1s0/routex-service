package sysapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"routex-service/route/httphelper"
	"routex-service/sys"

	"github.com/go-chi/chi/v5"
)

type dnsRequest struct {
	Device  string   `json:"device,omitempty"`
	Servers []string `json:"servers,omitempty"`
}

type tunCleanupRequest struct {
	Device       string   `json:"device,omitempty"`
	FakeIPRanges []string `json:"fake_ip_ranges,omitempty"`
}

type tunCleanupResponse struct {
	Status string                `json:"status"`
	Result *sys.TunCleanupResult `json:"result"`
}

func Router() http.Handler {
	r := chi.NewRouter()

	r.Post("/dns/set", setDns)
	r.Post("/tun/cleanup", cleanupTun)

	return r
}

func setDns(w http.ResponseWriter, r *http.Request) {
	var req dnsRequest
	if err := httphelper.DecodeRequest(r, &req); err != nil {
		httphelper.SendError(w, httphelper.BadRequest(fmt.Sprintf("无效的请求体: %v", err)))
		return
	}
	if err := sys.SetDns(req.Device, req.Servers); err != nil {
		httphelper.SendError(w, err)
		return
	}
	httphelper.SendJSON(w, "success", "DNS 设置成功")
}

func cleanupTun(w http.ResponseWriter, r *http.Request) {
	var req tunCleanupRequest
	if err := httphelper.DecodeRequest(r, &req); err != nil {
		httphelper.SendError(w, httphelper.BadRequest(fmt.Sprintf("无效的请求体: %v", err)))
		return
	}
	result, err := sys.CleanupTun(sys.TunCleanupOptions{
		Device:       req.Device,
		FakeIPRanges: req.FakeIPRanges,
	})
	if err != nil {
		httphelper.SendError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tunCleanupResponse{Status: "success", Result: result}); err != nil {
		httphelper.SendError(w, err)
	}
}
