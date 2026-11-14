package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/cron"
)

const (
	cronAPIBasePath      = "/cron/"
	maxCronRequestBytes  = 1 << 20
	maxCronNameLen       = 120
	maxCronMessageLen    = 8000
	maxCronChannelLen    = 64
	maxCronTargetLen     = 200
	cronJobNameFallback  = "Scheduled task"
	cronDefaultChannel   = "cli"
	cronDefaultRecipient = "direct"
)

type cronAPIHandler struct {
	svc         *cron.CronService
	token       string
	toolEnabled bool
}

func newCronAPIHandler(svc *cron.CronService, token string, toolEnabled bool) http.Handler {
	return &cronAPIHandler{svc: svc, token: token, toolEnabled: toolEnabled}
}

// registerCronAPI mounts the internal cron management API on the shared
// gateway mux. The launcher proxies these endpoints behind dashboard auth and
// injects the process bearer token.
func registerCronAPI(manager *channels.Manager, svc *cron.CronService, token string, toolEnabled bool) {
	if manager == nil || svc == nil {
		return
	}
	manager.Mount(cronAPIBasePath, newCronAPIHandler(svc, token, toolEnabled))
}

func (h *cronAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeCronAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if h.svc == nil {
		writeCronAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Cron service is unavailable.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(cronAPIBasePath, "/"))
	path = strings.Trim(path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}

	switch {
	case len(segments) == 1 && segments[0] == "jobs":
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreate(w, r)
		default:
			writeCronAPIMethodNotAllowed(w)
		}
	case len(segments) == 2 && segments[0] == "jobs":
		h.handleJob(w, r, segments[1])
	case len(segments) == 3 && segments[0] == "jobs" && segments[2] == "enable":
		h.handleEnable(w, r, segments[1])
	case len(segments) == 3 && segments[0] == "jobs" && segments[2] == "run":
		h.handleRun(w, r, segments[1])
	default:
		writeCronAPIError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	}
}

func (h *cronAPIHandler) authorized(r *http.Request) bool {
	if h.token == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	given, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || given == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(h.token)) == 1
}

func (h *cronAPIHandler) handleList(w http.ResponseWriter, r *http.Request) {
	jobs := h.svc.ListJobs(true)
	writeCronAPIJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
		"status": map[string]any{
			"running":         h.svc.Status()["enabled"],
			"tool_enabled":    h.toolEnabled,
			"jobs":            len(jobs),
			"next_wake_at_ms": h.svc.Status()["nextWakeAtMS"],
			"gateway_time_ms": time.Now().UnixMilli(),
		},
	})
}

func (h *cronAPIHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	input, ok := h.decodeInput(w, r)
	if !ok {
		return
	}

	schedule := normalizeCronSchedule(input.Schedule)
	if err := cron.ValidateSchedule(schedule, time.Now()); err != nil {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
		return
	}

	job, err := h.svc.AddJob(
		normalizeCronName(input.Name),
		schedule,
		input.Message,
		normalizeCronChannel(input.Channel),
		normalizeCronTarget(input.To),
	)
	if err != nil {
		writeCronAPIError(w, http.StatusInternalServerError, "save_failed", "Failed to save the scheduled task.")
		return
	}
	if input.Enabled != nil && !*input.Enabled {
		if updated := h.svc.EnableJob(job.ID, false); updated != nil {
			job = updated
		}
	}

	writeCronAPIJSON(w, http.StatusCreated, job)
}

func (h *cronAPIHandler) handleJob(w http.ResponseWriter, r *http.Request, jobID string) {
	switch r.Method {
	case http.MethodGet:
		job, found := h.svc.GetJob(jobID)
		if !found {
			writeCronAPIError(w, http.StatusNotFound, "not_found", "Scheduled task not found.")
			return
		}
		writeCronAPIJSON(w, http.StatusOK, job)
	case http.MethodPut:
		h.handleUpdate(w, r, jobID)
	case http.MethodDelete:
		if !h.svc.RemoveJob(jobID) {
			writeCronAPIError(w, http.StatusNotFound, "not_found", "Scheduled task not found.")
			return
		}
		writeCronAPIJSON(w, http.StatusOK, map[string]any{"removed": true, "id": jobID})
	default:
		writeCronAPIMethodNotAllowed(w)
	}
}

