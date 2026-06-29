package readings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Reading struct {
	ID          string     `json:"id"`
	Systolic    int        `json:"systolic"`
	Diastolic   int        `json:"diastolic"`
	Pulse       *int       `json:"pulse,omitempty"`
	MeasuredAt  time.Time  `json:"measuredAt"`
	Note        *string    `json:"note,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

type SyncUpsert struct {
	LocalID    *int64     `json:"localId"`
	ServerID   *string    `json:"serverId"`
	Systolic   int        `json:"systolic"`
	Diastolic  int        `json:"diastolic"`
	Pulse      *int       `json:"pulse"`
	MeasuredAt time.Time  `json:"measuredAt"`
	Note       *string    `json:"note"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type SyncMapping struct {
	LocalID  int64  `json:"localId"`
	ServerID string `json:"serverId"`
	Status   string `json:"status"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, userID string, since *time.Time, limit int) ([]Reading, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id, systolic, diastolic, pulse, measured_at, note, created_at, updated_at, deleted_at
		FROM blood_pressure_readings
		WHERE user_id = $1 AND deleted_at IS NULL
	`
	args := []any{userID}
	if since != nil {
		query += ` AND updated_at > $2`
		args = append(args, *since)
	}
	query += ` ORDER BY measured_at DESC LIMIT ` + fmt.Sprintf("%d", limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list readings: %w", err)
	}
	defer rows.Close()

	var readings []Reading
	for rows.Next() {
		reading, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		readings = append(readings, reading)
	}
	return readings, rows.Err()
}

func (r *Repository) Get(ctx context.Context, userID, id string) (Reading, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, systolic, diastolic, pulse, measured_at, note, created_at, updated_at, deleted_at
		FROM blood_pressure_readings
		WHERE user_id = $1 AND id = $2 AND deleted_at IS NULL
	`, userID, id)
	reading, err := scanReading(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Reading{}, ErrNotFound
		}
		return Reading{}, err
	}
	return reading, nil
}

func (r *Repository) Create(ctx context.Context, userID string, input Reading) (Reading, error) {
	id := newUUID()
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO blood_pressure_readings(
			id, user_id, systolic, diastolic, pulse, measured_at, note, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, id, userID, input.Systolic, input.Diastolic, input.Pulse, input.MeasuredAt.UTC(), input.Note, now)
	if err != nil {
		return Reading{}, fmt.Errorf("create reading: %w", err)
	}
	return r.Get(ctx, userID, id)
}

func (r *Repository) Update(ctx context.Context, userID, id string, input Reading, clientUpdatedAt time.Time) (Reading, error) {
	var serverUpdatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT updated_at FROM blood_pressure_readings
		WHERE user_id = $1 AND id = $2 AND deleted_at IS NULL
	`, userID, id).Scan(&serverUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Reading{}, ErrNotFound
		}
		return Reading{}, fmt.Errorf("select reading: %w", err)
	}

	if serverUpdatedAt.After(clientUpdatedAt.UTC()) {
		return Reading{}, ErrConflict
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE blood_pressure_readings
		SET systolic = $1, diastolic = $2, pulse = $3, measured_at = $4, note = $5, updated_at = $6
		WHERE user_id = $7 AND id = $8 AND deleted_at IS NULL
	`, input.Systolic, input.Diastolic, input.Pulse, input.MeasuredAt.UTC(), input.Note, now, userID, id)
	if err != nil {
		return Reading{}, fmt.Errorf("update reading: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Reading{}, ErrNotFound
	}
	return r.Get(ctx, userID, id)
}

func (r *Repository) SoftDelete(ctx context.Context, userID, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE blood_pressure_readings
		SET deleted_at = $1, updated_at = $1
		WHERE user_id = $2 AND id = $3 AND deleted_at IS NULL
	`, now, userID, id)
	if err != nil {
		return fmt.Errorf("delete reading: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListChangesSince(ctx context.Context, userID string, since time.Time) ([]Reading, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, systolic, diastolic, pulse, measured_at, note, created_at, updated_at, deleted_at
		FROM blood_pressure_readings
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
	`, userID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	defer rows.Close()

	var readings []Reading
	for rows.Next() {
		reading, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		readings = append(readings, reading)
	}
	return readings, rows.Err()
}

func (r *Repository) UpsertSync(ctx context.Context, userID string, item SyncUpsert) (string, error) {
	now := time.Now().UTC()
	clientUpdated := item.UpdatedAt.UTC()

	if item.ServerID != nil && *item.ServerID != "" {
		var serverUpdatedAt time.Time
		var deletedAt sql.NullTime
		err := r.db.QueryRowContext(ctx, `
			SELECT updated_at, deleted_at FROM blood_pressure_readings
			WHERE user_id = $1 AND id = $2
		`, userID, *item.ServerID).Scan(&serverUpdatedAt, &deletedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return r.insertReading(ctx, userID, item, now)
			}
			return "", fmt.Errorf("select for upsert: %w", err)
		}
		if deletedAt.Valid {
			return *item.ServerID, nil
		}
		if serverUpdatedAt.After(clientUpdated) {
			return *item.ServerID, nil
		}
		_, err = r.db.ExecContext(ctx, `
			UPDATE blood_pressure_readings
			SET systolic = $1, diastolic = $2, pulse = $3, measured_at = $4, note = $5, updated_at = $6
			WHERE user_id = $7 AND id = $8
		`, item.Systolic, item.Diastolic, item.Pulse, item.MeasuredAt.UTC(), item.Note, now, userID, *item.ServerID)
		if err != nil {
			return "", fmt.Errorf("update sync reading: %w", err)
		}
		return *item.ServerID, nil
	}

	return r.insertReading(ctx, userID, item, now)
}

func (r *Repository) insertReading(ctx context.Context, userID string, item SyncUpsert, now time.Time) (string, error) {
	id := newUUID()
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO blood_pressure_readings(
			id, user_id, systolic, diastolic, pulse, measured_at, note, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (user_id, measured_at, systolic, diastolic) DO UPDATE
		SET pulse = EXCLUDED.pulse,
		    note = EXCLUDED.note,
		    updated_at = EXCLUDED.updated_at,
		    deleted_at = NULL
		RETURNING id
	`, id, userID, item.Systolic, item.Diastolic, item.Pulse, item.MeasuredAt.UTC(), item.Note, now).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert sync reading: %w", err)
	}
	return id, nil
}

func (r *Repository) SoftDeleteMany(ctx context.Context, userID string, ids []string) error {
	for _, id := range ids {
		if id == "" {
			continue
		}
		_ = r.SoftDelete(ctx, userID, id)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReading(scanner rowScanner) (Reading, error) {
	var reading Reading
	var pulse sql.NullInt64
	var note sql.NullString
	var deletedAt sql.NullTime

	err := scanner.Scan(
		&reading.ID,
		&reading.Systolic,
		&reading.Diastolic,
		&pulse,
		&reading.MeasuredAt,
		&note,
		&reading.CreatedAt,
		&reading.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return Reading{}, err
	}
	if pulse.Valid {
		value := int(pulse.Int64)
		reading.Pulse = &value
	}
	if note.Valid {
		value := note.String
		reading.Note = &value
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		reading.DeletedAt = &value
	}
	return reading, nil
}
