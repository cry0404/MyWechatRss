package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func (s *Store) RecordSubscriptionFetchLog(ctx context.Context, log *model.SubscriptionFetchLog) error {
	if log.StartedAt == 0 {
		log.StartedAt = time.Now().Unix()
	}
	if log.ErrorCode == "" {
		log.ErrorCode = detectErrorCode(log.Error)
	}
	if log.ErrorCode == "-2041" && log.PreviousRateLimitAt == 0 {
		previous, err := s.previousRateLimitAt(ctx, log.AccountID, log.StartedAt)
		if err != nil {
			return err
		}
		if previous > 0 {
			log.PreviousRateLimitAt = previous
			log.SecondsSinceLastRateLimit = log.StartedAt - previous
		}
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO fetch_logs
			(subscription_id, account_id, started_at, cost_ms, new_count, error, error_code,
			 previous_rate_limit_at, seconds_since_last_rate_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.SubscriptionID, log.AccountID, log.StartedAt, log.CostMs, log.NewCount, log.Error, log.ErrorCode,
		log.PreviousRateLimitAt, log.SecondsSinceLastRateLimit)
	if err != nil {
		return err
	}
	log.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) previousRateLimitAt(ctx context.Context, accountID, before int64) (int64, error) {
	var previous int64
	var row *sql.Row
	if accountID > 0 {
		row = s.db.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(started_at), 0)
			FROM fetch_logs
			WHERE error_code = '-2041' AND account_id = ? AND started_at < ?
		`, accountID, before)
	} else {
		row = s.db.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(started_at), 0)
			FROM fetch_logs
			WHERE error_code = '-2041' AND started_at < ?
		`, before)
	}
	if err := row.Scan(&previous); err != nil {
		return 0, err
	}
	return previous, nil
}

func (s *Store) ListFetchEvents(ctx context.Context, userID int64, limit, offset int, rateLimitOnly bool) ([]*model.FetchEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	sourceFilter := ""
	contentFilter := ""
	if rateLimitOnly {
		sourceFilter = "AND fl.error_code = '-2041'"
		contentFilter = "AND afl.error LIKE '%-2041%'"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT *
		FROM (
			SELECT
				fl.id AS id,
				'source' AS event_type,
				'source' AS chain,
				fl.subscription_id AS subscription_id,
				s.alias AS subscription_alias,
				s.mp_name AS mp_name,
				'' AS review_id,
				s.book_id AS book_id,
				fl.account_id AS account_id,
				CASE WHEN fl.error = '' THEN 1 ELSE 0 END AS success,
				fl.cost_ms AS cost_ms,
				fl.new_count AS new_count,
				fl.error AS error,
				fl.error_code AS error_code,
				fl.started_at AS created_at,
				fl.previous_rate_limit_at AS previous_rate_limit_at,
				fl.seconds_since_last_rate_limit AS seconds_since_last_rate_limit
			FROM fetch_logs fl
			JOIN subscriptions s ON s.id = fl.subscription_id
			WHERE s.user_id = ? `+sourceFilter+`

			UNION ALL

			SELECT
				afl.id AS id,
				'content' AS event_type,
				afl.chain AS chain,
				COALESCE(s.id, 0) AS subscription_id,
				COALESCE(s.alias, '') AS subscription_alias,
				COALESCE(s.mp_name, '') AS mp_name,
				afl.review_id AS review_id,
				afl.book_id AS book_id,
				0 AS account_id,
				afl.success AS success,
				afl.cost_ms AS cost_ms,
				0 AS new_count,
				afl.error AS error,
				CASE WHEN afl.error LIKE '%-2041%' THEN '-2041' ELSE '' END AS error_code,
				afl.created_at AS created_at,
				0 AS previous_rate_limit_at,
				0 AS seconds_since_last_rate_limit
			FROM article_fetch_logs afl
			JOIN subscriptions s ON s.book_id = afl.book_id AND s.user_id = ?
			WHERE 1 = 1 `+contentFilter+`
		)
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, userID, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.FetchEvent
	for rows.Next() {
		ev := &model.FetchEvent{}
		var successInt int
		if err := rows.Scan(
			&ev.ID,
			&ev.EventType,
			&ev.Chain,
			&ev.SubscriptionID,
			&ev.SubscriptionAlias,
			&ev.MPName,
			&ev.ReviewID,
			&ev.BookID,
			&ev.AccountID,
			&successInt,
			&ev.CostMs,
			&ev.NewCount,
			&ev.Error,
			&ev.ErrorCode,
			&ev.CreatedAt,
			&ev.PreviousRateLimitAt,
			&ev.SecondsSinceLastRateLimit,
		); err != nil {
			return nil, err
		}
		ev.Success = successInt == 1
		out = append(out, ev)
	}
	return out, rows.Err()
}

func detectErrorCode(errText string) string {
	switch {
	case strings.Contains(errText, "-2041"):
		return "-2041"
	case strings.Contains(errText, "-2010"):
		return "-2010"
	case strings.Contains(errText, "-2012"):
		return "-2012"
	default:
		return ""
	}
}
