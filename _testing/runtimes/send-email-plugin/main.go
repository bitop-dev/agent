package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
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
	addr := flag.String("addr", "127.0.0.1:8091", "listen address")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/send-email", handleSendEmail)
	mux.HandleFunc("/draft-email", handleDraftEmail)
	log.Printf("send-email plugin listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleSendEmail(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	to := stringArg(req.Arguments, "to")
	subject := stringArg(req.Arguments, "subject")
	if strings.EqualFold(stringArg(req.Config, "provider"), "smtp") {
		if err := sendSMTP(req.Config, to, subject, stringArg(req.Arguments, "body")); err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, response{
		Output: fmt.Sprintf("sent email to %s with subject %q", to, subject),
		Data: map[string]any{
			"status":  "accepted",
			"to":      to,
			"subject": subject,
		},
	})
}

func handleDraftEmail(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	body := stringArg(req.Arguments, "body")
	writeJSON(w, http.StatusOK, response{
		Output: fmt.Sprintf("drafted email: %s", body),
		Data:   map[string]any{"status": "drafted"},
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

func intArg(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(n))
		return parsed
	default:
		return 0
	}
}

func sendSMTP(cfg map[string]any, to, subject, body string) error {
	host := stringArg(cfg, "smtpHost")
	port := intArg(cfg, "smtpPort")
	username := stringArg(cfg, "username")
	password := stringArg(cfg, "password")
	from := stringArg(cfg, "from")
	if from == "" {
		from = username
	}
	if host == "" || port == 0 || username == "" || password == "" || from == "" || to == "" {
		return fmt.Errorf("smtp config requires smtpHost, smtpPort, username, password, from, and recipient")
	}
	message := []byte(fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", to, from, subject, body))
	addr := fmt.Sprintf("%s:%d", host, port)
	auth := smtp.PlainAuth("", username, password, host)
	if port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Quit()
		if err := client.Auth(auth); err != nil {
			return err
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write(message); err != nil {
			return err
		}
		return writer.Close()
	}
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Quit()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		return err
	}
	return writer.Close()
}
