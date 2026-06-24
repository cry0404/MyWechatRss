package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

const defaultFetchIntervalSec = 6 * 60 * 60

func (s *Store) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	now := time.Now().Unix()
	if sub.CreatedAt == 0 {
		sub.CreatedAt = now
	}
	if sub.FetchIntervalSec <= 0 {
		sub.FetchIntervalSec = defaultFetchIntervalSec
	}
	res, err := s.db.ExecContext(ctx, `
        INSERT INTO subscriptions
            (user_id, book_id, alias, mp_name, cover_url,
             fetch_interval_sec, fetch_window_start_min, fetch_window_end_min,
             last_fetch_at, last_review_time, article_synckey, created_at, disabled)
        VALUES (?, ?, ?, ?, ?, ?, -1, -1, 0, 0, ?, ?, 0)
    `, sub.UserID, sub.BookID, sub.Alias, sub.MPName, sub.CoverURL,
		sub.FetchIntervalSec, sub.ArticleSynckey, sub.CreatedAt)
	if err != nil {
		return err
	}
	sub.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListSubscriptionsByUser(ctx context.Context, userID int64) ([]*model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min, next_fetch_after,
               last_fetch_at, last_review_time, article_synckey, created_at, disabled
        FROM subscriptions
        WHERE user_id = ?
        ORDER BY created_at DESC
    `, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) GetSubscription(ctx context.Context, userID, id int64) (*model.Subscription, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min, next_fetch_after,
               last_fetch_at, last_review_time, article_synckey, created_at, disabled
        FROM subscriptions
        WHERE user_id = ? AND id = ?
    `, userID, id)
	sub, err := scanSubscription(row)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return sub, nil
}

