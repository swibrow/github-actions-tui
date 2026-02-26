package cache

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages a SQLite cache database.
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// Open creates or opens the SQLite cache database at the given path.
// It creates parent directories, enables WAL mode, and runs migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("opening cache db: %w", err)
	}

	// Verify connection works
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging cache db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating cache db: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DefaultPath returns the default cache database path using os.UserCacheDir().
func DefaultPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gha", "cache.db"), nil
}

// --- Schema migrations ---

func (s *Store) migrate() error {
	// Create schema_version table if not exists
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}

	var version int
	err := s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	migrations := []func() error{
		s.migrateV1,
	}

	for i := version; i < len(migrations); i++ {
		if err := migrations[i](); err != nil {
			return fmt.Errorf("migration v%d: %w", i+1, err)
		}
		if _, err := s.db.Exec(`UPDATE schema_version SET version = ?`, i+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateV1() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS job_logs (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			job_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			cached_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (owner, repo, job_id)
		)`,
		`CREATE TABLE IF NOT EXISTS workflow_yaml (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			workflow_path TEXT NOT NULL,
			deps_json TEXT NOT NULL,
			cached_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (owner, repo, workflow_path)
		)`,
		`CREATE TABLE IF NOT EXISTS workflows (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			data_json TEXT NOT NULL,
			cached_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (owner, repo)
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			run_id INTEGER NOT NULL,
			data_json TEXT NOT NULL,
			status TEXT NOT NULL,
			cached_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (owner, repo, run_id)
		)`,
		`CREATE TABLE IF NOT EXISTS jobs (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			run_id INTEGER NOT NULL,
			data_json TEXT NOT NULL,
			cached_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (owner, repo, run_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// --- Job Logs ---

// GetJobLogs returns cached log content for a job, or empty string if not found/expired.
func (s *Store) GetJobLogs(owner, repo string, jobID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var content string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT content, expires_at FROM job_logs WHERE owner = ? AND repo = ? AND job_id = ?`,
		owner, repo, jobID,
	).Scan(&content, &expiresAt)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}
	return content, true
}

// PutJobLogs stores log content for a job.
func (s *Store) PutJobLogs(owner, repo string, jobID int64, content string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO job_logs (owner, repo, job_id, content, cached_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		owner, repo, jobID, content, now, now+int64(ttl.Seconds()),
	)
	if err != nil {
		log.Printf("cache: put job logs: %v", err)
	}
}

// --- Workflow YAML ---

// GetWorkflowYAML returns cached deps JSON for a workflow path.
func (s *Store) GetWorkflowYAML(owner, repo, path string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var depsJSON string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT deps_json, expires_at FROM workflow_yaml WHERE owner = ? AND repo = ? AND workflow_path = ?`,
		owner, repo, path,
	).Scan(&depsJSON, &expiresAt)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}
	return depsJSON, true
}

// PutWorkflowYAML stores deps JSON for a workflow path.
func (s *Store) PutWorkflowYAML(owner, repo, path, depsJSON string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO workflow_yaml (owner, repo, workflow_path, deps_json, cached_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		owner, repo, path, depsJSON, now, now+int64(ttl.Seconds()),
	)
	if err != nil {
		log.Printf("cache: put workflow yaml: %v", err)
	}
}

// --- Workflows ---

// GetWorkflows returns cached workflows JSON.
func (s *Store) GetWorkflows(owner, repo string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var dataJSON string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT data_json, expires_at FROM workflows WHERE owner = ? AND repo = ?`,
		owner, repo,
	).Scan(&dataJSON, &expiresAt)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}
	return dataJSON, true
}

// PutWorkflows stores workflows JSON.
func (s *Store) PutWorkflows(owner, repo, dataJSON string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO workflows (owner, repo, data_json, cached_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		owner, repo, dataJSON, now, now+int64(ttl.Seconds()),
	)
	if err != nil {
		log.Printf("cache: put workflows: %v", err)
	}
}

// --- Runs (individual completed runs) ---

// GetRun returns a cached single run JSON by run ID.
func (s *Store) GetRun(owner, repo string, runID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var dataJSON string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT data_json, expires_at FROM runs WHERE owner = ? AND repo = ? AND run_id = ?`,
		owner, repo, runID,
	).Scan(&dataJSON, &expiresAt)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}
	return dataJSON, true
}

// PutRun stores a single run.
func (s *Store) PutRun(owner, repo string, runID int64, status, dataJSON string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO runs (owner, repo, run_id, data_json, status, cached_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		owner, repo, runID, dataJSON, status, now, now+int64(ttl.Seconds()),
	)
	if err != nil {
		log.Printf("cache: put run: %v", err)
	}
}

// --- Jobs (all jobs for a run) ---

// GetJobs returns cached jobs JSON for a run.
func (s *Store) GetJobs(owner, repo string, runID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var dataJSON string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT data_json, expires_at FROM jobs WHERE owner = ? AND repo = ? AND run_id = ?`,
		owner, repo, runID,
	).Scan(&dataJSON, &expiresAt)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}
	return dataJSON, true
}

// PutJobs stores all jobs for a run.
func (s *Store) PutJobs(owner, repo string, runID int64, dataJSON string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO jobs (owner, repo, run_id, data_json, cached_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		owner, repo, runID, dataJSON, now, now+int64(ttl.Seconds()),
	)
	if err != nil {
		log.Printf("cache: put jobs: %v", err)
	}
}

// --- Cleanup ---

// DeleteExpired removes all expired entries from all tables.
func (s *Store) DeleteExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	tables := []string{"job_logs", "workflow_yaml", "workflows", "runs", "jobs"}
	for _, t := range tables {
		if _, err := s.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE expires_at < ?`, t), now); err != nil {
			log.Printf("cache: cleanup %s: %v", t, err)
		}
	}
}

// StartCleanup runs DeleteExpired periodically until done is closed.
func StartCleanup(s *Store, interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				s.DeleteExpired()
			}
		}
	}()
}
