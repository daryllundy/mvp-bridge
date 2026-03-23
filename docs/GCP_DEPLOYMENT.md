# GCP Deployment Guide

This guide explains how to deploy your application to Google Cloud Run using MVPBridge.

## Prerequisites

### 1. Google Cloud project
- Create or choose a Google Cloud project
- Record the project ID
- Pick a Cloud Run region such as `us-central1`

### 2. Required APIs
Enable these APIs in the target project:
- Cloud Run Admin API
- Cloud Build API
- Artifact Registry API

### 3. Service account credentials
Create a service account that can deploy to Cloud Run and build container images.

Recommended roles:
- `roles/run.admin`
- `roles/cloudbuild.builds.editor`
- `roles/artifactregistry.admin`
- `roles/iam.serviceAccountUser`

Download the service account JSON key and keep its path available locally.

### 4. gcloud CLI
Install the Google Cloud CLI and ensure `gcloud` is in your `PATH`.

MVPBridge uses non-interactive `gcloud run deploy --source .` for the GCP deploy path.

## Environment Variables

Set these environment variables before deploying:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/service-account.json
export GCP_PROJECT_ID=your-gcp-project
```

Optional:

```bash
export CLOUDSDK_CORE_DISABLE_PROMPTS=1
```

## Deployment Steps

### 1. Initialize MVPBridge with GCP target

```bash
mvpbridge init --target gcp
```

Example config:

```yaml
version: 1
framework: vite
target: gcp
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
  node_version: "20"
  output_type: static
deploy:
  project_id: your-gcp-project
  region: us-central1
  app_name: my-app
```

### 2. Inspect your project

```bash
mvpbridge inspect
```

### 3. Normalize your project

```bash
mvpbridge normalize
```

For the GCP deploy path, normalization matters because Cloud Run deploys from the local source tree and expects a deployable Dockerfile.

### 4. Deploy to Cloud Run

```bash
mvpbridge deploy gcp
```

Example output:

```text
Deploying to Google Cloud Run...

[1/4] Validating credentials... ✓
[2/4] Preparing Cloud Run deployment... ✓
[3/4] Configuring secrets (3 vars)... ✓
[4/4] Triggering deployment... ✓

Deployment started!
  App URL: https://my-app-abc123-uc.a.run.app
  Console: https://console.cloud.google.com/run/detail/us-central1/my-app/metrics?project=your-gcp-project
```

## Configuration Options

Update `.mvpbridge/config.yaml` to pin GCP settings:

```yaml
version: 1
framework: vite
target: gcp
deploy:
  app_name: my-app
  project_id: your-gcp-project
  region: us-central1
```

Defaults:
- `deploy.project_id` falls back to `GCP_PROJECT_ID`
- `deploy.region` defaults to `us-central1`
- `deploy.app_name` falls back to the repo name

## How GCP Deploy Works

- MVPBridge checks for a local `Dockerfile`
- It deploys from the current source tree with `gcloud run deploy --source .`
- Static apps deploy with Cloud Run port `80`
- SSR apps deploy with Cloud Run port `3000`
- `.env` values are passed to Cloud Run as service environment variables

## Troubleshooting

### Missing credentials

```text
Error: GOOGLE_APPLICATION_CREDENTIALS environment variable must be set
```

Set the service account key path:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/service-account.json
```

### Missing project ID

```text
Error: GCP project ID must be set in config deploy.project_id or GCP_PROJECT_ID
```

Set the project in config or export:

```bash
export GCP_PROJECT_ID=your-gcp-project
```

### Missing Dockerfile

```text
Error: Dockerfile not found - run 'mvpbridge normalize' first
```

Run:

```bash
mvpbridge normalize
```

### gcloud not installed

```text
Error: gcloud CLI not found in PATH
```

Install the Google Cloud CLI and verify:

```bash
gcloud version
```
