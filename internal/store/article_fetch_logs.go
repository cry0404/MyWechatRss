package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func (s *Store) RecordArticleFetchLog(ctx context.Context, log *model.ArticleFetchLog) error {
	if log.CreatedAt == 0 {
		log.CreatedAt = time.Now().Unix()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO article_fetch_logs
			(review_id, book_id, chain, success, cost_ms, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, log.ReviewID, log.BookID, log.Chain, boolInt(log.Success), log.CostMs, log.Error, log.CreatedAt)
	if err != nil {
		return err
	}
	log.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListArticleFetchLogs(ctx context.Context, reviewID string, limit, offset int) ([]*model.ArticleFetchLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows *sql.Rows
	var err error
	if reviewID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, review_id, book_id, chain, success, cost_ms, error, created_at
			FROM article_fetch_logs
			WHERE review_id = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`, reviewID, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, review_id, book_id, chain, success, cost_ms, error, created_at
			FROM article_fetch_logs
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ArticleFetchLog
	for rows.Next() {
		l, err := scanFetchLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetFetchStats(ctx context.Context, since, until int64) ([]*model.FetchStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			chain,
			COUNT(*) AS total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) AS success_cnt,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) AS fail_cnt,
			AVG(cost_ms) AS avg_cost_ms
		FROM article_fetch_logs
		WHERE created_at >= ? AND created_at <= ?
		GROUP BY chain
		ORDER BY total DESC
	`, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.FetchStats
	for rows.Next() {
		var st model.FetchStats
		var avgCost float64
		if err := rows.Scan(&st.Chain, &st.Total, &st.Success, &st.Fail, &avgCost); err != nil {
			return nil, err
		}
		if st.Total > 0 {
			st.SuccessPct = float64(st.Success) / float64(st.Total) * 100
		}
		st.AvgCostMs = int64(avgCost)
		out = append(out, &st)
	}
	return out, rows.Err()
}

func (s *Store) GetRecentFailureRate(ctx context.Context, windowSec int64) (float64, error) {
	since := time.Now().Unix() - windowSec
	var total, failed int64
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END), 0)
		FROM article_fetch_logs
		WHERE created_at >= ?
	`, since).Scan(&total, &failed)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return float64(failed) / float64(total) * 100, nil
}

func scanFetchLog(row rowScanner) (*model.ArticleFetchLog, error) {
	l := &model.ArticleFetchLog{}
	var successInt int
	if err := row.Scan(
		&l.ID, &l.ReviewID, &l.BookID, &l.Chain, &successInt, &l.CostMs, &l.Error, &l.CreatedAt,
	); err != nil {
		return nil, err
	}
	l.Success = successInt == 1
	return l, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
