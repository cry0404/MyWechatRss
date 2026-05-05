package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func (s *Store) CreateAccount(ctx context.Context, a *model.WeReadAccount) error {
	skeyEnc, err := s.codec.Encrypt(a.SKey)
	if err != nil {
		return err
	}
	rtEnc, err := s.codec.Encrypt(a.RefreshToken)
	if err != nil {
		return err
	}
	cookiesJSON, err := json.Marshal(a.Cookies)
	if err != nil {
		return err
	}
	cookiesEnc, err := s.codec.Encrypt(string(cookiesJSON))
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	if a.Status == "" {
		a.Status = model.AccountActive
	}

	res, err := s.db.ExecContext(ctx, `
        INSERT INTO weread_accounts
            (user_id, vid, skey_enc, refresh_token_enc, cookies_enc,
             nickname, avatar, status, cooldown_until, last_ok_at, last_err,
             created_at, device_id, install_id, device_name)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?, ?, ?)
        ON CONFLICT (user_id, vid) DO UPDATE SET
            skey_enc          = excluded.skey_enc,
            refresh_token_enc = excluded.refresh_token_enc,
            cookies_enc       = excluded.cookies_enc,
            nickname          = excluded.nickname,
            avatar            = excluded.avatar,
            status            = 'active',
            cooldown_until    = 0,
            last_err          = ''
    `,
		a.UserID, a.VID, skeyEnc, rtEnc, cookiesEnc,
		a.Nickname, a.Avatar, string(a.Status),
		a.CreatedAt, a.DeviceID, a.InstallID, a.DeviceName,
	)
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil && id > 0 {
		a.ID = id
	} else {
		_ = s.db.QueryRowContext(ctx,
			`SELECT id FROM weread_accounts WHERE user_id = ? AND vid = ?`,
			a.UserID, a.VID,
		).Scan(&a.ID)
	}
	return nil
}

func (s *Store) ListAccountsByUser(ctx context.Context, userID int64) ([]*model.WeReadAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, user_id, vid, skey_enc, refresh_token_enc, cookies_enc,
               nickname, avatar, status, cooldown_until, last_ok_at, last_err,
               created_at, device_id, install_id, device_name
        FROM weread_accounts
        WHERE user_id = ?
        ORDER BY id ASC
    `, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.WeReadAccount
	for rows.Next() {
		a, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) PickActiveAccount(ctx context.Context, userID int64) (*model.WeReadAccount, error) {
	now := time.Now().Unix()
	// 把已过期的 cooldown 自动恢复为 active，避免状态与 cooldown_until 不一致。
	_, _ = s.db.ExecContext(ctx, `
        UPDATE weread_accounts
        SET status = 'active', cooldown_until = 0, last_err = ''
        WHERE user_id = ? AND status = 'cooldown' AND cooldown_until <= ?
    `, userID, now)
	row := s.db.QueryRowContext(ctx, `
        SELECT id, user_id, vid, skey_enc, refresh_token_enc, cookies_enc,
               nickname, avatar, status, cooldown_until, last_ok_at, last_err,
               created_at, device_id, install_id, device_name
        FROM weread_accounts
        WHERE user_id = ?
          AND status = 'active'
          AND cooldown_until <= ?
        ORDER BY last_ok_at ASC
        LIMIT 1
    `, userID, now)
	a, err := s.scanAccountRow(row)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return a, nil
}

func (s *Store) GetActiveAccountByID(ctx context.Context, userID, id int64) (*model.WeReadAccount, error) {
	now := time.Now().Unix()
	// 把已过期的 cooldown 自动恢复为 active，避免状态与 cooldown_until 不一致。
	_, _ = s.db.ExecContext(ctx, `
        UPDATE weread_accounts
        SET status = 'active', cooldown_until = 0, last_err = ''
        WHERE user_id = ? AND id = ? AND status = 'cooldown' AND cooldown_until <= ?
    `, userID, id, now)
	row := s.db.QueryRowContext(ctx, `
        SELECT id, user_id, vid, skey_enc, refresh_token_enc, cookies_enc,
               nickname, avatar, status, cooldown_until, last_ok_at, last_err,
               created_at, device_id, install_id, device_name
        FROM weread_accounts
        WHERE user_id = ? AND id = ?
          AND status = 'active'
          AND cooldown_until <= ?
    `, userID, id, now)
	a, err := s.scanAccountRow(row)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return a, nil
}

func (s *Store) MarkAccountOK(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE weread_accounts SET last_ok_at = ?, last_err = '' WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (s *Store) MarkAccountCooldown(ctx context.Context, id int64, errMsg string, duration time.Duration) error {
	until := time.Now().Add(duration).Unix()
	_, err := s.db.ExecContext(ctx, `
        UPDATE weread_accounts
        SET status = 'cooldown', cooldown_until = ?, last_err = ?
        WHERE id = ?
    `, until, errMsg, id)
	return err
}

func (s *Store) MarkAccountDead(ctx context.Context, userID, id int64, errMsg string) error {
	res, err := s.db.ExecContext(ctx, `
        UPDATE weread_accounts SET status = 'dead', last_err = ?
        WHERE id = ? AND status != 'dead'
    `, errMsg, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 && s.deadHook != nil {
		hookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer cancel()
			s.deadHook(hookCtx, userID, errMsg)
		}()
	}
	return nil
}

func (s *Store) SetDeadHook(fn func(ctx context.Context, userID int64, lastErr string)) {
	s.deadHook = fn
}

func (s *Store) CountActiveAccounts(ctx context.Context, userID int64) (int, error) {
	now := time.Now().Unix()
	// 把已过期的 cooldown 自动恢复为 active，避免 scheduler 因 CountActiveAccounts=0
	// 而跳过该用户所有订阅，导致 PickActiveAccount 永远不被调用、恢复逻辑永不被执行的死锁。
	_, _ = s.db.ExecContext(ctx, `
		UPDATE weread_accounts
		SET status = 'active', cooldown_until = 0, last_err = ''
		WHERE user_id = ? AND status = 'cooldown' AND cooldown_until <= ?
	`, userID, now)
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM weread_accounts WHERE user_id = ? AND status = 'active'`,
		userID,
	).Scan(&n)
	return n, err
}

