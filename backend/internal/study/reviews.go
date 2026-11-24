package study

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
	"lexis/internal/study/store"
)

type ReviewInput struct {
	ID      string `json:"id"`
	CardID  int64  `json:"cardId"`
	Known   bool   `json:"known"`
	Reverse bool   `json:"reverse"`
}
type Review struct {
	ReviewInput
	ReviewedAt time.Time `json:"reviewedAt"`
}

var reviewID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (r ReviewInput) Validate() error {
	if !reviewID.MatchString(r.ID) || r.CardID < 1 || r.CardID > 9007199254740991 {
		return httpx.BadRequest("invalid_study_review", "invalid review")
	}
	return nil
}
func ValidateHistory(reviews []Review) error {
	ids := map[string]bool{}
	for _, r := range reviews {
		if err := r.ReviewInput.Validate(); err != nil {
			return err
		}
		if ids[r.ID] || r.ReviewedAt.IsZero() || r.ReviewedAt.Year() < 1 || r.ReviewedAt.Year() > 9999 {
			return httpx.BadRequest("invalid_study_review", "invalid review history")
		}
		ids[r.ID] = true
	}
	return nil
}
func (s *Service) Record(ctx context.Context, in ReviewInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockStudy(ctx); err != nil {
			return err
		}
		grammar, err := q.IsGrammarCard(ctx, in.CardID)
		if err != nil {
			return err
		}
		if grammar {
			return httpx.BadRequest("invalid_study_review", "grammar rules are read without a review")
		}
		count, err := q.InsertReview(ctx, store.InsertReviewParams{ID: in.ID, CardID: in.CardID, ReviewedAt: s.clock.Now(), Known: in.Known, Reverse: in.Reverse})
		if err != nil {
			return err
		}
		if count == 0 {
			r, err := q.GetReview(ctx, in.ID)
			if err != nil {
				return err
			}
			if r.CardID != in.CardID || r.Known != in.Known || r.Reverse != in.Reverse {
				return httpx.Conflict("invalid_study_review", "review id was already used")
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("study review: %w", err)
	}
	return nil
}
func ReadHistory(ctx context.Context, db store.DBTX) ([]Review, error) {
	rows, err := store.New(db).ListReviews(ctx)
	if err != nil {
		return nil, fmt.Errorf("study history: %w", err)
	}
	out := []Review{}
	for _, r := range rows {
		out = append(out, Review{ReviewInput: ReviewInput{ID: r.ID, CardID: r.CardID, Known: r.Known, Reverse: r.Reverse}, ReviewedAt: r.ReviewedAt})
	}
	return out, nil
}
func WriteHistory(ctx context.Context, db store.DBTX, reviews []Review) error {
	q := store.New(db)
	if err := q.ClearReviews(ctx); err != nil {
		return err
	}
	for _, r := range reviews {
		if _, err := q.InsertReview(ctx, store.InsertReviewParams{ID: r.ID, CardID: r.CardID, ReviewedAt: r.ReviewedAt, Known: r.Known, Reverse: r.Reverse}); err != nil {
			return fmt.Errorf("restore study history: %w", err)
		}
	}
	return nil
}

type ReviewDay struct {
	Day   string `json:"day"`
	Total int    `json:"total"`
	Known int    `json:"known"`
}
type Stats struct {
	Days       []ReviewDay `json:"days"`
	Total      int         `json:"total"`
	Known      int         `json:"known"`
	ActiveDays int         `json:"activeDays"`
	BestStreak int         `json:"bestStreak"`
}

// FillStats uses civil dates in the configured timezone, including days without reviews.
func FillStats(reviews []Review, from, to time.Time, loc *time.Location) Stats {
	out := Stats{Days: []ReviewDay{}}
	counts := map[string]ReviewDay{}
	for _, r := range reviews {
		day := clock.DateOf(r.ReviewedAt.In(loc))
		if day.Before(from) || day.After(to) {
			continue
		}
		key := clock.FormatDate(day)
		v := counts[key]
		v.Total++
		if r.Known {
			v.Known++
		}
		counts[key] = v
	}
	streak := 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := clock.FormatDate(d)
		v := counts[key]
		v.Day = key
		out.Days = append(out.Days, v)
		out.Total += v.Total
		out.Known += v.Known
		if v.Total > 0 {
			out.ActiveDays++
			streak++
			out.BestStreak = max(out.BestStreak, streak)
		} else {
			streak = 0
		}
	}
	return out
}
func (s *Service) Stats(ctx context.Context, days int) (Stats, error) {
	if days != 7 && days != 30 && days != 90 && days != 365 {
		return Stats{}, httpx.BadRequest("invalid_range", "days must be 7, 30, 90 or 365")
	}
	now := s.clock.Now().In(s.clock.Location())
	to := clock.DateOf(now)
	from := to.AddDate(0, 0, 1-days)
	end := clock.StartOfDay(now).AddDate(0, 0, 1)
	start := clock.StartOfDay(now).AddDate(0, 0, 1-days)
	rows, err := store.New(s.pool).ListReviewsInRange(ctx, store.ListReviewsInRangeParams{FromAt: start, ToAt: end})
	if err != nil {
		return Stats{}, fmt.Errorf("study stats: %w", err)
	}
	reviews := make([]Review, 0, len(rows))
	for _, r := range rows {
		reviews = append(reviews, Review{ReviewInput: ReviewInput{Known: r.Known}, ReviewedAt: r.ReviewedAt})
	}
	return FillStats(reviews, from, to, s.clock.Location()), nil
}
