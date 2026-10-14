# Firebase credentials (FCM)

Docker Compose mounts this file into the API container as `/run/secrets/firebase.json`.

## Setup (local or server)

1. Firebase Console → Project settings → Service accounts → **Generate new private key**
2. Save the JSON as:

```text
BeCourtScore/secrets/firebase-adminsdk.json
```

3. Recreate the API container:

```bash
docker compose up -d --build --force-recreate api
docker compose logs api | head -n 40
```

You should see `notify: FCM sender ready` (not `using NoopSender`).

## Play billing (Android subscriptions)

Docker Compose also mounts the Play Developer API service account as `/run/secrets/play.json` and sets `PLAY_SERVICE_ACCOUNT_JSON` to that path. Without it, `POST /v1/billing/google/verify` returns 400 `store verifier unavailable` and Play RTDN retries fail.

1. Use the Play Console service account that has access to Court Score (`AndCourtScore/fastlane/play-service-account.json` on a machine that already uploads to Play).
2. Copy it to the server as:

```text
BeCourtScore/secrets/play-service-account.json
```

Or point Compose at another path with `PLAY_CREDENTIALS_FILE`.

3. Recreate the API container (same command as above).

You should see `billing: Play verifier ready`. `billing: Play verifier unavailable` means the file is missing, unreadable, or not valid JSON.

## Notes

- `*.json` under this directory is gitignored — never commit the key.
- Override the host path with `FIREBASE_CREDENTIALS_FILE=/absolute/path.json` if needed.
- Alternative to a file mount: set `FIREBASE_CREDENTIALS_JSON={...}` in `.env` (takes precedence).
