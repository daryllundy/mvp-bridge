# Azure Deployment Guide

This guide explains how to deploy your application to Azure Container Apps using MVPBridge.

## Prerequisites

### 1. Azure subscription
- Create or choose an Azure subscription
- Pick a region such as `eastus` or `westus3`

### 2. Azure CLI
Install the Azure CLI and ensure `az` is in your `PATH`.

MVPBridge uses non-interactive Azure CLI commands:
- `az containerapp up --source .`
- `az containerapp update --set-env-vars ...`

### 3. Azure login
Authenticate locally before deploying:

```bash
az login
```

Optional: set a specific subscription if your account has multiple:

```bash
az account set --subscription <subscription-id>
```

## Deployment Steps

### 1. Initialize MVPBridge with Azure target

```bash
mvpbridge init --target azure
```

Example config:

```yaml
version: 1
framework: vite
target: azure
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
  node_version: "20"
  output_type: static
deploy:
  app_name: my-app
  region: eastus
  subscription_id: 00000000-0000-0000-0000-000000000000
  resource_group: mvpbridge-my-app-rg
  environment: mvpbridge-my-app-env
```

### 2. Inspect your project

```bash
mvpbridge inspect
```

### 3. Normalize your project

```bash
mvpbridge normalize
```

For the Azure deploy path, normalization matters because deployment expects a local Dockerfile and deployable source tree.

### 4. Deploy to Azure Container Apps

```bash
mvpbridge deploy azure
```

Example output:

```text
Deploying to Azure Container Apps...

[1/4] Validating credentials... ✓
[2/4] Preparing Container App deployment... ✓
[3/4] Configuring secrets (3 vars)... ✓
[4/4] Triggering deployment... ✓

Deployment started!
  App URL: https://my-app.azurecontainerapps.io
  Console: https://portal.azure.com/#@/resource/subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.App/containerApps/my-app
```

## Configuration Options

Update `.mvpbridge/config.yaml` to pin Azure settings:

```yaml
version: 1
framework: vite
target: azure
deploy:
  app_name: my-app
  region: eastus
  subscription_id: 00000000-0000-0000-0000-000000000000
  resource_group: mvpbridge-my-app-rg
  environment: mvpbridge-my-app-env
```

Defaults:
- `deploy.region` defaults to `eastus`
- `deploy.app_name` falls back to repo name
- `deploy.resource_group` defaults to `mvpbridge-<app-name>-rg`
- `deploy.environment` defaults to `mvpbridge-<app-name>-env`
- `deploy.subscription_id` is optional; if omitted, Azure CLI active subscription is used

## How Azure Deploy Works

- MVPBridge checks for a local `Dockerfile`
- It deploys from the current source tree with `az containerapp up --source .`
- Static apps deploy with target port `80`
- SSR apps deploy with target port `3000`
- `.env` values are applied as Container App environment variables with `az containerapp update --set-env-vars`
- Resource group and Container Apps environment are auto-created or reused by Azure CLI

## Troubleshooting

### Not logged in

```text
Error: azure CLI is not authenticated - run 'az login'
```

Run:

```bash
az login
```

### Missing Azure CLI

```text
Error: az CLI not found in PATH
```

Install Azure CLI and verify:

```bash
az version
```

### Missing Dockerfile

```text
Error: Dockerfile not found - run 'mvpbridge normalize' first
```

Run:

```bash
mvpbridge normalize
```
