package importexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"lexis/internal/platform/httpx"
)

// BackupVersion — версия формата. Меняем при несовместимых изменениях; старые версии
// при необходимости конвертируем при восстановлении.
const BackupVersion = 1

// Backup — полный снимок данных (§4: слова, группы, уровни, история).
// id внутри файла — только для ссылок между сущностями: при восстановлении
// записи получают новые id, ссылки переназначаются.
type Backup struct {
	Version      int                        `json:"version"`
	ExportedAt   time.Time                  `json:"exportedAt"`
	Settings     map[string]json.RawMessage `json:"settings"`
	Groups       []BackupGroup              `json:"groups"`
	Words        []BackupWord               `json:"words"`
	Sessions     []BackupSession            `json:"sessions"`
	Achievements []BackupAchievement        `json:"achievements"`
}

type BackupGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupWord struct {
	ID             int64      `json:"id"`
	Term           string     `json:"term"`
	Forms          []string   `json:"forms"`
	Translation    string     `json:"translation"`
	Level          int16      `json:"level"`
	Streak         int16      `json:"streak"`
	NextReviewAt   time.Time  `json:"nextReviewAt"`
	LastReviewedAt *time.Time `json:"lastReviewedAt"`
	LastGoodAt     *time.Time `json:"lastGoodAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	Examples       []string   `json:"examples"` // разметка "I [go] home", в порядке position
	GroupIDs       []int64    `json:"groupIds"`
}

type BackupSession struct {
	ID          int64          `json:"id"`
	GroupID     *int64         `json:"groupId"`
	Strategy    string         `json:"strategy"`
	Free        bool           `json:"free"`
	StartedAt   time.Time      `json:"startedAt"`
	FinishedAt  *time.Time     `json:"finishedAt"`
	Total       int32          `json:"total"`
	Good        int32          `json:"good"`
	Hard        int32          `json:"hard"`
	Again       int32          `json:"again"`
	DurationSec int32          `json:"durationSec"`
	Reviews     []BackupReview `json:"reviews"`
}

type BackupReview struct {
	WordID         int64     `json:"wordId"`
	Answer         string    `json:"answer"`
	FirstInSession bool      `json:"firstInSession"`
	LevelBefore    int16     `json:"levelBefore"`
	LevelAfter     int16     `json:"levelAfter"`
	AnsweredAt     time.Time `json:"answeredAt"`
}

type BackupAchievement struct {
	Code       string    `json:"code"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

// DecodeBackup разбирает файл бэкапа строго: неизвестное поле — скорее всего, не наш файл
// или другая версия, и восстанавливать «как получится» поверх всех данных нельзя.
func DecodeBackup(data []byte) (Backup, error) {
	var b Backup
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Backup{}, httpx.BadRequest("invalid_backup", "invalid backup: "+err.Error())
	}
	return b, validateBackup(b)
}

// validateBackup проверяет версию и ссылки до транзакции: висячая ссылка в файле —
// ошибка 400, а не 500 от внешнего ключа посреди восстановления.
// Значения полей (уровни 1..5, непустые строки, стратегии) проверяют CHECK-и схемы.
func validateBackup(b Backup) error {
	if b.Version != BackupVersion {
		return httpx.BadRequest("unsupported_backup_version", fmt.Sprintf("backup version %d is not supported", b.Version))
	}
	bad := func(format string, args ...any) error {
		return httpx.BadRequest("invalid_backup", fmt.Sprintf(format, args...))
	}

	groups := make(map[int64]bool, len(b.Groups))
	for _, g := range b.Groups {
		if groups[g.ID] {
			return bad("duplicate group id %d", g.ID)
		}
		groups[g.ID] = true
	}
	wordIDs := make(map[int64]bool, len(b.Words))
	for _, w := range b.Words {
		if wordIDs[w.ID] {
			return bad("duplicate word id %d", w.ID)
		}
		wordIDs[w.ID] = true
		for _, gid := range w.GroupIDs {
			if !groups[gid] {
				return bad("word %d refers to unknown group %d", w.ID, gid)
			}
		}
	}
	sessions := make(map[int64]bool, len(b.Sessions))
	for _, s := range b.Sessions {
		if sessions[s.ID] {
			return bad("duplicate session id %d", s.ID)
		}
		sessions[s.ID] = true
		if s.GroupID != nil && !groups[*s.GroupID] {
			return bad("session %d refers to unknown group %d", s.ID, *s.GroupID)
		}
		for _, r := range s.Reviews {
			if !wordIDs[r.WordID] {
				return bad("review in session %d refers to unknown word %d", s.ID, r.WordID)
			}
		}
	}
	return nil
}
