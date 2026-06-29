package readings

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound       = errors.New("reading not found")
	ErrConflict       = errors.New("conflict")
	ErrInvalidReading = errors.New("invalid reading")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	Systolic   int
	Diastolic  int
	Pulse      *int
	MeasuredAt time.Time
	Note       *string
}

type UpdateInput struct {
	Systolic      int
	Diastolic     int
	Pulse         *int
	MeasuredAt    time.Time
	Note          *string
	ClientUpdated time.Time
}

type SyncRequest struct {
	LastSyncAt       *time.Time   `json:"lastSyncAt"`
	Upserts          []SyncUpsert `json:"upserts"`
	DeletedServerIDs []string     `json:"deletedServerIds"`
}

type SyncResponse struct {
	Mappings      []SyncMapping `json:"mappings"`
	RemoteChanges []Reading     `json:"remoteChanges"`
	ServerTime    time.Time     `json:"serverTime"`
}

func (s *Service) List(ctx context.Context, userID string, since *time.Time, limit int) ([]Reading, error) {
	return s.repo.List(ctx, userID, since, limit)
}

func (s *Service) Get(ctx context.Context, userID, id string) (Reading, error) {
	return s.repo.Get(ctx, userID, id)
}

func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (Reading, error) {
	if err := validateReading(input.Systolic, input.Diastolic, input.Pulse, input.Note); err != nil {
		return Reading{}, err
	}
	return s.repo.Create(ctx, userID, Reading{
		Systolic:   input.Systolic,
		Diastolic:  input.Diastolic,
		Pulse:      input.Pulse,
		MeasuredAt: input.MeasuredAt.UTC(),
		Note:       input.Note,
	})
}

func (s *Service) Update(ctx context.Context, userID, id string, input UpdateInput) (Reading, error) {
	if err := validateReading(input.Systolic, input.Diastolic, input.Pulse, input.Note); err != nil {
		return Reading{}, err
	}
	return s.repo.Update(ctx, userID, id, Reading{
		Systolic:   input.Systolic,
		Diastolic:  input.Diastolic,
		Pulse:      input.Pulse,
		MeasuredAt: input.MeasuredAt.UTC(),
		Note:       input.Note,
	}, input.ClientUpdated)
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	return s.repo.SoftDelete(ctx, userID, id)
}

func (s *Service) Sync(ctx context.Context, userID string, req SyncRequest) (SyncResponse, error) {
	mappings := make([]SyncMapping, 0, len(req.Upserts))
	for _, item := range req.Upserts {
		if err := validateReading(item.Systolic, item.Diastolic, item.Pulse, item.Note); err != nil {
			continue
		}
		serverID, err := s.repo.UpsertSync(ctx, userID, item)
		if err != nil {
			return SyncResponse{}, err
		}
		if item.LocalID != nil {
			mappings = append(mappings, SyncMapping{
				LocalID:  *item.LocalID,
				ServerID: serverID,
				Status:   "SYNCED",
			})
		}
	}

	if len(req.DeletedServerIDs) > 0 {
		_ = s.repo.SoftDeleteMany(ctx, userID, req.DeletedServerIDs)
	}

	since := time.Time{}
	if req.LastSyncAt != nil {
		since = req.LastSyncAt.UTC()
	}
	remoteChanges, err := s.repo.ListChangesSince(ctx, userID, since)
	if err != nil {
		return SyncResponse{}, err
	}

	return SyncResponse{
		Mappings:      mappings,
		RemoteChanges: remoteChanges,
		ServerTime:    time.Now().UTC(),
	}, nil
}

func validateReading(systolic, diastolic int, pulse *int, note *string) error {
	if systolic < 70 || systolic > 250 {
		return fmt.Errorf("%w: invalid systolic", ErrInvalidReading)
	}
	if diastolic < 40 || diastolic > 150 {
		return fmt.Errorf("%w: invalid diastolic", ErrInvalidReading)
	}
	if diastolic >= systolic {
		return fmt.Errorf("%w: diastolic must be less than systolic", ErrInvalidReading)
	}
	if pulse != nil && (*pulse < 30 || *pulse > 200) {
		return fmt.Errorf("%w: invalid pulse", ErrInvalidReading)
	}
	if note != nil && len(*note) > 500 {
		return fmt.Errorf("%w: note too long", ErrInvalidReading)
	}
	return nil
}