// UpdateAccountCredential 续期成功后回写凭证。nil 指针表示"weread 这次没给新值，
// 保留旧值不动"—— 这是续期的关键语义：benign refresh 时 weread 常常只回 skey，
// refreshToken / cookies 字段是空的，直接写空会把还能用的旧值抹掉。
// skey 走过 server 层校验保证非空，因此这里是必传的。
//
// 无论哪几个字段实际被更新，status / cooldown_until / last_err 都会被重置成
// "active 且健康"—— 续期成功本身就是"这个账号当前是活的"的最强信号。
func (s *Store) UpdateAccountCredential(
	ctx context.Context,
	id int64,
	skey string,
	refreshToken *string,
	cookies *map[string]string,
) error {
	skeyEnc, err := s.codec.Encrypt(skey)
	if err != nil {
		return err
	}

	sets := []string{"skey_enc = ?", "status = 'active'", "cooldown_until = 0", "last_ok_at = ?", "last_err = ''"}
	args := []any{skeyEnc, time.Now().Unix()}

	if refreshToken != nil {
		rtEnc, err := s.codec.Encrypt(*refreshToken)
		if err != nil {
			return err
		}
		sets = append(sets, "refresh_token_enc = ?")
		args = append(args, rtEnc)
	}
	if cookies != nil {
		cookiesJSON, err := json.Marshal(*cookies)
		if err != nil {
			return err
		}
		cookiesEnc, err := s.codec.Encrypt(string(cookiesJSON))
		if err != nil {
			return err
		}
		sets = append(sets, "cookies_enc = ?")
		args = append(args, cookiesEnc)
	}

	args = append(args, id)
	query := "UPDATE weread_accounts SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) UpdateAccountCookies(ctx context.Context, id int64, cookies map[string]string) error {
	cookiesJSON, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	cookiesEnc, err := s.codec.Encrypt(string(cookiesJSON))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE weread_accounts SET cookies_enc = ? WHERE id = ?`,
		cookiesEnc, id,
	)
	return err
}

func (s *Store) DeleteAccount(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM weread_accounts WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

func (s *Store) ListAllActiveAccounts(ctx context.Context) ([]*model.WeReadAccount, error) {
	now := time.Now().Unix()
	_, _ = s.db.ExecContext(ctx, `
		UPDATE weread_accounts
		SET status = 'active', cooldown_until = 0, last_err = ''
		WHERE status = 'cooldown' AND cooldown_until <= ?
	`, now)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, vid, skey_enc, refresh_token_enc, cookies_enc,
		       nickname, avatar, status, cooldown_until, last_ok_at, last_err,
		       created_at, device_id, install_id, device_name
		FROM weread_accounts
		WHERE status = 'active'
		ORDER BY last_ok_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.WeReadAccount
	for rows.Next() {
		a, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanAccountRow(row rowScanner) (*model.WeReadAccount, error) {
	return s.scanAccount(row)
}

func (s *Store) scanAccount(row rowScanner) (*model.WeReadAccount, error) {
	a := &model.WeReadAccount{}
	var skeyEnc, rtEnc, cookiesEnc, status string
	if err := row.Scan(
		&a.ID, &a.UserID, &a.VID, &skeyEnc, &rtEnc, &cookiesEnc,
		&a.Nickname, &a.Avatar, &status, &a.CooldownUntil, &a.LastOkAt, &a.LastErr,
		&a.CreatedAt, &a.DeviceID, &a.InstallID, &a.DeviceName,
	); err != nil {
		return nil, err
	}

	var err error
	if a.SKey, err = s.codec.Decrypt(skeyEnc); err != nil {
		return nil, err
	}
	if a.RefreshToken, err = s.codec.Decrypt(rtEnc); err != nil {
		return nil, err
	}
	if cookiesEnc != "" {
		plain, err := s.codec.Decrypt(cookiesEnc)
		if err != nil {
			return nil, err
		}
		if plain != "" {
			_ = json.Unmarshal([]byte(plain), &a.Cookies)
		}
	}
	a.Status = model.AccountStatus(status)
	return a, nil
}
