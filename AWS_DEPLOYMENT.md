# AWS Deployment Guide

This guide explains how to deploy your application to AWS Amplify using MVPBridge.

## Prerequisites

### 1. AWS Account
- Create an AWS account at https://aws.amazon.com
- Note your preferred region (e.g., `us-east-1`, `us-west-2`)

### 2. AWS Credentials
You need AWS access credentials with permissions for AWS Amplify.

#### Create IAM User:
1. Go to AWS IAM Console
2. Create new user with programmatic access
3. Attach policy: `AdministratorAccess-Amplify` or create custom policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "amplify:*"
      ],
      "Resource": "*"
    }
  ]
}
```

4. Save the **Access Key ID** and **Secret Access Key**

### 3. GitHub Token
AWS Amplify needs access to your GitHub repository.

1. Go to GitHub Settings > Developer settings > Personal access tokens
2. Generate new token (classic) with scopes:
   - `repo` (full control)
   - `admin:repo_hook` (write)
3. Save the token

## Environment Variables

Set these environment variables before deploying:

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1  # optional, defaults to us-east-1
export GITHUB_TOKEN=your_github_token
```

## Deployment Steps

### 1. Initialize MVPBridge with AWS target

```bash
mvpbridge init --target aws
```

This creates `.mvpbridge/config.yaml`:

```yaml
version: 1
framework: vite  # or nextjs
target: aws
detected:
  package_manager: npm
  build_command: npm run build
  output_dir: dist
  node_version: "20"
  output_type: static
```

### 2. Inspect your project

```bash
mvpbridge inspect
```

### 3. Normalize your project

```bash
mvpbridge normalize
```

This adds necessary files but note that AWS Amplify uses its own build system, so the Dockerfile is mainly for local development.

### 4. Deploy to AWS Amplify

```bash
mvpbridge deploy aws
```

Output:
```
Deploying to AWS Amplify...

[1/4] Validating credentials... ✓
[2/4] Creating app spec... ✓
[3/4] Configuring secrets (3 vars)... ✓
[4/4] Triggering deployment... ✓

Deployment started!
  App URL: https://main.d1a2b3c4d5e6f7.amplifyapp.com
  Console: https://us-east-1.console.aws.amazon.com/amplify/home?region=us-east-1#/d1a2b3c4d5e6f7
```

## Configuration Options

### Custom Region

Edit `.mvpbridge/config.yaml`:

```yaml
version: 1
framework: vite
target: aws
deploy:
  region: us-west-2  # Change region
  app_name: my-custom-app-name
```

### Environment Variables

Add secrets to your `.env` file:

```env
API_KEY=your_api_key
DATABASE_URL=your_database_url
```

MVPBridge will automatically configure these in Amplify during deployment.

## AWS Amplify Features

### Auto-Deployments
Once connected, AWS Amplify automatically deploys on every push to your main branch.

### Custom Domains
1. Go to your app in Amplify Console
2. Click "Domain management"
3. Add your custom domain
4. Follow DNS configuration instructions

### Build Settings
The build spec is automatically generated based on your framework:

**For Vite:**
```yaml
version: 1
frontend:
  phases:
    preBuild:
      commands:
        - npm ci
    build:
      commands:
        - npm run build
  artifacts:
    baseDirectory: dist
    files:
      - '**/*'
```

**For Next.js Static:**
```yaml
version: 1
frontend:
  phases:
    preBuild:
      commands:
        - npm ci
    build:
      commands:
        - npm run build
  artifacts:
    baseDirectory: out
    files:
      - '**/*'
```

### Manual Overrides
You can customize build settings in the Amplify Console:
1. Go to your app
2. Click "Build settings"
3. Edit the build spec YAML

## Troubleshooting

### Authentication Failed
```
Error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables must be set
```

**Solution:** Export your AWS credentials:
```bash
export AWS_ACCESS_KEY_ID=your_key
export AWS_SECRET_ACCESS_KEY=your_secret
```

### GitHub Token Missing
```
Error: GITHUB_TOKEN environment variable required for AWS Amplify
```

**Solution:** Export your GitHub token:
```bash
export GITHUB_TOKEN=your_github_token
```

### Build Failures
Check the Amplify Console for detailed build logs:
1. Open the app in Amplify Console
2. Click on the latest deployment
3. View build logs

Common issues:
- Missing dependencies in `package.json`
- Incorrect build command
- Missing environment variables

### Permission Errors
```
Error: User is not authorized to perform: amplify:CreateApp
```

**Solution:** Add Amplify permissions to your IAM user:
- Attach `AdministratorAccess-Amplify` policy
- Or create custom policy with required Amplify permissions

## Comparison: DigitalOcean vs AWS

| Feature | DigitalOcean | AWS Amplify |
|---------|--------------|-------------|
| Setup complexity | Simpler | More complex |
| Credentials | 1 token | Access key + secret |
| Pricing | $5/month | Pay per use |
| Auto-deploy | Yes | Yes |
| Custom domains | Yes | Yes |
| Global CDN | Yes | CloudFront |
| Build logs | Dashboard | Console |
| SSR Support | Yes (Docker) | Limited |

## Cost Estimation

AWS Amplify pricing (as of 2024):
- **Build minutes:** $0.01 per minute (1000 free/month)
- **Hosting:** $0.15 per GB served (15 GB free/month)
- **Storage:** $0.023 per GB/month (5 GB free/month)

Typical small app:
- ~10 builds/month (2 min each) = ~$0
- ~5 GB data transfer = ~$0
- ~1 GB storage = ~$0

**Free tier covers most MVPs!**

## Next Steps

1. **Monitor deployments:** Check Amplify Console for build status
2. **Set up custom domain:** Add your domain in Amplify settings
3. **Configure CI/CD:** Use GitHub Actions for advanced workflows
4. **Enable notifications:** Set up SNS alerts for deployment events

## Advanced: Using AWS SDK (Alternative)

For more control, you can use the AWS SDK directly. However, the simplified implementation above uses the Amplify REST API.

To upgrade to full AWS SDK:
1. Add dependency: `go get github.com/aws/aws-sdk-go-v2`
2. Replace HTTP client with AWS SDK client
3. Use SigV4 signing for authentication

## Support

For issues specific to:
- **MVPBridge:** https://github.com/daryllundy/mvp-bridge/issues
- **AWS Amplify:** https://docs.aws.amazon.com/amplify/
- **AWS Support:** https://console.aws.amazon.com/support/
