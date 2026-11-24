// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
)

const (
	fakeSTSCallerIdentity = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
<GetCallerIdentityResult><Arn>arn:aws:iam::111122223333:user/test</Arn><UserId>TEST</UserId><Account>111122223333</Account></GetCallerIdentityResult>
<ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></GetCallerIdentityResponse>`
	fakeSTSAssumeRole = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
<AssumeRoleResult><Credentials><AccessKeyId>AKID</AccessKeyId><SecretAccessKey>SECRET</SecretAccessKey><SessionToken>TOKEN</SessionToken><Expiration>2099-01-01T00:00:00Z</Expiration></Credentials>
<AssumedRoleUser><Arn>arn:aws:sts::111122223333:assumed-role/r/s</Arn><AssumedRoleId>ID:s</AssumedRoleId></AssumedRoleUser></AssumeRoleResult>
<ResponseMetadata><RequestId>2</RequestId></ResponseMetadata></AssumeRoleResponse>`
	fakeSTSAccessDenied = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
<Error><Type>Sender</Type><Code>AccessDenied</Code><Message>denied</Message></Error><RequestId>3</RequestId></ErrorResponse>`
	failingRoleARN = "arn:aws:iam::111122223333:role/fail"
)

// fakeSTS serves GetCallerIdentity and AssumeRole; AssumeRole for
// failingRoleARN is denied. It counts AssumeRole requests.
func fakeSTS(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var assumeRoles int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "text/xml")
		switch r.Form.Get("Action") {
		case "GetCallerIdentity":
			_, _ = w.Write([]byte(fakeSTSCallerIdentity))
		case "AssumeRole":
			atomic.AddInt32(&assumeRoles, 1)
			if r.Form.Get("RoleArn") == failingRoleARN {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(fakeSTSAccessDenied))
				return
			}
			_, _ = w.Write([]byte(fakeSTSAssumeRole))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &assumeRoles
}

// awsConfigForTest builds a config through NewAWSConfig against the fake STS
// endpoint, with static credentials and no shared config files.
func awsConfigForTest(
	t *testing.T,
	endpoint string,
	gateOn bool,
	roleARN ackv1alpha1.AWSResourceName,
) aws.Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	gates, err := featuregate.GetFeatureGatesWithOverrides(
		map[string]bool{featuregate.ConvergenceDetection: gateOn},
	)
	require.NoError(t, err)
	sc := newTestServiceController(ackcfg.Config{FeatureGates: gates})
	cfg, err := sc.NewAWSConfig(
		context.Background(),
		ackv1alpha1.AWSRegion("us-west-2"),
		aws.String(endpoint),
		roleARN,
		schema.GroupVersionKind{Group: "test", Version: "v1alpha1", Kind: "TestKind"},
		nil,
	)
	require.NoError(t, err)
	return cfg
}

func assumeRole(ctx context.Context, c *sts.Client, roleARN string) error {
	_, err := c.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String("test"),
	})
	return err
}

func TestAPICallRecorder_RecordsSuccessfulWritesOnly(t *testing.T) {
	require := require.New(t)
	srv, _ := fakeSTS(t)
	client := sts.NewFromConfig(awsConfigForTest(t, srv.URL, true, ""))
	okRole := "arn:aws:iam::111122223333:role/ok"

	ctx, rec := withAPICallRecorder(context.Background())
	_, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(err)
	require.Empty(rec.Calls(), "a read is not recorded")

	require.Error(assumeRole(ctx, client, failingRoleARN))
	require.Empty(rec.Calls(), "a failed write is not recorded")

	require.NoError(assumeRole(ctx, client, okRole))
	require.NoError(assumeRole(ctx, client, okRole))
	require.Equal([]string{"STS.AssumeRole"}, rec.Calls(), "a repeated write is recorded once")

	// Without a recorder in the context the middleware does nothing.
	require.NoError(assumeRole(context.Background(), client, okRole))
}

func TestAPICallRecorder_GateOffAddsNoMiddleware(t *testing.T) {
	require := require.New(t)
	srv, _ := fakeSTS(t)
	cfg := awsConfigForTest(t, srv.URL, false, "")
	require.Empty(cfg.APIOptions)

	ctx, rec := withAPICallRecorder(context.Background())
	require.NoError(assumeRole(ctx, sts.NewFromConfig(cfg), "arn:aws:iam::111122223333:role/ok"))
	require.Empty(rec.Calls())
}

// The credentials of an assumed role are fetched with the request's context
// in the middle of an Update. That AssumeRole call must not count as a write.
func TestAPICallRecorder_IgnoresCredentialRefresh(t *testing.T) {
	require := require.New(t)
	srv, assumeRoles := fakeSTS(t)
	cfg := awsConfigForTest(t, srv.URL, true, "arn:aws:iam::111122223333:role/carm")
	client := sts.NewFromConfig(cfg)

	ctx, rec := withAPICallRecorder(context.Background())
	_, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(err)
	require.Equal(int32(1), atomic.LoadInt32(assumeRoles), "the role was assumed during the call")
	require.Empty(rec.Calls())
}

func TestIsReadOperation(t *testing.T) {
	for _, op := range []string{
		"DescribeDBInstances", "GetQueueAttributes", "ListTagsForResource",
		"HeadBucket", "BatchGetCollection",
	} {
		require.True(t, isReadOperation(op), op)
	}
	for _, op := range []string{
		"SetQueueAttributes", "PutMetricAlarm", "UpdateWebACL",
		"ModifySubnetAttribute", "TagResource", "BatchDisassociateScramSecret",
	} {
		require.False(t, isReadOperation(op), op)
	}
}
