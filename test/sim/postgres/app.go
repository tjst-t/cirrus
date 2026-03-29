// Package postgres provides an embedded PostgreSQL server for cirrus-sim.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
)

// Config holds configuration for the embedded PostgreSQL server.
type Config struct {
	Port     uint32
	Database string
	Username string
	Password string
	DataDir  string // empty = use temp dir (deleted on stop)
	Logger   io.Writer
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Port:     5432,
		Database: "cirrus",
		Username: "cirrus",
		Password: "cirrus",
	}
}

// Server wraps an embedded PostgreSQL instance with a management HTTP API.
type Server struct {
	db         *embeddedpostgres.EmbeddedPostgres
	httpServer *http.Server
	config     Config
	connURL    string
	mgmtPort   string
	tmpDataDir string
	logger     *slog.Logger
}

// New creates a new embedded PostgreSQL server.
// port is the PostgreSQL listen port. mgmtPort is the HTTP management API port.
func New(port, mgmtPort string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	cfg := DefaultConfig()

	var p uint32
	if _, err := fmt.Sscanf(port, "%d", &p); err == nil && p > 0 {
		cfg.Port = p
	}

	if v := os.Getenv("POSTGRES_DATABASE"); v != "" {
		cfg.Database = v
	}
	if v := os.Getenv("POSTGRES_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := os.Getenv("POSTGRES_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	return &Server{
		config:   cfg,
		mgmtPort: mgmtPort,
		logger:   logger,
	}
}

// Start starts the embedded PostgreSQL server and the management HTTP API.
func (s *Server) Start() {
	dataDir := s.config.DataDir
	if dataDir == "" {
		tmpDir, err := os.MkdirTemp("", "cirrus-sim-pg-*")
		if err != nil {
			s.logger.Error("failed to create temp data dir", "error", err)
			return
		}
		dataDir = filepath.Join(tmpDir, "data")
		s.tmpDataDir = tmpDir
	}

	logWriter := s.config.Logger
	if logWriter == nil {
		logWriter = io.Discard
	}

	epConfig := embeddedpostgres.DefaultConfig().
		Port(s.config.Port).
		Database(s.config.Database).
		Username(s.config.Username).
		Password(s.config.Password).
		DataPath(dataDir).
		StartTimeout(60 * time.Second).
		Logger(logWriter)

	s.connURL = epConfig.GetConnectionURL()
	s.db = embeddedpostgres.NewDatabase(epConfig)

	s.logger.Info("postgres starting",
		"port", s.config.Port,
		"database", s.config.Database,
		"user", s.config.Username,
		"data_dir", dataDir,
	)

	if err := s.db.Start(); err != nil {
		s.logger.Error("postgres failed to start", "error", err)
		return
	}

	s.logger.Info("postgres ready", "url", s.connURL)

	// Start management HTTP API
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sim/stats", s.handleStats)
	mux.HandleFunc("GET /sim/tables", s.handleListTables)
	mux.HandleFunc("GET /sim/tables/{name}", s.handleGetTable)
	mux.HandleFunc("GET /sim/tables/{name}/schema", s.handleGetSchema)

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%s", s.mgmtPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		s.logger.Info("postgres management API starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("postgres management API failed", "error", err)
		}
	}()
}

// Shutdown stops the management API and the embedded PostgreSQL server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		s.httpServer.Shutdown(ctx)
	}

	if s.db == nil {
		return nil
	}

	s.logger.Info("postgres stopping")
	if err := s.db.Stop(); err != nil {
		return fmt.Errorf("stop postgres: %w", err)
	}

	if s.tmpDataDir != "" {
		os.RemoveAll(s.tmpDataDir)
	}

	s.logger.Info("postgres stopped")
	return nil
}

// ConnectionURL returns the PostgreSQL connection URL.
func (s *Server) ConnectionURL() string {
	return s.connURL
}

// ── Management API handlers ──

func (s *Server) openDB() (*sql.DB, error) {
	return sql.Open("postgres", s.connURL+"?sslmode=disable")
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	db, err := s.openDB()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	var tableCount int
	err = db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tableCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var dbSize string
	err = db.QueryRow(`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize)
	if err != nil {
		dbSize = "unknown"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"database":    s.config.Database,
		"port":        s.config.Port,
		"table_count": tableCount,
		"db_size":     dbSize,
		"url":         s.connURL,
	})
}

func (s *Server) handleListTables(w http.ResponseWriter, _ *http.Request) {
	db, err := s.openDB()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT t.table_name,
		       (SELECT count(*) FROM information_schema.columns c WHERE c.table_name = t.table_name AND c.table_schema = 'public') as column_count
		FROM information_schema.tables t
		WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		ORDER BY t.table_name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type tableInfo struct {
		Name        string `json:"name"`
		ColumnCount int    `json:"column_count"`
		RowCount    int64  `json:"row_count"`
	}

	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.Name, &t.ColumnCount); err != nil {
			continue
		}
		// Get row count (use estimate for large tables)
		db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %q`, t.Name)).Scan(&t.RowCount)
		tables = append(tables, t)
	}

	if tables == nil {
		tables = []tableInfo{}
	}

	writeJSON(w, http.StatusOK, tables)
}

func (s *Server) handleGetTable(w http.ResponseWriter, r *http.Request) {
	tableName := r.PathValue("name")
	if tableName == "" {
		writeError(w, http.StatusBadRequest, "table name required")
		return
	}

	db, err := s.openDB()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// Verify table exists
	var exists bool
	err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`, tableName).Scan(&exists)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("table %q not found", tableName))
		return
	}

	// Get total count
	var totalCount int64
	db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %q`, tableName)).Scan(&totalCount)

	// Limit rows
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
		if limit <= 0 || limit > 1000 {
			limit = 100
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}

	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM %q LIMIT %d OFFSET %d`, tableName, limit, offset))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[col] = v
		}
		results = append(results, row)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"table":   tableName,
		"columns": columns,
		"total":   totalCount,
		"limit":   limit,
		"offset":  offset,
		"rows":    results,
	})
}

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	tableName := r.PathValue("name")
	if tableName == "" {
		writeError(w, http.StatusBadRequest, "table name required")
		return
	}

	db, err := s.openDB()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, tableName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type columnInfo struct {
		Name     string  `json:"name"`
		Type     string  `json:"type"`
		Nullable string  `json:"nullable"`
		Default  *string `json:"default"`
	}

	var columns []columnInfo
	for rows.Next() {
		var c columnInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default); err != nil {
			continue
		}
		columns = append(columns, c)
	}

	if columns == nil {
		columns = []columnInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"table":   tableName,
		"columns": columns,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
