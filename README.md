# Logos Video Payment Server

Go API for creating Freedom Pay payments and unlocking Bunny Stream videos after a verified payment callback.

## What It Provides

* `POST /api/videos` saves video metadata and price. Requires `Authorization: Bearer ADMIN_TOKEN`.
* `GET /api/videos`
* `POST /api/orders` creates a Freedom Pay payment for a video.
* `GET|POST /api/payments/freedompay/callback` verifies Freedom Pay callback signatures and marks orders paid.
* `GET /api/videos/{video_id}/access?phone_number=...` tells your website if the phone number can watch and returns a Bunny Stream playback URL.
* `GET /health` is used by Railway health checks.
* `GET /ready` checks whether the database and Freedom Pay settings are ready.

## Railway Setup

1. Create a Railway project and add a Postgres database.
2. Deploy this folder as a service.
3. Set the variables from `.env.example`.
4. Set `PUBLIC_URL` to the final Railway service URL. If omitted, the server will infer it from Railway request headers.
5. Set `PAYMENT_SUCCESS_URL` and `PAYMENT_FAILURE_URL` to pages on your existing website.
6. In Freedom Pay, set the result/callback URL to:
   `https://your-railway-service.up.railway.app/api/payments/freedompay/callback`
7. In Bunny Stream, create a Video Library and note its **Library ID** and **Pull Zone hostname** (e.g. `vz-xxxxxxxx-xxx.b-cdn.net`). If you want private/signed playback URLs, enable **Token Authentication** on the pull zone and copy the **Authentication Key**.

## API Examples

Create or update a video:

```
curl -X POST "$PUBLIC_URL/api/videos" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "lesson-1",
    "title": "Lesson 1",
    "price_cents": 500000,
    "bunny_video_id": "YOUR_BUNNY_VIDEO_ID"
  }'
```

Create a payment order:

```
curl -X POST "$PUBLIC_URL/api/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+77001234567",
    "video_id": "lesson-1",
    "email": "buyer@example.com"
  }'
```

If the same phone number already paid for this video, the server returns:

```
{
  "status": "paid",
  "already_unlocked": true,
  "message": "video is already unlocked for this phone number"
}
```

If payment is already pending, the server returns the existing `payment_url` instead of creating a second payment.

Check video access:

```
curl "$PUBLIC_URL/api/videos/lesson-1/access?phone_number=%2B77001234567"
```

## Environment Variables (`.env.example`)

```
# Bunny Stream
BUNNY_PULL_ZONE=vz-xxxxxxxx-xxx.b-cdn.net
BUNNY_LIBRARY_ID=123456
BUNNY_AUTH_KEY=
```

* `BUNNY_PULL_ZONE` — the pull zone hostname for your Bunny Stream library, used to build thumbnail URLs.
* `BUNNY_LIBRARY_ID` — your Bunny Stream Video Library ID, used to build the iframe embed URL.
* `BUNNY_AUTH_KEY` — optional. The pull zone's Token Authentication key. If set, playback URLs are signed with a token and expiry. If omitted, the server returns a plain (unsigned) playback URL.

## Notes

* Prices are stored as cents. For KZT, `500000` means `5000.00`.
* Users are identified by normalized mobile number. For Kazakhstan-style numbers, `87001234567`, `7001234567`, and `+77001234567` are treated as the same user.
* The server will not create a new payment if that phone number already has paid access to the same video.
* Railway uses `/health` as a liveness check, so deployment can succeed even while payment variables are being filled in. Use `/ready` to confirm the server can actually process payments and unlock videos.
* Freedom Pay redirects buyers to `PAYMENT_SUCCESS_URL` or `PAYMENT_FAILURE_URL` with `order_id` appended.
* If `BUNNY_AUTH_KEY` is not configured, the API returns a normal iframe embed URL. Use signed URLs (by setting `BUNNY_AUTH_KEY` and enabling Token Authentication in Bunny) for private paid videos.
* Freedom Pay installations can differ slightly. If your merchant dashboard shows different script names or API URL, update `FREEDOMPAY_INIT_URL`, `FREEDOMPAY_INIT_SCRIPT`, and `FREEDOMPAY_CALLBACK_SCRIPT`.