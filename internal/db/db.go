// Package db — SQLite ulanishi va schema migration'lari.
//
// modernc.org/sqlite ishlatilgan — pure-Go SQLite implementatsiyasi.
// CGo kerakmas, cross-compile va static link osongina ishlaydi.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open — SQLite faylini ochadi yoki yaratadi va migration'larni qo'llaydi.
//
// DSN'ga `?_pragma=journal_mode=WAL` qo'shilgan — bu bir vaqtda yozish va o'qishni yengillashtiradi
// (UI status'larni o'qisa, biror joyda yozuv jarayoni bo'lsa).
func Open(filePath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", filePath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite ochilmadi: %w", err)
	}

	// Connection ishlayotganini tasdiqlash
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sqlite ping xatoligi: %w", err)
	}

	// SQLite uchun bitta connection'da yozish optimal
	conn.SetMaxOpenConns(1)

	// Migration'larni qo'llash
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migration xatoligi: %w", err)
	}

	return conn, nil
}

// schemaVersion — joriy schema versiyasi. Yangi migration qo'shilsa, bu raqamni oshiring.
const schemaVersion = 2

// migrate — schema versiyasini tekshiradi va kerakli SQL'larni ishga tushiradi.
func migrate(conn *sql.DB) error {
	// Versiya jadvali
	if _, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS _schema_version (
			version INTEGER PRIMARY KEY
		)
	`); err != nil {
		return err
	}

	var current int
	row := conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM _schema_version")
	if err := row.Scan(&current); err != nil {
		return err
	}

	if current >= schemaVersion {
		return nil
	}

	// V1 — boshlang'ich tuzilma
	if current < 1 {
		if err := applyV1(conn); err != nil {
			return fmt.Errorf("v1 migration: %w", err)
		}
		if _, err := conn.Exec("INSERT INTO _schema_version(version) VALUES(1)"); err != nil {
			return err
		}
	}

	// V2 — mavjud stream key'larni tozalash (RTMP URL kiritilgan bo'lsa)
	if current < 2 {
		if err := applyV2(conn); err != nil {
			return fmt.Errorf("v2 migration: %w", err)
		}
		if _, err := conn.Exec("INSERT INTO _schema_version(version) VALUES(2)"); err != nil {
			return err
		}
	}

	return nil
}

// applyV2 — eski versiyalarda stream key sifatida noto'g'ri saqlangan RTMP URL'larni tozalaydi.
// Misol: "rtmp://a.rtmp.youtube.com/live2/abc-def" → "abc-def"
func applyV2(conn *sql.DB) error {
	rows, err := conn.Query(`SELECT id, stream_key FROM streams WHERE stream_key LIKE 'rtmp%'`)
	if err != nil {
		return err
	}
	type update struct {
		id  int64
		key string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return err
		}
		// Oxirgi `/` dan keyin nima borligini olamiz
		newKey := key
		for i := len(key) - 1; i >= 0; i-- {
			if key[i] == '/' {
				newKey = key[i+1:]
				break
			}
		}
		if newKey != key {
			updates = append(updates, update{id, newKey})
		}
	}
	rows.Close()

	for _, u := range updates {
		if _, err := conn.Exec(`UPDATE streams SET stream_key = ? WHERE id = ?`, u.key, u.id); err != nil {
			return err
		}
	}
	return nil
}

// applyV1 — boshlang'ich jadvalar.
func applyV1(conn *sql.DB) error {
	stmts := []string{
		`CREATE TABLE cameras (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			vendor          TEXT NOT NULL DEFAULT 'hikvision',
			host            TEXT NOT NULL,
			port            INTEGER NOT NULL DEFAULT 554,
			username        TEXT NOT NULL DEFAULT '',
			password        TEXT NOT NULL DEFAULT '',
			channel         INTEGER NOT NULL DEFAULT 1,
			use_sub_stream  INTEGER NOT NULL DEFAULT 0,
			raw_rtsp_url    TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		)`,
		`CREATE TABLE streams (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			name                TEXT NOT NULL,
			layout              TEXT NOT NULL DEFAULT 'single',
			camera_ids          TEXT NOT NULL DEFAULT '[]',  -- JSON array of int64
			quality             TEXT NOT NULL DEFAULT '1080p30',
			encoder             TEXT NOT NULL DEFAULT 'auto',
			audio_mode          TEXT NOT NULL DEFAULT 'first',
			audio_camera_index  INTEGER NOT NULL DEFAULT 0,
			platform            TEXT NOT NULL DEFAULT 'youtube',
			stream_key          TEXT NOT NULL DEFAULT '',
			custom_url          TEXT NOT NULL DEFAULT '',
			auto_restart        INTEGER NOT NULL DEFAULT 1,
			max_restarts        INTEGER NOT NULL DEFAULT 0,
			restart_delay_ms    INTEGER NOT NULL DEFAULT 5000,
			created_at          TEXT NOT NULL,
			updated_at          TEXT NOT NULL
		)`,
		`CREATE INDEX idx_streams_platform ON streams(platform)`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			return fmt.Errorf("%s: %w", s[:40], err)
		}
	}
	return nil
}
