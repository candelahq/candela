package cloudauth

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/oauth2"
)

// AWSProvider implements Provider for Amazon Web Services.
// It supports multiple credential resolution strategies:
//   - AWS SSO (delegates to `aws sso login`)
//   - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
//   - Shared credentials file (~/.aws/credentials)
//   - IAM instance roles (EC2/ECS/Lambda)
type AWSProvider struct {
	// region is the default AWS region for API calls.
	region string

	// profile is the AWS CLI profile to use. Empty = "default".
	profile string
}

// NewAWSProvider creates a new AWS credential provider.
func NewAWSProvider(region, profile string) *AWSProvider {
	if region == "" {
		region = "us-east-1"
	}
	return &AWSProvider{
		region:  region,
		profile: profile,
	}
}

// Name returns "aws".
func (a *AWSProvider) Name() string { return "aws" }

// IsConfigured returns true if AWS credentials can be resolved from any source.
func (a *AWSProvider) IsConfigured() bool {
	cfg, err := a.loadConfig(context.Background())
	if err != nil {
		return false
	}
	creds, err := cfg.Credentials.Retrieve(context.Background())
	return err == nil && creds.HasKeys()
}

// Login performs the AWS authentication flow.
// For SSO profiles, it delegates to `aws sso login`.
// For access key profiles, it validates existing credentials.
func (a *AWSProvider) Login(ctx context.Context) error {
	// First, check if credentials already work.
	cfg, err := a.loadConfig(ctx)
	if err == nil {
		if identity, verifyErr := a.verifyIdentity(ctx, cfg); verifyErr == nil {
			fmt.Printf("\n🔑 Already authenticated as: %s\n", aws.ToString(identity.Arn))
			return nil
		}
	}

	// Try SSO login if the aws CLI is available.
	if a.hasSSOProfile() {
		fmt.Println("Starting AWS SSO login...")
		return a.ssoLogin(ctx)
	}

	// Check if credentials exist but are expired.
	if err != nil {
		fmt.Println("No AWS credentials found.")
		fmt.Println()
		fmt.Println("Configure credentials using one of:")
		fmt.Println("  • aws configure              — set up access keys")
		fmt.Println("  • aws configure sso          — set up SSO profile")
		fmt.Println("  • export AWS_ACCESS_KEY_ID   — set environment variables")
		fmt.Println()
		fmt.Println("Then run: candela auth login --provider aws")
		return fmt.Errorf("no AWS credentials configured")
	}

	return nil
}

// Status returns the current AWS credential state.
func (a *AWSProvider) Status(ctx context.Context) (*CredentialStatus, error) {
	cfg, err := a.loadConfig(ctx)
	if err != nil {
		return &CredentialStatus{
			Provider: "aws",
			Valid:    false,
			FilePath: a.credentialsPath(),
		}, nil
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil || !creds.HasKeys() {
		return &CredentialStatus{
			Provider: "aws",
			Valid:    false,
			FilePath: a.credentialsPath(),
			Account:  "no credentials: " + err.Error(),
		}, nil
	}

	// Try STS GetCallerIdentity for detailed info.
	identity, stsErr := a.verifyIdentity(ctx, cfg)
	if stsErr != nil {
		return &CredentialStatus{
			Provider: "aws",
			Valid:    false,
			FilePath: a.credentialsPath(),
			Account:  "credentials invalid: " + stsErr.Error(),
		}, nil
	}

	status := &CredentialStatus{
		Provider: "aws",
		Account:  aws.ToString(identity.Arn),
		Valid:    true,
		FilePath: a.credentialsPath(),
	}

	// Session credentials have an expiry.
	if creds.CanExpire && !creds.Expires.IsZero() {
		status.ExpiresIn = time.Until(creds.Expires)
	}

	return status, nil
}

// TokenSource returns nil for AWS — AWS uses request-level SigV4 signing,
// not Bearer tokens. Use AWSConfig() to get the aws.Config for SigV4 signing.
func (a *AWSProvider) TokenSource(_ context.Context, _ ...string) (oauth2.TokenSource, error) {
	return nil, fmt.Errorf("AWS uses request signing (SigV4), not Bearer tokens — use AWSConfig() instead")
}

// AWSConfig returns a configured aws.Config for use with AWS SDK clients
// and the SigV4 request signer.
func (a *AWSProvider) AWSConfig(ctx context.Context) (aws.Config, error) {
	return a.loadConfig(ctx)
}

// Region returns the configured AWS region.
func (a *AWSProvider) Region() string { return a.region }

// loadConfig creates an aws.Config using the standard credential chain.
func (a *AWSProvider) loadConfig(ctx context.Context) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(a.region),
	}
	if a.profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(a.profile))
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

