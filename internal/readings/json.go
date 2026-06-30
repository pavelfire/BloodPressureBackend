package readings

import (
	"encoding/json"
	"strings"
	"time"
)

type utcTime time.Time

func (t utcTime) MarshalJSON() ([]byte, error) {
	formatted := time.Time(t).UTC().Format(time.RFC3339Nano)
	formatted = strings.Replace(formatted, "+00:00", "Z", 1)
	return json.Marshal(formatted)
}

func (t *utcTime) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return err
		}
	}
	*t = utcTime(parsed)
	return nil
}

type readingJSON struct {
	ID          string    `json:"id"`
	Systolic    int       `json:"systolic"`
	Diastolic   int       `json:"diastolic"`
	Pulse       *int      `json:"pulse,omitempty"`
	MeasuredAt  utcTime   `json:"measuredAt"`
	Note        *string   `json:"note,omitempty"`
	CreatedAt   utcTime   `json:"createdAt"`
	UpdatedAt   utcTime   `json:"updatedAt"`
	DeletedAt   *utcTime  `json:"deletedAt,omitempty"`
}

func readingToJSON(r Reading) readingJSON {
	result := readingJSON{
		ID:         r.ID,
		Systolic:   r.Systolic,
		Diastolic:  r.Diastolic,
		Pulse:      r.Pulse,
		MeasuredAt: utcTime(r.MeasuredAt),
		Note:       r.Note,
		CreatedAt:  utcTime(r.CreatedAt),
		UpdatedAt:  utcTime(r.UpdatedAt),
	}
	if r.DeletedAt != nil {
		value := utcTime(*r.DeletedAt)
		result.DeletedAt = &value
	}
	return result
}

type syncResponseJSON struct {
	Mappings      []SyncMapping `json:"mappings"`
	RemoteChanges []readingJSON `json:"remoteChanges"`
	ServerTime    utcTime       `json:"serverTime"`
}

func syncResponseToJSON(response SyncResponse) syncResponseJSON {
	remote := make([]readingJSON, 0, len(response.RemoteChanges))
	for _, reading := range response.RemoteChanges {
		remote = append(remote, readingToJSON(reading))
	}
	return syncResponseJSON{
		Mappings:      response.Mappings,
		RemoteChanges: remote,
		ServerTime:    utcTime(response.ServerTime),
	}
}

func readingsToJSON(readings []Reading) []readingJSON {
	result := make([]readingJSON, 0, len(readings))
	for _, reading := range readings {
		result = append(result, readingToJSON(reading))
	}
	return result
}
