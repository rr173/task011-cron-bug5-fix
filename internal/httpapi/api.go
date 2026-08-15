package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"task011-cron/internal/cron"
)

var ErrBadJSON = errors.New("请求体不是合法的单个 JSON 对象")

// API 是 Cron 服务的 HTTP 接口层。
type API struct{}

// New 创建默认 API。
func New() *API { return &API{} }

// Handler 返回所有路由。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/cron/parse", a.parse)
	mux.HandleFunc("POST /api/cron/next", a.next)
	mux.HandleFunc("POST /api/cron/validate", a.validate)
	return mux
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return ErrBadJSON
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return ErrBadJSON
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 将 cron 层错误映射为合适的 4xx 状态码，绝不返回 5xx。
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, cron.ErrNoOccur):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrBadJSON):
		status = http.StatusBadRequest
	default:
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{"error": err.Error(), "status": status})
}

type exprReq struct {
	Expr string `json:"expr"`
}

type nextReq struct {
	Expr string `json:"expr"`
	From string `json:"from"`
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) parse(w http.ResponseWriter, r *http.Request) {
	var req exprReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	e, err := cron.Parse(req.Expr)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"minute":       e.Minute.Values(),
		"hour":         e.Hour.Values(),
		"day_of_month": e.Dom.Values(),
		"month":        e.Month.Values(),
		"day_of_week":  e.Dow.Values(),
		"dom_star":     e.Dom.Star(),
		"dow_star":     e.Dow.Star(),
	})
}

func (a *API) next(w http.ResponseWriter, r *http.Request) {
	var req nextReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	e, err := cron.Parse(req.Expr)
	if err != nil {
		writeError(w, err)
		return
	}
	from, err := time.Parse(time.RFC3339, req.From)
	if err != nil {
		writeError(w, cron.ErrBadFromTime)
		return
	}
	nx, err := e.Next(from)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": nx.Format(time.RFC3339)})
}

func (a *API) validate(w http.ResponseWriter, r *http.Request) {
	var req exprReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if _, err := cron.Parse(req.Expr); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}
