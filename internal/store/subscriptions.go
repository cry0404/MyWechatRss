package store

import (
	"context"
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
             last_fetch_at, last_review_time, created_at, disabled)
        VALUES (?, ?, ?, ?, ?, ?, -1, -1, 0, 0, ?, 0)
    `, sub.UserID, sub.BookID, sub.Alias, sub.MPName, sub.CoverURL,
		sub.FetchIntervalSec, sub.CreatedAt)
	if err != nil {
		return err
	}
	sub.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListSubscriptionsByUser(ctx context.Context, userID int64) ([]*model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min,
               last_fetch_at, last_review_time, created_at, disabled
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
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min,
               last_fetch_at, last_review_time, created_at, disabled
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
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min,
               last_fetch_at, last_review_time, created_at, disabled
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

func (s *Store) UpdateSubscriptionFetchState(ctx context.Context, id int64, lastFetchAt, lastReviewTime int64) error {
	_, err := s.db.ExecContext(ctx, `
        UPDATE subscriptions SET last_fetch_at = ?, last_review_time = ? WHERE id = ?
    `, lastFetchAt, lastReviewTime, id)
	return err
}

func (s *Store) DeleteSubscription(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

func (s *Store) ListSubscriptionsDueForFetch(ctx context.Context, now int64) ([]*model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, book_id, alias, mp_name, cover_url,
               fetch_interval_sec, fetch_window_start_min, fetch_window_end_min,
               last_fetch_at, last_review_time, created_at, disabled
        FROM subscriptions
        WHERE disabled = 0
          AND last_fetch_at + fetch_interval_sec <= ?
    `, now)
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
		&sub.FetchIntervalSec, &sub.FetchWindowStartMin, &sub.FetchWindowEndMin,
		&sub.LastFetchAt, &sub.LastReviewTime, &sub.CreatedAt, &disabled,
	); err != nil {
		return nil, err
	}
	sub.Disabled = disabled != 0
	return sub, nil
}