// verifyIdentity calls STS GetCallerIdentity to validate credentials
// and return the caller's ARN/account.
func (a *AWSProvider) verifyIdentity(ctx context.Context, cfg aws.Config) (*sts.GetCallerIdentityOutput, error) {
	stsClient := sts.NewFromConfig(cfg)
	verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return stsClient.GetCallerIdentity(verifyCtx, &sts.GetCallerIdentityInput{})
}

// hasSSOProfile checks if the current profile is configured for SSO
// by parsing the AWS config INI file and looking for sso_start_url
// in the section matching the target profile.
func (a *AWSProvider) hasSSOProfile() bool {
	dir := a.awsDir()
	if dir == "" {
		return false
	}
	configPath := filepath.Join(dir, "config")
	f, err := os.Open(configPath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	// Determine which INI section header to look for.
	// AWS config uses [default] for the default profile and
	// [profile <name>] for named profiles.
	profile := a.profile
	var targetSection string
	if profile == "" || profile == "default" {
		targetSection = "[default]"
	} else {
		targetSection = "[profile " + profile + "]"
	}

	// Parse INI: find the target section, then look for sso_start_url
	// before the next section begins.
	inTargetSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			if inTargetSection {
				return false // hit next section without finding sso_start_url
			}
			inTargetSection = (line == targetSection)
			continue
		}
		if inTargetSection && strings.HasPrefix(line, "sso_start_url") {
			return true
		}
	}
	return false
}

// ssoLogin delegates to the aws CLI for SSO authentication.
func (a *AWSProvider) ssoLogin(ctx context.Context) error {
	args := []string{"sso", "login"}
	if a.profile != "" {
		args = append(args, "--profile", a.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws sso login failed: %w", err)
	}

	// Verify credentials after SSO login.
	cfg, err := a.loadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load credentials after SSO login: %w", err)
	}

	identity, err := a.verifyIdentity(ctx, cfg)
	if err != nil {
		return fmt.Errorf("SSO login succeeded but credentials are invalid: %w", err)
	}

	fmt.Printf("\n🔑 Authenticated as: %s\n", aws.ToString(identity.Arn))
	return nil
}

// awsDir returns the AWS config directory (~/.aws).
// Returns "" if the home directory cannot be determined — callers must
// check for the empty string before joining paths.
func (a *AWSProvider) awsDir() string {
	if dir := os.Getenv("AWS_CONFIG_FILE"); dir != "" {
		return filepath.Dir(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Debug("cannot determine home directory for AWS config", "error", err)
		return ""
	}
	return filepath.Join(home, ".aws")
}

// credentialsPath returns the AWS credentials file path.
// Returns a relative path if the home directory cannot be determined,
// which is acceptable for display purposes in error messages.
func (a *AWSProvider) credentialsPath() string {
	if path := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); path != "" {
		return path
	}
	dir := a.awsDir()
	if dir == "" {
		return "~/.aws/credentials" // display fallback
	}
	return filepath.Join(dir, "credentials")
}

func init() {
	// Register with default region — actual region comes from config at runtime.
	// This ensures `cloudauth.Get("aws")` works for status/listing even before
	// config is loaded. The real AWSProvider with correct region/profile is
	// created by buildCloudProxy when an anthropic-bedrock provider is configured.
	slog.Debug("registering default AWS provider in cloudauth registry")
	Register(NewAWSProvider("", ""))
}
