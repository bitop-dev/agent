package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
)

type request struct {
	Plugin    string         `json:"plugin"`
	Tool      string         `json:"tool"`
	Operation string         `json:"operation"`
	Arguments map[string]any `json:"arguments"`
	Config    map[string]any `json:"config"`
}

type response struct {
	Output string         `json:"output,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
	Error  string         `json:"error,omitempty"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8092", "listen address")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/web-search", handleWebSearch)
	mux.HandleFunc("/web-fetch", handleWebFetch)
	log.Printf("web-research plugin listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleWebSearch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	query := stringArg(req.Arguments, "query")
	writeJSON(w, http.StatusOK, response{
		Output: fmt.Sprintf("search results for %q: result one, result two", query),
		Data: map[string]any{
			"results": []map[string]any{
				{"title": "Result One", "url": "https://example.com/one"},
				{"title": "Result Two", "url": "https://example.com/two"},
			},
		},
	})
}

func handleWebFetch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	url := stringArg(req.Arguments, "url")
	writeJSON(w, http.StatusOK, response{
		Output: fmt.Sprintf("fetched content from %s", url),
		Data: map[string]any{
			"url":      url,
			"contents": "Example fetched content for local testing.",
		},
	})
}

func decodeRequest(r *http.Request) (request, error) {
	defer r.Body.Close()
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return request{}, err
	}
	return req, nil
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func stringArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
