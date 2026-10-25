// user-manager: a small REST service that lists and deletes users of the auth database.
//
//	GET    /auth/users?page=1&page_size=10   paginated list of users
//	DELETE /auth/user?id=3                   delete a user by id
//	DELETE /auth/user?email=a@b.com          delete a user by email
//	GET    /healthz                          liveness/readiness (pings the database)
//
// It talks ONLY to the auth MySQL database (table "users" created by the auth service).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	defaultPageSize = 10
	maxPageSize     = 100
	queryTimeout    = 5 * time.Second
)

type user struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type usersPage struct {
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	Total      int    `json:"total"`
	TotalPages int    `json:"total_pages"`
	Users      []user `json:"users"`
}

type server struct {
	db *sql.DB
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{"error": msg, "status": status})
}

func intParam(r *http.Request, name string, def, min, max int) (int, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, min, max)
	}
	return n, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

// ---------- handlers ----------

// GET /auth/users
func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use GET")
		return
	}

	page, err := intParam(r, "page", 1, 1, 1000000)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pageSize, err := intParam(r, "page_size", defaultPageSize, 1, maxPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		log.Printf("count users: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, email FROM users ORDER BY id LIMIT ? OFFSET ?", pageSize, offset)
	if err != nil {
		log.Printf("list users: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	users := make([]user, 0, pageSize)
	for rows.Next() {
		var id int64
		var email sql.NullString
		if err := rows.Scan(&id, &email); err != nil {
			log.Printf("scan user: %v", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		users = append(users, user{ID: id, Email: email.String})
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate users: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, usersPage{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: (total + pageSize - 1) / pageSize,
		Users:      users,
	})
}

// DELETE /auth/user?id=<n>  or  DELETE /auth/user?email=<address>
func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use DELETE")
		return
	}

	idStr := strings.TrimSpace(r.URL.Query().Get("id"))
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if (idStr == "") == (email == "") {
		writeError(w, http.StatusBadRequest, "provide exactly one query parameter: id or email")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	var res sql.Result
	var err error
	if idStr != "" {
		id, perr := strconv.ParseInt(idStr, 10, 64)
		if perr != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "id must be a positive integer")
			return
		}
		res, err = s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	} else {
		res, err = s.db.ExecContext(ctx, "DELETE FROM users WHERE email = ?", email)
	}
	if err != nil {
		log.Printf("delete user: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		log.Printf("rows affected: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "user deleted",
		"deleted_count": n,
	})
}

// GET /healthz - also proves that the database is reachable
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		log.Printf("health check: database unreachable: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "up"})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "route not found")
}

// ---------- middleware ----------

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}

// ---------- main ----------

func main() {
	cfg := mysql.NewConfig()
	cfg.User = mustGetenv("DB_USER")
	cfg.Passwd = mustGetenv("DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = mustGetenv("DB_HOST") + ":" + getenv("DB_PORT", "3306")
	cfg.DBName = mustGetenv("DB_NAME")
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second

	// sql.Open does not connect; the first query does. The service therefore starts even if
	// the database is still coming up, and /healthz reports the real state.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	s := &server{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/users", s.listUsers)
	mux.HandleFunc("/auth/user", s.deleteUser)
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/", notFound)

	srv := &http.Server{
		Addr:              ":" + getenv("HTTP_PORT", "8090"),
		Handler:           recoverMiddleware(logMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("user-manager listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	// graceful shutdown on SIGTERM (sent by Kubernetes when a Pod is stopped)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
