package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq"
)

var ErrNotFound = errors.New("not found")

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
	VideoID           string
	Amount            int64
	Currency          string
	Status            string
	FreedomPayPayment string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateOrderParams struct {
	UserID     string
	VideoID    string
	Amount     int64
	Currency   string
	CustomerID string
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

CREATE TABLE IF NOT EXISTS orders (
	id BIGSERIAL PRIMARY KEY,
	user_id TEXT NOT NULL,
	video_id TEXT NOT NULL REFERENCES videos(id),
	amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
	currency TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	customer_id TEXT NOT NULL DEFAULT '',
	freedompay_payment_id TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS orders_user_video_paid_idx
	ON orders(user_id, video_id)
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
	var order Order
	err := s.db.QueryRowContext(ctx, `
INSERT INTO orders (user_id, video_id, amount_cents, currency, customer_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, video_id, amount_cents, currency, status, freedompay_payment_id, created_at, updated_at
`, params.UserID, params.VideoID, params.Amount, params.Currency, params.CustomerID).
		Scan(&order.ID, &order.UserID, &order.VideoID, &order.Amount, &order.Currency, &order.Status, &order.FreedomPayPayment, &order.CreatedAt, &order.UpdatedAt)
	return order, err
}

func (s *Store) GetOrder(ctx context.Context, id int64) (Order, error) {
	var order Order
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, video_id, amount_cents, currency, status, freedompay_payment_id, created_at, updated_at
FROM orders
WHERE id = $1
`, id).Scan(&order.ID, &order.UserID, &order.VideoID, &order.Amount, &order.Currency, &order.Status, &order.FreedomPayPayment, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	return order, err
}

func (s *Store) MarkOrderPaid(ctx context.Context, id int64, paymentID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID, videoID string
	err = tx.QueryRowContext(ctx, `
UPDATE orders
SET status = 'paid', freedompay_payment_id = $2, updated_at = now()
WHERE id = $1
RETURNING user_id, video_id
`, id, paymentID).Scan(&userID, &videoID)
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

func (s *Store) UserHasAccess(ctx context.Context, userID, videoID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM orders
	WHERE user_id = $1 AND video_id = $2 AND status = 'paid'
)
`, userID, videoID).Scan(&exists)
	return exists, err
}
