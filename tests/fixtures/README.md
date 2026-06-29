# Test video fixtures

The videos listed in `videos.json` are stored outside Git in a private S3-compatible bucket. The current provider is intended to be Backblaze B2.

## Backblaze setup

1. Create a private bucket in the Backblaze web console.
2. Create an application key restricted to that bucket and the `test-fixtures/` prefix.
3. Grant `listAllBucketNames`, `listFiles`, `readFiles`, and `writeFiles`. Deletion access is not required.
4. Note the S3 endpoint shown by Backblaze, for example `https://s3.us-east-005.backblazeb2.com`.

Set credentials only in the current shell. Do not commit them:

```powershell
$env:FIXTURE_S3_ENDPOINT = "https://s3.<region>.backblazeb2.com"
$env:FIXTURE_S3_BUCKET = "<private-bucket-name>"
$env:FIXTURE_S3_ACCESS_KEY_ID = "<application-key-id>"
$env:FIXTURE_S3_SECRET_ACCESS_KEY = "<application-key>"
```

Alternatively, add the same values to the ignored project `.env` using dotenv syntax (`FIXTURE_S3_ENDPOINT=...`, without the PowerShell `$env:` prefix). The synchronization script reads only these four fixture variables from that file.

Upload the generated recordings from `../output/playwright/videos`:

```powershell
./scripts/sync-test-videos.ps1 -Mode Upload
```

Download and verify them on another machine:

```powershell
./scripts/sync-test-videos.ps1 -Mode Download
```

Downloaded files are written to `tests/fixtures/videos/` and ignored by Git. Docker is required; the script uses a pinned MinIO Client image to communicate through the S3 API.