func (s *Store) GetSubscriptionByBookID(ctx context.Context, userID int64, bookID string) (*model.Subscription, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min, next_fetch_after,
               last_fetch_at, last_review_time, article_synckey, created_at, disabled
        FROM subscriptions
        WHERE user_id = ? AND book_id = ?
    `, userID, bookID)
	sub, err := scanSubscription(row)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return sub, nil
}

func (s *Store) UpdateSubscriptionMeta(ctx context.Context, userID, id int64, alias *string, intervalSec *int64, disabled *bool, winStart *int64, winEnd *int64) error {
	sub, err := s.GetSubscription(ctx, userID, id)
	if err != nil {
		return err
	}
	if alias != nil {
		sub.Alias = *alias
	}
	if intervalSec != nil && *intervalSec > 0 {
		sub.FetchIntervalSec = *intervalSec
	}
	if disabled != nil {
		sub.Disabled = *disabled
	}
	if winStart != nil {
		sub.FetchWindowStartMin = *winStart
	}
	if winEnd != nil {
		sub.FetchWindowEndMin = *winEnd
	}
	_, err = s.db.ExecContext(ctx, `
        UPDATE subscriptions
        SET alias = ?, fetch_interval_sec = ?, fetch_window_start_min = ?, fetch_window_end_min = ?, disabled = ?
        WHERE id = ?
    `, sub.Alias, sub.FetchIntervalSec, sub.FetchWindowStartMin, sub.FetchWindowEndMin, boolToInt(sub.Disabled), sub.ID)
	return err
}

func (s *Store) UpdateSubscriptionFetchState(ctx context.Context, id int64, lastFetchAt, lastReviewTime, articleSynckey int64) error {
	_, err := s.db.ExecContext(ctx, `
        UPDATE subscriptions
        SET last_fetch_at = ?,
            last_review_time = ?,
            article_synckey = CASE WHEN ? > 0 THEN ? ELSE article_synckey END,
            next_fetch_after = 0
        WHERE id = ?
    `, lastFetchAt, lastReviewTime, articleSynckey, articleSynckey, id)
	return err
}

func (s *Store) DeferSubscriptionsFetch(ctx context.Context, ids []int64, firstUntil int64, step time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	stepSec := int64(step.Seconds())
	if stepSec < 0 {
		stepSec = 0
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		for i, id := range ids {
			until := firstUntil + int64(i)*stepSec
			if _, err := tx.ExecContext(ctx, `
				UPDATE subscriptions SET next_fetch_after = ? WHERE id = ?
			`, until, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeferDueSubscriptionsByUser(ctx context.Context, userID, now, firstUntil int64, step time.Duration) (int, error) {
	return s.DeferDueSubscriptionsByUserRotating(ctx, userID, now, firstUntil, step, 0)
}

func (s *Store) DeferAllEnabledSubscriptionsByUserRotating(ctx context.Context, userID, firstUntil int64, step time.Duration, rotateLastID int64) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM subscriptions
		WHERE user_id = ?
		  AND disabled = 0
		ORDER BY
		  CASE
		    WHEN next_fetch_after > 0 THEN next_fetch_after
		    ELSE last_fetch_at + fetch_interval_sec
		  END ASC,
		  last_fetch_at ASC,
		  id ASC
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if rotateLastID > 0 && len(ids) > 1 {
		ids = rotateIDToEnd(ids, rotateLastID)
	}
	if err := s.deferSubscriptionsFetchAtLeast(ctx, ids, firstUntil, step); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (s *Store) DeferDueSubscriptionsByUserRotating(ctx context.Context, userID, now, firstUntil int64, step time.Duration, rotateLastID int64) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM subscriptions
		WHERE user_id = ?
		  AND disabled = 0
		  AND last_fetch_at + fetch_interval_sec <= ?
		  AND (next_fetch_after = 0 OR next_fetch_after <= ?)
		ORDER BY
		  CASE WHEN next_fetch_after > 0 THEN next_fetch_after ELSE last_fetch_at + fetch_interval_sec END ASC,
		  last_fetch_at ASC,
		  id ASC
	`, userID, now, firstUntil)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if rotateLastID > 0 && len(ids) > 1 {
		ids = rotateIDToEnd(ids, rotateLastID)
	}
	if err := s.deferSubscriptionsFetchAtLeast(ctx, ids, firstUntil, step); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (s *Store) deferSubscriptionsFetchAtLeast(ctx context.Context, ids []int64, firstUntil int64, step time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	stepSec := int64(step.Seconds())
	if stepSec < 0 {
		stepSec = 0
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		for i, id := range ids {
			until := firstUntil + int64(i)*stepSec
			if _, err := tx.ExecContext(ctx, `
				UPDATE subscriptions
				SET next_fetch_after = CASE
					WHEN next_fetch_after > ? THEN next_fetch_after
					ELSE ?
				END
				WHERE id = ?
			`, until, until, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func rotateIDToEnd(ids []int64, id int64) []int64 {
	for i, got := range ids {
		if got != id {
			continue
		}
		if i == len(ids)-1 {
			return ids
		}
		out := make([]int64, 0, len(ids))
		out = append(out, ids[:i]...)
		out = append(out, ids[i+1:]...)
		out = append(out, id)
		return out
	}
	return ids
}

func (s *Store) DeleteSubscription(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

func (s *Store) ListSubscriptionsDueForFetch(ctx context.Context, now int64) ([]*model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min, next_fetch_after,
               last_fetch_at, last_review_time, article_synckey, created_at, disabled
        FROM subscriptions
        WHERE disabled = 0
          AND last_fetch_at + fetch_interval_sec <= ?
          AND (next_fetch_after = 0 OR next_fetch_after <= ?)
        ORDER BY
          CASE WHEN next_fetch_after > 0 THEN next_fetch_after ELSE last_fetch_at + fetch_interval_sec END ASC,
          last_fetch_at ASC,
          id ASC
    `, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func scanSubscription(row rowScanner) (*model.Subscription, error) {
	sub := &model.Subscription{}
	var disabled int
	if err := row.Scan(
		&sub.ID, &sub.UserID, &sub.BookID, &sub.Alias, &sub.MPName, &sub.CoverURL,
		&sub.FetchIntervalSec, &sub.FetchWindowStartMin, &sub.FetchWindowEndMin, &sub.NextFetchAfter,
		&sub.LastFetchAt, &sub.LastReviewTime, &sub.ArticleSynckey, &sub.CreatedAt, &disabled,
	); err != nil {
		return nil, err
	}
	sub.Disabled = disabled != 0
	return sub, nil
}
