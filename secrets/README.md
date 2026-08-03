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

## Notes

- `*.json` under this directory is gitignored — never commit the key.
- Override the host path with `FIREBASE_CREDENTIALS_FILE=/absolute/path.json` if needed.
- Alternative to a file mount: set `FIREBASE_CREDENTIALS_JSON={...}` in `.env` (takes precedence).
