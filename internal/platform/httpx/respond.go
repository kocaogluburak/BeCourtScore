package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode", "err", err)
	}
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

func Decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
