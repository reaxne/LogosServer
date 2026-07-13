package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrNotConfigured = errors.New("database is not configured")
)

type Store struct {
	db *sql.DB
}

type Video struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	PriceCents          int64  `json:"price_cents"`
	CloudflareStreamUID string `json:"cloudflare_stream_uid"`
}

type Order struct {
	ID                int64
	UserID            string
	PhoneNumber       string
	VideoID           string
	Amount            int64
	Currency          string
	Status            string
	PaymentURL        string
	FreedomPayPayment string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateOrderParams struct {
	PhoneNumber string
	VideoID     string
	Amount      int64
	Currency    string
	CustomerID  string
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrNotConfigured
	}
	return s.db.PingContext(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS videos (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	price_cents BIGINT NOT NULL CHECK (price_cents > 0),
	cloudflare_stream_uid TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
	phone_number TEXT PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
	id BIGSERIAL PRIMARY KEY,
	user_id TEXT NOT NULL,
	video_id TEXT NOT NULL REFERENCES videos(id),
	amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
	currency TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	customer_id TEXT NOT NULL DEFAULT '',
	freedompay_payment_id TEXT NOT NULL DEFAULT '',
	payment_url TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS phone_number TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_url TEXT NOT NULL DEFAULT '';

UPDATE orders
SET phone_number = user_id
WHERE phone_number IS NULL OR phone_number = '';

INSERT INTO users (phone_number)
SELECT DISTINCT phone_number
FROM orders
WHERE phone_number IS NOT NULL AND phone_number <> ''
ON CONFLICT (phone_number) DO NOTHING;

ALTER TABLE orders ALTER COLUMN phone_number SET NOT NULL;

CREATE INDEX IF NOT EXISTS orders_user_video_paid_idx
	ON orders(user_id, video_id)
	WHERE status = 'paid';

CREATE INDEX IF NOT EXISTS orders_phone_video_paid_idx
	ON orders(phone_number, video_id)
	WHERE status = 'paid';

CREATE UNIQUE INDEX IF NOT EXISTS orders_phone_video_paid_unique_idx
	ON orders(phone_number, video_id)
	WHERE status = 'paid';
`)
	return err
}

func (s *Store) UpsertVideo(ctx context.Context, video Video) (Video, error) {
	err := s.db.QueryRowContext(ctx, `
INSERT INTO videos (id, title, price_cents, cloudflare_stream_uid)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET
	title = EXCLUDED.title,
	price_cents = EXCLUDED.price_cents,
	cloudflare_stream_uid = EXCLUDED.cloudflare_stream_uid,
	updated_at = now()
RETURNING id, title, price_cents, cloudflare_stream_uid
`, video.ID, video.Title, video.PriceCents, video.CloudflareStreamUID).
		Scan(&video.ID, &video.Title, &video.PriceCents, &video.CloudflareStreamUID)
	return video, err
}

func (s *Store) GetVideo(ctx context.Context, id string) (Video, error) {
	var video Video
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, price_cents, cloudflare_stream_uid
FROM videos
WHERE id = $1
`, id).Scan(&video.ID, &video.Title, &video.PriceCents, &video.CloudflareStreamUID)
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrNotFound
	}
	return video, err
}

func (s *Store) CreateOrder(ctx context.Context, params CreateOrderParams) (Order, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO users (phone_number)
VALUES ($1)
ON CONFLICT (phone_number) DO UPDATE SET updated_at = now()
`, params.PhoneNumber)
	if err != nil {
		return Order{}, err
	}

	var order Order
	err = tx.QueryRowContext(ctx, `
INSERT INTO orders (user_id, phone_number, video_id, amount_cents, currency, customer_id)
VALUES ($1, $1, $2, $3, $4, $5)
RETURNING id, user_id, phone_number, video_id, amount_cents, currency, status, payment_url, freedompay_payment_id, created_at, updated_at
`, params.PhoneNumber, params.VideoID, params.Amount, params.Currency, params.CustomerID).
		Scan(&order.ID, &order.UserID, &order.PhoneNumber, &order.VideoID, &order.Amount, &order.Currency, &order.Status, &order.PaymentURL, &order.FreedomPayPayment, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return Order{}, err
	}
	return order, tx.Commit()
}

func (s *Store) GetOrder(ctx context.Context, id int64) (Order, error) {
	var order Order
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, phone_number, video_id, amount_cents, currency, status, payment_url, freedompay_payment_id, created_at, updated_at
FROM orders
WHERE id = $1
`, id).Scan(&order.ID, &order.UserID, &order.PhoneNumber, &order.VideoID, &order.Amount, &order.Currency, &order.Status, &order.PaymentURL, &order.FreedomPayPayment, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	return order, err
}

func (s *Store) GetActiveOrderForPhoneVideo(ctx context.Context, phoneNumber, videoID string) (Order, error) {
	var order Order
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, phone_number, video_id, amount_cents, currency, status, payment_url, freedompay_payment_id, created_at, updated_at
FROM orders
WHERE phone_number = $1 AND video_id = $2 AND status IN ('pending', 'paid')
ORDER BY
	CASE status WHEN 'paid' THEN 0 ELSE 1 END,
	created_at DESC
LIMIT 1
`, phoneNumber, videoID).Scan(&order.ID, &order.UserID, &order.PhoneNumber, &order.VideoID, &order.Amount, &order.Currency, &order.Status, &order.PaymentURL, &order.FreedomPayPayment, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	return order, err
}

func (s *Store) SaveOrderPaymentURL(ctx context.Context, id int64, paymentURL string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE orders
SET payment_url = $2, updated_at = now()
WHERE id = $1 AND status = 'pending'
`, id, paymentURL)
	return err
}

func (s *Store) MarkOrderPaid(ctx context.Context, id int64, paymentID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var phoneNumber, videoID string
	err = tx.QueryRowContext(ctx, `
SELECT phone_number, video_id
FROM orders
WHERE id = $1
`, id).Scan(&phoneNumber, &videoID)
	if err != nil {
		return err
	}

	var alreadyPaid bool
	err = tx.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM orders
	WHERE phone_number = $1 AND video_id = $2 AND status = 'paid' AND id <> $3
)
`, phoneNumber, videoID, id).Scan(&alreadyPaid)
	if err != nil {
		return err
	}
	if alreadyPaid {
		_, err = tx.ExecContext(ctx, `
UPDATE orders
SET status = 'duplicate', freedompay_payment_id = $2, updated_at = now()
WHERE id = $1 AND status <> 'paid'
`, id, paymentID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	_, err = tx.ExecContext(ctx, `
UPDATE orders
SET status = 'paid', freedompay_payment_id = $2, updated_at = now()
WHERE id = $1
`, id, paymentID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOrderFailed(ctx context.Context, id int64, paymentID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE orders
SET status = 'failed', freedompay_payment_id = $2, updated_at = now()
WHERE id = $1 AND status <> 'paid'
`, id, paymentID)
	return err
}

func (s *Store) PhoneHasAccess(ctx context.Context, phoneNumber, videoID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM orders
	WHERE phone_number = $1 AND video_id = $2 AND status = 'paid'
)
`, phoneNumber, videoID).Scan(&exists)
	return exists, err
}
