# Logos Video Payment Server

Go API for creating Freedom Pay payments and unlocking Cloudflare Stream videos after a verified payment callback.

## What It Provides

- `POST /api/videos` saves video metadata and price. Requires `Authorization: Bearer ADMIN_TOKEN`.
- `POST /api/orders` creates a Freedom Pay payment for a video.
- `GET|POST /api/payments/freedompay/callback` verifies Freedom Pay callback signatures and marks orders paid.
- `GET /api/videos/{video_id}/access?user_id=...` tells your website if the user can watch and returns a Cloudflare Stream playback URL.
- `GET /health` is used by Railway health checks.
- `GET /ready` checks whether the database and Freedom Pay settings are ready.

## Railway Setup

1. Create a Railway project and add a Postgres database.
2. Deploy this folder as a service.
3. Set the variables from `.env.example`.
4. Set `PUBLIC_URL` to the final Railway service URL. If omitted, the server will infer it from Railway request headers.
5. Set `PAYMENT_SUCCESS_URL` and `PAYMENT_FAILURE_URL` to pages on your existing website.
6. In Freedom Pay, set the result/callback URL to:

   `https://your-railway-service.up.railway.app/api/payments/freedompay/callback`

## API Examples

Create or update a video:

```bash
curl -X POST "$PUBLIC_URL/api/videos" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "lesson-1",
    "title": "Lesson 1",
    "price_cents": 500000,
    "cloudflare_stream_uid": "YOUR_STREAM_UID"
  }'
```

Create a payment order:

```bash
curl -X POST "$PUBLIC_URL/api/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "video_id": "lesson-1",
    "email": "buyer@example.com"
  }'
```

Check video access:

```bash
curl "$PUBLIC_URL/api/videos/lesson-1/access?user_id=user-123"
```

## Notes

- Prices are stored as cents. For KZT, `500000` means `5000.00`.
- Railway uses `/health` as a liveness check, so deployment can succeed even while payment variables are being filled in. Use `/ready` to confirm the server can actually process payments and unlock videos.
- Freedom Pay redirects buyers to `PAYMENT_SUCCESS_URL` or `PAYMENT_FAILURE_URL` with `order_id` appended.
- If Cloudflare Stream signing keys are not configured, the API returns a normal iframe URL. Use signed URLs for private paid videos.
- Freedom Pay installations can differ slightly. If your merchant dashboard shows different script names or API URL, update `FREEDOMPAY_INIT_URL`, `FREEDOMPAY_INIT_SCRIPT`, and `FREEDOMPAY_CALLBACK_SCRIPT`.
