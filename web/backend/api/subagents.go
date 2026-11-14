package api

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	ppid "github.com/sipeed/picoclaw/pkg/pid"
)

// registerSubagentRoutes proxies the gateway's internal subagent monitor API
// behind the dashboard session. The gateway token never reaches the browser.
func (h *Handler) registerSubagentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/subagents/", h.handleSubagentsProxy())
}

func (h *Handler) handleSubagentsProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gatewayAvailableForProxy() {
			writeSubagentProxyError(w, http.StatusServiceUnavailable, "gateway_unavailable",
				"The gateway is not running. Start it to monitor subagents.")
			return
		}

		pidData := ppid.ReadPidFileWithCheck(globalConfigDir())
		if pidData == nil || strings.TrimSpace(pidData.Token) == "" {
			writeSubagentProxyError(w, http.StatusServiceUnavailable, "gateway_token_unavailable",
				"The gateway session token is unavailable. Restart the gateway and try again.")
			return
		}
		token := pidData.Token

		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				target := h.gatewayProxyURL()
				pr.SetURL(target)
				pr.Out.URL.Path = "/subagents/" + strings.TrimPrefix(pr.In.URL.Path, "/api/subagents/")
				pr.Out.URL.RawPath = ""
				pr.Out.Header.Set("Authorization", "Bearer "+token)
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				logger.Errorf("Failed to proxy subagent API: %v", err)
				writeSubagentProxyError(w, http.StatusBadGateway, "gateway_error",
					"The gateway could not handle the request.")
			},
		}
		proxy.ServeHTTP(w, r)
	}
}

func writeSubagentProxyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
