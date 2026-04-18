package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func (s *Store) UpsertArticle(ctx context.Context, a *model.Article) (isNew bool, err error) {
	if a.FetchedAt == 0 {
		a.FetchedAt = time.Now().Unix()
	}

	// 先查是否存在，真正区分"插入"和"更新"，让调用方拿到准确的 isNew。
	// MaxOpenConns=1 保证串行，无需显式事务。
	var existingID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM articles WHERE review_id = ?`, a.ReviewID,
	).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	if errors.Is(err, sql.ErrNoRows) {
		res, err := s.db.ExecContext(ctx, `
            INSERT INTO articles
                (book_id, review_id, title, summary, content_html, cover_url, url,
                 publish_at, fetched_at, read_num, like_num)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        `, a.BookID, a.ReviewID, a.Title, a.Summary, a.ContentHTML, a.CoverURL, a.URL,
			a.PublishAt, a.FetchedAt, a.ReadNum, a.LikeNum,
		)
		if err != nil {
			return false, err
		}
		a.ID, _ = res.LastInsertId()
		return true, nil
	}

	// 已存在 — 列表字段按最新覆盖，url / content_html 只在原来为空时才覆盖
	a.ID = existingID
	_, err = s.db.ExecContext(ctx, `
        UPDATE articles SET
            title     = ?,
            summary   = ?,
            cover_url = ?,
            read_num  = ?,
            like_num  = ?,
            url = CASE WHEN IFNULL(url, '') = '' THEN ? ELSE url END,
            content_html = CASE WHEN IFNULL(content_html, '') = '' THEN ? ELSE content_html END
        WHERE review_id = ?
    `, a.Title, a.Summary, a.CoverURL, a.ReadNum, a.LikeNum, a.URL, a.ContentHTML, a.ReviewID)
	return false, err
}

func (s *Store) ListArticlesByUser(ctx context.Context, userID int64, limit, offset int) ([]*model.Article, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT a.id, a.book_id, a.review_id, a.title, a.summary, a.content_html,
               a.cover_url, a.url, a.publish_at, a.fetched_at, a.read_num, a.like_num
        FROM articles a
        JOIN subscriptions s ON s.book_id = a.book_id
        WHERE s.user_id = ? AND s.disabled = 0
        ORDER BY a.publish_at DESC
        LIMIT ? OFFSET ?
    `, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListArticlesByBook(ctx context.Context, bookID string, limit, offset int) ([]*model.Article, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, book_id, review_id, title, summary, content_html, cover_url, url,
               publish_at, fetched_at, read_num, like_num
        FROM articles
        WHERE book_id = ?
        ORDER BY publish_at DESC
        LIMIT ? OFFSET ?
    `, bookID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetArticleByReviewID(ctx context.Context, reviewID string) (*model.Article, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, book_id, review_id, title, summary, content_html, cover_url, url,
               publish_at, fetched_at, read_num, like_num
        FROM articles
        WHERE review_id = ?
    `, reviewID)
	a, err := scanArticle(row)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return a, nil
}

func (s *Store) UpdateArticleContent(ctx context.Context, reviewID, contentHTML string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE articles SET content_html = ? WHERE review_id = ?`,
		contentHTML, reviewID,
	)
	return err
}

func scanArticle(row rowScanner) (*model.Article, error) {
	a := &model.Article{}
	if err := row.Scan(
		&a.ID, &a.BookID, &a.ReviewID, &a.Title, &a.Summary, &a.ContentHTML, &a.CoverURL, &a.URL,
		&a.PublishAt, &a.FetchedAt, &a.ReadNum, &a.LikeNum,
	); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) UpdateArticleURL(ctx context.Context, reviewID, url string) error {
	if url == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE articles SET url = ? WHERE review_id = ?`,
		url, reviewID,
	)
	return err
}
