package awsauth

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jackc/pgx/v5"
)

func TestNew_Success(t *testing.T) {
	orig := defaultLoadConfig
	defaultLoadConfig = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	t.Cleanup(func() { defaultLoadConfig = orig })

	p, err := New(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.region != "us-east-1" || p.build == nil {
		t.Errorf("provider not initialized: %+v", p)
	}
}

// TestNew_RealLoader exercises the real defaultLoadConfig seam (LoadDefaultConfig
// builds the credential chain lazily, so this does not touch the network).
func TestNew_RealLoader(t *testing.T) {
	p, err := New(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("New with real loader: %v", err)
	}
	if p.creds == nil {
		t.Error("expected a credentials provider")
	}
}

func TestNew_LoadError(t *testing.T) {
	orig := defaultLoadConfig
	defaultLoadConfig = func(context.Context, string) (aws.Config, error) {
		return aws.Config{}, errors.New("no credentials")
	}
	t.Cleanup(func() { defaultLoadConfig = orig })

	if _, err := New(context.Background(), "us-east-1"); err == nil {
		t.Fatal("expected load error")
	}
}

func TestBeforeConnect_Success(t *testing.T) {
	var gotEndpoint, gotRegion, gotUser string
	p := &Provider{
		region: "eu-west-1",
		build: func(_ context.Context, endpoint, region, dbUser string, _ aws.CredentialsProvider) (string, error) {
			gotEndpoint, gotRegion, gotUser = endpoint, region, dbUser
			return "generated-token", nil
		},
	}
	cc := &pgx.ConnConfig{}
	cc.Host = "db.example.rds.amazonaws.com"
	cc.Port = 5432
	cc.User = "descoperead"

	if err := p.BeforeConnect(context.Background(), cc); err != nil {
		t.Fatalf("BeforeConnect: %v", err)
	}
	if cc.Password != "generated-token" {
		t.Errorf("password = %q, want generated-token", cc.Password)
	}
	if gotEndpoint != "db.example.rds.amazonaws.com:5432" {
		t.Errorf("endpoint = %q", gotEndpoint)
	}
	if gotRegion != "eu-west-1" || gotUser != "descoperead" {
		t.Errorf("region/user = %q/%q", gotRegion, gotUser)
	}
}

func TestBeforeConnect_BuildError(t *testing.T) {
	p := &Provider{
		region: "eu-west-1",
		build: func(context.Context, string, string, string, aws.CredentialsProvider) (string, error) {
			return "", errors.New("sts denied")
		},
	}
	cc := &pgx.ConnConfig{}
	cc.Host = "h"
	cc.Port = 5432
	cc.User = "u"
	if err := p.BeforeConnect(context.Background(), cc); err == nil {
		t.Fatal("expected build error")
	}
}

// TestDefaultBuildToken exercises the real token builder seam (no network: it
// signs locally with static credentials).
func TestDefaultBuildToken(t *testing.T) {
	creds := aws.NewCredentialsCache(staticCreds{})
	tok, err := defaultBuildToken(context.Background(), "host:5432", "us-east-1", "user", creds)
	if err != nil {
		t.Fatalf("buildToken: %v", err)
	}
	if tok == "" {
		t.Error("expected a non-empty token")
	}
}

type staticCreds struct{}

func (staticCreds) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", Source: "test"}, nil
}
