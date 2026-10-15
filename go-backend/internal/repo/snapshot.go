package repo

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"piguard/go-backend/internal/model"
)

type SnapshotRepository interface {
	Find(context.Context, string) (*model.Snapshot, error)
	FindFrame(context.Context, string, string) (*model.Snapshot, error)
	FindLatest(context.Context, string) (*model.Snapshot, error)
	// Accept atomically resolves identity and decides whether this is the latest preview.
	Accept(context.Context, *model.Snapshot) (row *model.Snapshot, created, latest bool, err error)
}
type snapshotRepo struct{ db *gorm.DB }

func NewSnapshotRepository(db *gorm.DB) SnapshotRepository { return &snapshotRepo{db: db} }
func snapshotResult(q *gorm.DB) (*model.Snapshot, error) {
	var row model.Snapshot
	err := q.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func (r *snapshotRepo) Find(ctx context.Context, id string) (*model.Snapshot, error) {
	return snapshotResult(r.db.WithContext(ctx).Where("snapshot_id = ?", id))
}
func (r *snapshotRepo) FindFrame(ctx context.Context, device, frame string) (*model.Snapshot, error) {
	return snapshotResult(r.db.WithContext(ctx).Where("device_id = ? AND frame_id = ?", device, frame))
}
func latestSnapshot(q *gorm.DB, device string) (*model.Snapshot, error) {
	return snapshotResult(q.Where("device_id = ? AND type = ?", device, "preview").Order("captured_seconds DESC, captured_nanosecond DESC, id DESC"))
}
func (r *snapshotRepo) FindLatest(ctx context.Context, device string) (*model.Snapshot, error) {
	return latestSnapshot(r.db.WithContext(ctx), device)
}
func (r *snapshotRepo) Accept(ctx context.Context, input *model.Snapshot) (row *model.Snapshot, created, latest bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "device_id"}, {Name: "frame_id"}}, DoNothing: true}).Create(input)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var e error
			row, e = snapshotResult(tx.Where("device_id = ? AND frame_id = ?", input.DeviceID, *input.FrameID))
			return e
		}
		row, created = input, true
		if input.Type == "preview" {
			current, e := latestSnapshot(tx, input.DeviceID)
			if e != nil {
				return e
			}
			latest = current.SnapshotID == input.SnapshotID
		}
		return ctx.Err()
	})
	return
}