func (h *cronAPIHandler) handleUpdate(w http.ResponseWriter, r *http.Request, jobID string) {
	job, found := h.svc.GetJob(jobID)
	if !found {
		writeCronAPIError(w, http.StatusNotFound, "not_found", "Scheduled task not found.")
		return
	}
	if strings.TrimSpace(job.Payload.Command) != "" {
		writeCronAPIError(w, http.StatusConflict, "command_job_read_only",
			"Command tasks created by the agent cannot be edited here. You can still run, disable or delete them.")
		return
	}

	input, ok := h.decodeInput(w, r)
	if !ok {
		return
	}

	schedule := normalizeCronSchedule(input.Schedule)
	if err := cron.ValidateSchedule(schedule, time.Now()); err != nil {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
		return
	}

	job.Name = normalizeCronName(input.Name)
	job.Schedule = schedule
	job.Payload.Message = input.Message
	job.Payload.Channel = normalizeCronChannel(input.Channel)
	job.Payload.To = normalizeCronTarget(input.To)
	if input.Enabled != nil {
		job.Enabled = *input.Enabled
	}

	if err := h.svc.UpdateJob(job); err != nil {
		writeCronAPIError(w, http.StatusInternalServerError, "save_failed", "Failed to save the scheduled task.")
		return
	}

	updated, _ := h.svc.GetJob(jobID)
	writeCronAPIJSON(w, http.StatusOK, updated)
}

func (h *cronAPIHandler) handleEnable(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeCronAPIMethodNotAllowed(w)
		return
	}

	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeCronJSON(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_request", "enabled is required.")
		return
	}

	job := h.svc.EnableJob(jobID, *body.Enabled)
	if job == nil {
		writeCronAPIError(w, http.StatusNotFound, "not_found", "Scheduled task not found.")
		return
	}
	writeCronAPIJSON(w, http.StatusOK, job)
}

func (h *cronAPIHandler) handleRun(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		writeCronAPIMethodNotAllowed(w)
		return
	}

	err := h.svc.RunNow(jobID)
	switch {
	case err == nil:
		writeCronAPIJSON(w, http.StatusAccepted, map[string]any{"queued": true, "id": jobID})
	case errors.Is(err, cron.ErrJobNotFound):
		writeCronAPIError(w, http.StatusNotFound, "not_found", "Scheduled task not found.")
	case errors.Is(err, cron.ErrNotRunning):
		writeCronAPIError(w, http.StatusConflict, "not_running",
			"The scheduler is not running. Start the gateway and try again.")
	default:
		writeCronAPIError(w, http.StatusInternalServerError, "run_failed", "Failed to queue the scheduled task.")
	}
}

type cronScheduleInput struct {
	Kind    string `json:"kind"`
	AtMS    *int64 `json:"at_ms"`
	EveryMS *int64 `json:"every_ms"`
	Expr    string `json:"expr"`
	TZ      string `json:"tz"`
}

func (in cronScheduleInput) toSchedule() cron.CronSchedule {
	return cron.CronSchedule{
		Kind:    in.Kind,
		AtMS:    in.AtMS,
		EveryMS: in.EveryMS,
		Expr:    in.Expr,
		TZ:      in.TZ,
	}
}

type cronJobInput struct {
	Name     string            `json:"name"`
	Message  string            `json:"message"`
	Schedule cronScheduleInput `json:"schedule"`
	Channel  string            `json:"channel"`
	To       string            `json:"to"`
	Enabled  *bool             `json:"enabled"`
}

func (h *cronAPIHandler) decodeInput(w http.ResponseWriter, r *http.Request) (cronJobInput, bool) {
	var input cronJobInput
	if !decodeCronJSON(w, r, &input) {
		return cronJobInput{}, false
	}

	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_request", "message is required.")
		return cronJobInput{}, false
	}
	if utf8.RuneCountInString(input.Message) > maxCronMessageLen {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_request",
			fmt.Sprintf("message must be at most %d characters.", maxCronMessageLen))
		return cronJobInput{}, false
	}
	return input, true
}

func normalizeCronSchedule(input cronScheduleInput) cron.CronSchedule {
	schedule := input.toSchedule()
	schedule.Kind = strings.ToLower(strings.TrimSpace(schedule.Kind))
	schedule.Expr = strings.TrimSpace(schedule.Expr)
	schedule.TZ = strings.TrimSpace(schedule.TZ)
	return schedule
}

func normalizeCronName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if utf8.RuneCountInString(name) > maxCronNameLen {
		name = string([]rune(name)[:maxCronNameLen])
	}
	if name == "" {
		return cronJobNameFallback
	}
	return name
}

func normalizeCronChannel(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return cronDefaultChannel
	}
	if len(value) > maxCronChannelLen {
		value = value[:maxCronChannelLen]
	}
	return value
}

func normalizeCronTarget(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return cronDefaultRecipient
	}
	if len(value) > maxCronTargetLen {
		value = value[:maxCronTargetLen]
	}
	return value
}

func decodeCronJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxCronRequestBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		writeCronAPIError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request body.")
		return false
	}
	return true
}

func writeCronAPIMethodNotAllowed(w http.ResponseWriter) {
	writeCronAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
}

func writeCronAPIJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeCronAPIError(w http.ResponseWriter, status int, code, message string) {
	writeCronAPIJSON(w, status, map[string]any{"code": code, "error": message})
}
