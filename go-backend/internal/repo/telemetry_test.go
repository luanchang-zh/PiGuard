package repo_test

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
	"testing"
	"time"
)

func TestTelemetryMigrationPreservesExistingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	old, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Exec(`CREATE TABLE telemetry (id integer PRIMARY KEY AUTOINCREMENT, device_id text NOT NULL, timestamp datetime NOT NULL, seq integer NOT NULL, speed real, distance real, temperature real, gyro_x real, gyro_y real, gyro_z real, yaw_rate real, lane_offset real, risk_level text)`).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if err := old.Exec(`INSERT INTO telemetry (device_id,timestamp,seq,speed,risk_level) VALUES (?,?,?,?,?)`, "car-001", at, 1, 32.4, "normal").Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := old.DB()
	_ = sql.Close()
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ = database.DB()
	defer sql.Close()
	history := repo.NewTelemetryRepository(database)
	rows, err := history.List(context.Background(), "car-001", repo.TelemetryQuery{Limit: 100})
	if err != nil || len(rows) != 1 || rows[0].Speed == nil || *rows[0].Speed != 32.4 {
		t.Fatalf("migration lost data: %+v %v", rows, err)
	}
	if err := history.Insert(context.Background(), &model.Telemetry{DeviceID: "car-001", Timestamp: at.Add(time.Second), Seq: 2, RiskLevel: "unknown"}); err != nil {
		t.Fatal(err)
	}
	latest, err := history.Latest(context.Background(), "car-001")
	if err != nil || latest.Seq != 2 || latest.Speed != nil {
		t.Fatal("missing measurements did not persist as NULL", latest, err)
	}
}
