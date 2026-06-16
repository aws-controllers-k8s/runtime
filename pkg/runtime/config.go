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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

const appName = "aws-controllers-k8s"

// addACKUserAgent returns a middleware function that prepends the ACK
// User-Agent string to outgoing HTTP requests. This uses the smithy
// middleware stack directly to manipulate the User-Agent header without
// character sanitization, preserving the original format including
// parentheses, semicolons, and slashes.
func addACKUserAgent(userAgent string) func(*smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Build.Add(
			smithymiddleware.BuildMiddlewareFunc("ACKUserAgent",
				func(ctx context.Context, in smithymiddleware.BuildInput, next smithymiddleware.BuildHandler) (
					smithymiddleware.BuildOutput, smithymiddleware.Metadata, error,
				) {
					req, ok := in.Request.(*smithyhttp.Request)
					if ok {
						existing := req.Header.Get("User-Agent")
						if existing != "" {
							req.Header.Set("User-Agent", userAgent+" "+existing)
						} else {
							req.Header.Set("User-Agent", userAgent)
						}
					}
					return next.HandleBuild(ctx, in)
				},
			),
			smithymiddleware.After,
		)
	}
}

func (c *serviceController) NewAWSConfig(
	ctx context.Context,
	region ackv1alpha1.AWSRegion,
	endpointURL *string,
	roleARN ackv1alpha1.AWSResourceName,
	groupVersionKind schema.GroupVersionKind,
	labels map[string]string,
) (aws.Config, error) {

	extra := []string{
		"GitCommit/" + c.VersionInfo.GitCommit,
		"BuildDate/" + c.VersionInfo.BuildDate,
		"CRDKind/" + groupVersionKind.Kind,
		"CRDVersion/" + groupVersionKind.Version,
	}

	// Add kro managed info if managed by kro
	if isKROManaged(labels) {
		extra = append(extra, "ManagedBy/kro")
		if kroVersion := getKROVersion(labels); kroVersion != "" {
			extra = append(extra, "KROVersion/"+kroVersion)
		}
	}

	val := formatUserAgent(
		appName,
		groupVersionKind.Group+"-"+c.VersionInfo.GitVersion,
		extra...,
	)

	httpClient := awshttp.NewBuildableClient()
	if c.cfg.HTTPClientTimeout > 0 {
		httpClient = httpClient.WithTimeout(c.cfg.HTTPClientTimeout)
	}

	// Pass the *awshttp.BuildableClient directly instead of wrapping it in a
	// custom struct. This ensures the SDK can type-assert the client to
	// *awshttp.BuildableClient when it needs to modify transport options
	// (e.g., when AWS_CA_BUNDLE is set to inject custom root CAs).
	// The custom User-Agent is injected via a smithy Build middleware that
	// directly manipulates the HTTP header, preserving the original format.
	// See: https://github.com/aws-controllers-k8s/community/issues/2915
	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(string(region)),
		config.WithHTTPClient(httpClient),
		config.WithAPIOptions([]func(*smithymiddleware.Stack) error{
			addACKUserAgent(val),
		}),
	)
	if err != nil {
		return awsCfg, err
	}

	if endpointURL != nil && *endpointURL != "" {
		awsCfg.BaseEndpoint = endpointURL
	}

	if roleARN != "" {
		client := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(client, string(roleARN))
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}
	return awsCfg, nil
}

func formatUserAgent(name, version string, extra ...string) string {
	ua := fmt.Sprintf("%s/%s", name, version)
	if len(extra) > 0 {
		ua += fmt.Sprintf(" (%s)", strings.Join(extra, "; "))
	}
	return ua
}
