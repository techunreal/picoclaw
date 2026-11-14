package api

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	ppid "github.com/sipeed/picoclaw/pkg/pid"
)

// registerCronRoutes proxies the gateway's internal cron management API behind
// the dashboard session. The gateway token never reaches the browser.
func (h *Handler) registerCronRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cron/", h.handleCronProxy())
}

func (h *Handler) handleCronProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gatewayAvailableForProxy() {
			writeCronProxyError(w, http.StatusServiceUnavailable, "gateway_unavailable",
				"The gateway is not running. Start it to manage scheduled tasks.")
			return
		}

		pidData := ppid.ReadPidFileWithCheck(globalConfigDir())
		if pidData == nil || strings.TrimSpace(pidData.Token) == "" {
			writeCronProxyError(w, http.StatusServiceUnavailable, "gateway_token_unavailable",
				"The gateway session token is unavailable. Restart the gateway and try again.")
			return
		}
		token := pidData.Token

		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				target := h.gatewayProxyURL()
				pr.SetURL(target)
				pr.Out.URL.Path = "/cron/" + strings.TrimPrefix(pr.In.URL.Path, "/api/cron/")
				pr.Out.URL.RawPath = ""
				pr.Out.Header.Set("Authorization", "Bearer "+token)
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				logger.Errorf("Failed to proxy cron API: %v", err)
				writeCronProxyError(w, http.StatusBadGateway, "gateway_error",
					"The gateway could not handle the request.")
			},
		}
		proxy.ServeHTTP(w, r)
	}
}

func writeCronProxyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
