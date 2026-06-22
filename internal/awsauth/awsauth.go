// Package awsauth mints short-lived RDS/Aurora IAM authentication tokens and
// injects them as the per-connection password via pgx's BeforeConnect hook. The
// app then authenticates to the database with AWS credentials from the default
// chain (IRSA / instance role / env), never a static database password.
package awsauth

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/jackc/pgx/v5"
)

// tokenBuilder mints an RDS auth token; seam for testing.
type tokenBuilder func(ctx context.Context, endpoint, region, dbUser string, creds aws.CredentialsProvider) (string, error)

// awsConfigLoader loads AWS config; seam for testing.
type awsConfigLoader func(ctx context.Context, region string) (aws.Config, error)

var defaultBuildToken tokenBuilder = func(ctx context.Context, endpoint, region, dbUser string, creds aws.CredentialsProvider) (string, error) {
	return auth.BuildAuthToken(ctx, endpoint, region, dbUser, creds)
}

var defaultLoadConfig awsConfigLoader = func(ctx context.Context, region string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

// Provider mints RDS IAM auth tokens for a region using AWS credentials.
type Provider struct {
	region string
	creds  aws.CredentialsProvider
	build  tokenBuilder
}

// New loads AWS config (default credential chain) for the given region.
func New(ctx context.Context, region string) (*Provider, error) {
	cfg, err := defaultLoadConfig(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return &Provider{region: region, creds: cfg.Credentials, build: defaultBuildToken}, nil
}

// BeforeConnect sets cc.Password to a freshly generated IAM token. It satisfies
// the db.Config.BeforeConnect hook signature.
func (p *Provider) BeforeConnect(ctx context.Context, cc *pgx.ConnConfig) error {
	endpoint := net.JoinHostPort(cc.Host, strconv.Itoa(int(cc.Port)))
	token, err := p.build(ctx, endpoint, p.region, cc.User, p.creds)
	if err != nil {
		return fmt.Errorf("build RDS IAM auth token: %w", err)
	}
	cc.Password = token
	return nil
}
